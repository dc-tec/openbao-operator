/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openbaov1alpha1.AddToScheme(scheme))
}

// Known operator manager names that should be filtered out to prevent infinite loops.
// These names may vary based on how controller-runtime client was initialized.
var operatorManagerNames = []string{
	"openbao-operator",
	"openbao-operator-controller",
	"manager",
	"openbao-controller",
}

// isOperatorUpdate checks if the most recent change was made by the operator.
// This prevents infinite loops where the operator updates a resource and the Sentinel
// immediately triggers reconciliation again.
func isOperatorUpdate(obj metav1.Object, operatorNames []string) bool {
	managedFields := obj.GetManagedFields()
	if len(managedFields) == 0 {
		return false
	}

	// Kubernetes sorts managedFields by time. The most recent entry is the last one.
	mostRecent := managedFields[len(managedFields)-1]

	for _, name := range operatorNames {
		if mostRecent.Manager == name {
			return true
		}
	}
	return false
}

// isSentinelManagedResource checks if a resource is managed by the OpenBao operator
// by checking labels.
func isSentinelManagedResource(obj metav1.Object, clusterName string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	managedBy := labels[constants.LabelAppManagedBy]
	instance := labels[constants.LabelAppInstance]

	return managedBy == constants.LabelValueAppManagedByOpenBaoOperator && instance == clusterName
}

// isUnsealOrRootTokenSecret checks if a Secret is an unseal key or root token.
func isUnsealOrRootTokenSecret(secret *corev1.Secret) bool {
	name := secret.Name
	return strings.Contains(name, "-unseal-key") || strings.Contains(name, "-root-token")
}

// pendingTrigger tracks whether a trigger is pending and when it was last set.
type pendingTrigger struct {
	mu          sync.RWMutex
	lastSetTime time.Time
	isPending   bool
}

func (p *pendingTrigger) set() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSetTime = time.Now()
	p.isPending = true
}

func (p *pendingTrigger) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.isPending = false
}

// SentinelReconciler reconciles infrastructure resources to detect drift.
type SentinelReconciler struct {
	client           client.Client
	logger           logr.Logger
	clusterName      string
	clusterNamespace string
	debounceWindow   time.Duration
	pendingTrigger   *pendingTrigger
	healthStatus     *healthStatus
}

type healthStatus struct {
	mu        sync.RWMutex
	ready     bool
	started   bool
	lastError string
}

func (h *healthStatus) setReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

func (h *healthStatus) setStarted(started bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = started
}

func (h *healthStatus) setError(err string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastError = err
}

func (h *healthStatus) isReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready && h.started
}

// Reconcile handles drift detection for a generic Kubernetes resource.
// It uses RequeueAfter to implement debouncing via controller-runtime's workqueue.
// The debouncing works by:
// 1. When drift is detected, mark a trigger as pending and return RequeueAfter
// 2. The workqueue will schedule the reconcile after the debounce window
// 3. On the next reconcile, if the trigger is still pending and the window has passed, fire it
// This batches multiple rapid drift detections into a single trigger.
func (r *SentinelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.logger.WithValues("resource", req.NamespacedName)

	// Check if we have a pending trigger that's ready to fire
	// Use a mutex-protected check-and-clear to ensure only one reconcile fires the trigger
	r.pendingTrigger.mu.Lock()
	shouldTrigger := r.pendingTrigger.isPending && time.Since(r.pendingTrigger.lastSetTime) >= r.debounceWindow
	if shouldTrigger {
		r.pendingTrigger.isPending = false
	}
	r.pendingTrigger.mu.Unlock()

	if shouldTrigger {
		if err := r.triggerReconciliation(ctx); err != nil {
			r.logger.Error(err, "Failed to trigger reconciliation")
			r.healthStatus.setError(err.Error())
			// Return error to let controller-runtime handle retry with backoff
			return ctrl.Result{}, fmt.Errorf("failed to trigger reconciliation: %w", err)
		}
		r.healthStatus.setError("")
		r.logger.Info("Triggered reconciliation via annotation patch")
		// No requeue needed - the annotation patch will trigger the main controller
		return ctrl.Result{}, nil
	}

	// Try to fetch as each resource type until we find a match
	// This is necessary because controller-runtime doesn't always set req.Kind
	var obj client.Object
	var err error

	// Try StatefulSet first
	obj = &appsv1.StatefulSet{}
	err = r.client.Get(ctx, req.NamespacedName, obj)
	if err == nil {
		goto found
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get StatefulSet: %w", err)
	}

	// Try Service
	obj = &corev1.Service{}
	err = r.client.Get(ctx, req.NamespacedName, obj)
	if err == nil {
		goto found
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get Service: %w", err)
	}

	// Try ConfigMap
	obj = &corev1.ConfigMap{}
	err = r.client.Get(ctx, req.NamespacedName, obj)
	if err == nil {
		goto found
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Resource not found - ignore (may have been deleted)
	return ctrl.Result{}, nil

found:

	// Filter: Only watch resources managed by the operator
	if !isSentinelManagedResource(obj, r.clusterName) {
		logger.V(1).Info("Resource not managed by operator, ignoring",
			"labels", obj.GetLabels(),
			"expected_managed_by", constants.LabelValueAppManagedByOpenBaoOperator,
			"expected_instance", r.clusterName)
		return ctrl.Result{}, nil
	}

	// Actor Filter: Ignore updates made by the operator itself
	if isOperatorUpdate(obj, operatorManagerNames) {
		managerName := "unknown"
		if len(obj.GetManagedFields()) > 0 {
			managerName = obj.GetManagedFields()[0].Manager
		}
		logger.V(1).Info("Ignoring operator update to prevent loop", "manager", managerName)
		return ctrl.Result{}, nil
	}

	// Drift detected! Mark as pending and requeue after debounce window
	// This allows multiple rapid drift detections to be batched into a single trigger
	logger.Info("Drift detected, marking trigger as pending", "resource", req.NamespacedName)
	r.pendingTrigger.set()

	// Requeue after debounce window to allow batching of multiple drift events
	// The workqueue will handle the timing and ensure we only trigger once
	return ctrl.Result{RequeueAfter: r.debounceWindow}, nil
}

// triggerReconciliation patches the OpenBaoCluster with the trigger annotation.
func (r *SentinelReconciler) triggerReconciliation(ctx context.Context) error {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	key := types.NamespacedName{
		Namespace: r.clusterNamespace,
		Name:      r.clusterName,
	}

	if err := r.client.Get(ctx, key, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("OpenBaoCluster %s/%s not found, Sentinel should exit", r.clusterNamespace, r.clusterName)
		}
		return fmt.Errorf("failed to get OpenBaoCluster: %w", err)
	}

	// Check if annotation already exists with the same value (idempotency)
	existingValue, exists := cluster.Annotations[constants.AnnotationSentinelTrigger]
	newValue := time.Now().Format(time.RFC3339Nano)
	if exists && existingValue == newValue {
		// Annotation already set with current timestamp, no need to patch
		r.logger.V(1).Info("Trigger annotation already set with current value, skipping patch")
		return nil
	}

	// Patch annotation
	original := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	cluster.Annotations[constants.AnnotationSentinelTrigger] = newValue

	if err := r.client.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		// Check for admission errors (VAP rejection)
		if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
			r.logger.Error(err, "Patch rejected by admission control - check ValidatingAdmissionPolicy",
				"annotation", constants.AnnotationSentinelTrigger,
				"existing_annotations", cluster.Annotations)
			return fmt.Errorf("patch rejected by admission control: %w", err)
		}
		if apierrors.IsConflict(err) {
			// Conflict is transient, will retry via controller-runtime backoff
			return fmt.Errorf("conflict patching OpenBaoCluster: %w", err)
		}
		return fmt.Errorf("failed to patch OpenBaoCluster: %w", err)
	}

	// Verify the patch succeeded by reading back the resource
	// This helps catch cases where the patch appears to succeed but the annotation isn't actually set
	// Note: There are several race conditions that can occur:
	// 1. The operator might have already processed the trigger and cleared the annotation
	// 2. Another sentinel reconciliation might have set a different (newer) value
	// 3. API server eventual consistency might not reflect our patch yet
	// We only log verification results but don't fail, as the patch operation itself succeeded
	// and any of these race conditions are acceptable outcomes
	verifyCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.client.Get(ctx, key, verifyCluster); err != nil {
		r.logger.V(1).Info("Could not verify patch - failed to read back OpenBaoCluster",
			"error", err)
		// Don't fail here - the patch might have succeeded but we just can't verify it
	} else {
		if val, ok := verifyCluster.Annotations[constants.AnnotationSentinelTrigger]; ok {
			// Annotation exists - check if it has the expected value
			if val == newValue {
				r.logger.V(1).Info("Successfully patched and verified OpenBaoCluster with trigger annotation",
					"annotation", constants.AnnotationSentinelTrigger,
					"value", newValue)
			} else {
				// Annotation has a different value - this could mean:
				// 1. Another sentinel reconciliation set a newer value (acceptable - drift was detected again)
				// 2. API server eventual consistency hasn't reflected our patch yet (acceptable - will be retried)
				// 3. An older value somehow persisted (rare but acceptable - will be overwritten on next reconciliation)
				// We log at debug level but don't fail, as the patch operation succeeded
				r.logger.V(1).Info("Patch verification: annotation has different value - may be due to concurrent reconciliation or eventual consistency",
					"annotation", constants.AnnotationSentinelTrigger,
					"expected_value", newValue,
					"actual_value", val)
			}
		} else {
			// Annotation doesn't exist - this could mean:
			// 1. The operator already processed the trigger and cleared it (acceptable - drift was handled)
			// 2. The patch failed (but Patch() didn't return an error - rare but acceptable, will retry)
			// We log at debug level but don't fail, as the operator processing it is acceptable
			r.logger.V(1).Info("Patch verification: annotation not found - may have been cleared by operator",
				"annotation", constants.AnnotationSentinelTrigger,
				"expected_value", newValue)
		}
	}

	return nil
}

// setupHealthEndpoint starts an HTTP server for health checks.
func (r *SentinelReconciler) setupHealthEndpoint(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if r.healthStatus.isReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Not Ready"))
		}
	})

	server := &http.Server{
		Addr:    ":8082",
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			r.logger.Error(err, "Health endpoint server error")
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. "+
		"Enabling this will ensure there is only one active controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Load configuration from environment
	clusterNamespace := strings.TrimSpace(os.Getenv(constants.EnvPodNamespace))
	if clusterNamespace == "" {
		setupLog.Error(nil, "POD_NAMESPACE environment variable is required")
		os.Exit(1)
	}

	clusterName := strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if clusterName == "" {
		setupLog.Error(nil, "CLUSTER_NAME environment variable is required")
		os.Exit(1)
	}

	debounceWindowStr := strings.TrimSpace(os.Getenv(constants.EnvSentinelDebounceWindowSeconds))
	if debounceWindowStr == "" {
		debounceWindowStr = "2" // Default
	}
	debounceWindowSeconds, err := strconv.ParseInt(debounceWindowStr, 10, 32)
	if err != nil {
		setupLog.Error(err, "Invalid SENTINEL_DEBOUNCE_WINDOW_SECONDS", "value", debounceWindowStr)
		os.Exit(1)
	}
	debounceWindow := time.Duration(debounceWindowSeconds) * time.Second

	setupLog.Info("Starting Sentinel",
		"cluster_namespace", clusterNamespace,
		"cluster_name", clusterName,
		"debounce_window", debounceWindow)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       fmt.Sprintf("openbao-sentinel-%s-%s", clusterNamespace, clusterName),
		// Restrict cache to the cluster namespace to align with namespace-scoped RBAC permissions.
		// This prevents the cache from requiring cluster-wide list/watch permissions.
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				clusterNamespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &SentinelReconciler{
		client:           mgr.GetClient(),
		logger:           ctrl.Log.WithName("sentinel").WithValues("cluster_namespace", clusterNamespace, "cluster_name", clusterName, "component", "sentinel"),
		clusterName:      clusterName,
		clusterNamespace: clusterNamespace,
		debounceWindow:   debounceWindow,
		pendingTrigger:   &pendingTrigger{},
		healthStatus:     &healthStatus{},
	}

	// Create a single controller that watches multiple resource types
	// Use StatefulSet as the primary resource and add watches for others
	driftPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isSentinelManagedResource(e.Object, clusterName)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			isManaged := isSentinelManagedResource(e.ObjectNew, clusterName)
			if !isManaged {
				return false
			}

			// Check if the most recent update was from the operator
			// However, if the resource was recently updated by a non-operator (within last 10 seconds),
			// we should still detect it as drift, even if the operator then reconciled it.
			// This handles the race condition where the operator fixes drift before Sentinel detects it.
			mostRecentIsOperator := isOperatorUpdate(e.ObjectNew, operatorManagerNames)

			// If the most recent update is from the operator, check for drift indicators:
			// 1. Check all managedFields entries for recent non-operator updates
			// 2. Compare old vs new annotations to detect if new annotations were added
			//    (SSA with ForceOwnership can take ownership of fields without removing values)
			if mostRecentIsOperator {
				managedFields := e.ObjectNew.GetManagedFields()
				// Check all managedFields entries for recent non-operator updates
				// (not just the second one, as SSA can create multiple entries)
				for _, field := range managedFields {
					isFieldOperator := false
					for _, name := range operatorManagerNames {
						if field.Manager == name {
							isFieldOperator = true
							break
						}
					}

					// If this field was managed by a non-operator and updated recently, it's likely drift
					if !isFieldOperator && time.Since(field.Time.Time) < 10*time.Second {
						setupLog.V(1).Info("Update event passed predicate: recent non-operator update detected",
							"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
							"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
							"most_recent_manager", managedFields[0].Manager,
							"non_operator_manager", field.Manager,
							"non_operator_time", field.Time.Time)
						return true
					}
				}

				// Check if annotations exist that operator wouldn't set
				// This handles the case where SSA takes ownership but preserves annotation values.
				// We check if non-operator annotations exist AND the resource was recently updated
				// (within last 10 seconds), indicating recent drift that the operator just reconciled.
				oldAnnotations := e.ObjectOld.GetAnnotations()
				newAnnotations := e.ObjectNew.GetAnnotations()

				// Handle nil annotations by treating them as empty maps
				// This is critical because Services created by the operator often have nil annotations
				if oldAnnotations == nil {
					oldAnnotations = make(map[string]string)
				}
				if newAnnotations == nil {
					newAnnotations = make(map[string]string)
				}

				if len(newAnnotations) > 0 && len(managedFields) > 0 {
					// Check if the most recent update was recent (within last 10 seconds)
					// The most recent entry is the last one in the managedFields slice
					mostRecentTime := managedFields[len(managedFields)-1].Time.Time
					recentlyUpdated := time.Since(mostRecentTime) < 10*time.Second

					if recentlyUpdated {
						// Check if any annotations exist that don't match operator patterns
						for key := range newAnnotations {
							// Ignore test annotations (e.g., e2e.openbao.org/*) - these are added by tests
							// and may persist after operator reconciliation due to SSA behavior
							if strings.HasPrefix(key, "e2e.") {
								continue
							}
							// Operator-managed annotations typically start with "openbao.org/" or "kubectl.kubernetes.io/"
							// Test annotations or other drift indicators might have different prefixes
							if !strings.HasPrefix(key, "openbao.org/") &&
								!strings.HasPrefix(key, "kubectl.kubernetes.io/") &&
								!strings.HasPrefix(key, "deployment.kubernetes.io/") {
								// This looks like a non-operator annotation that exists after operator reconciliation
								// Check if it was added/changed by comparing with old object
								oldValue, existed := oldAnnotations[key]
								wasAddedOrChanged := !existed || oldValue != newAnnotations[key]

								if wasAddedOrChanged {
									setupLog.V(1).Info("Update event passed predicate: non-operator annotation detected",
										"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
										"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
										"annotation", key,
										"most_recent_manager", managedFields[len(managedFields)-1].Manager,
										"most_recent_time", mostRecentTime)
									return true
								}
							}
						}
					}
				}

				setupLog.V(1).Info("Update event filtered: operator update",
					"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
					"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
					"manager", func() string {
						fields := e.ObjectNew.GetManagedFields()
						if len(fields) > 0 {
							return fields[len(fields)-1].Manager
						}
						return "unknown"
					}())
				return false
			}

			// Most recent update is from non-operator, this is drift
			setupLog.V(1).Info("Update event passed predicate filter: non-operator update",
				"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
				"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false // Ignore deletions
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	// Create a single controller that watches StatefulSet as primary
	// and uses Watches for other resource types with predicates
	// Note: The predicate is applied via WithEventFilter which applies to all watches
	// In controller-runtime v0.22, Watches accepts client.Object directly
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		WithEventFilter(driftPredicate).
		Watches(&corev1.Service{}, &handler.EnqueueRequestForObject{}).
		Watches(&corev1.ConfigMap{}, &handler.EnqueueRequestForObject{})

	if err := builder.Complete(reconciler); err != nil {
		setupLog.Error(err, "unable to create Sentinel controller")
		os.Exit(1)
	}

	// Setup health endpoint
	ctx := ctrl.SetupSignalHandler()
	go reconciler.setupHealthEndpoint(ctx)

	// Mark as started
	reconciler.healthStatus.setStarted(true)
	reconciler.healthStatus.setReady(true)

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
