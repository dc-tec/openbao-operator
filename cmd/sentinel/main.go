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
	"hash/fnv"
	"math"
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
	"k8s.io/apimachinery/pkg/util/uuid"
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

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/controller/openbaocluster"
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

// pendingTrigger tracks whether a trigger is pending and when it was last set.
type pendingTrigger struct {
	mu           sync.RWMutex
	lastSetTime  time.Time
	isPending    bool
	resourceKind string
	resourceName string
}

func (p *pendingTrigger) set(resourceKind, resourceName string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSetTime = time.Now()
	p.isPending = true
	p.resourceKind = resourceKind
	p.resourceName = resourceName
}

func (p *pendingTrigger) getResourceInfo() (kind, name string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.resourceKind, p.resourceName
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
		// Get resource info from the pending trigger
		resourceKind, resourceName := r.pendingTrigger.getResourceInfo()
		if err := r.triggerReconciliation(ctx, resourceKind, resourceName); err != nil {
			r.logger.Error(err, "Failed to trigger reconciliation")
			r.healthStatus.setError(err.Error())
			// Return error to let controller-runtime handle retry with backoff
			return ctrl.Result{}, fmt.Errorf("failed to trigger reconciliation: %w", err)
		}
		r.healthStatus.setError("")
		r.logger.Info("Triggered reconciliation via annotation patch",
			"resource_kind", resourceKind,
			"resource_name", resourceName)
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
	resourceKind := obj.GetObjectKind().GroupVersionKind().Kind
	nsName := req.NamespacedName
	resourceName := nsName.String()
	logger.Info("Drift detected, marking trigger as pending",
		"resource", req.NamespacedName,
		"resource_kind", resourceKind)
	r.pendingTrigger.set(resourceKind, resourceName)

	// Requeue after debounce window to allow batching of multiple drift events
	// The workqueue will handle the timing and ensure we only trigger once
	return ctrl.Result{RequeueAfter: r.debounceWindow}, nil
}

// triggerReconciliation patches the OpenBaoCluster status with a drift trigger.
// resourceKind and resourceName identify the resource that triggered the drift detection.
func (r *SentinelReconciler) triggerReconciliation(ctx context.Context, resourceKind, resourceName string) error {
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

	// Use UUID instead of timestamp to avoid timing-related race conditions
	// when the operator processes triggers.
	triggerID := string(uuid.NewUUID())
	original := cluster.DeepCopy()

	if cluster.Status.Sentinel == nil {
		cluster.Status.Sentinel = &openbaov1alpha1.SentinelStatus{}
	}
	cluster.Status.Sentinel.TriggerID = triggerID
	now := metav1.Now()
	cluster.Status.Sentinel.TriggeredAt = &now
	if resourceKind != "" && resourceName != "" {
		cluster.Status.Sentinel.TriggerResource = fmt.Sprintf("%s/%s", resourceKind, resourceName)
	}

	if err := r.client.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		// Status writes are blocked only by RBAC or API errors; there is no admission policy on status.
		if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) {
			r.logger.Error(err, "Status patch rejected; check RBAC for openbaoclusters/status")
			return fmt.Errorf("status patch rejected: %w", err)
		}
		if apierrors.IsConflict(err) {
			return fmt.Errorf("conflict patching OpenBaoCluster status: %w", err)
		}
		return fmt.Errorf("failed to patch OpenBaoCluster status: %w", err)
	}

	verifyCluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.client.Get(ctx, key, verifyCluster); err != nil {
		r.logger.V(1).Info("Could not verify status patch - failed to read back OpenBaoCluster", "error", err)
		return nil
	}
	if verifyCluster.Status.Sentinel == nil || verifyCluster.Status.Sentinel.TriggerID == "" {
		r.logger.V(1).Info(
			"Status patch verification: trigger not present yet (eventual consistency)",
			"triggerID", triggerID,
		)
		return nil
	}
	if verifyCluster.Status.Sentinel.TriggerID != triggerID {
		r.logger.V(1).Info("Status patch verification: triggerID differs (concurrent trigger?)",
			"expected", triggerID,
			"actual", verifyCluster.Status.Sentinel.TriggerID)
		return nil
	}
	r.logger.V(1).Info("Successfully patched OpenBaoCluster status with trigger", "triggerID", triggerID)

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
		Addr:              ":8082",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second, // Prevent Slowloris attack
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

// unknownManager is used when the manager name cannot be determined from managedFields.
const unknownManager = "unknown"

// sentinelConfig holds the configuration loaded from environment variables.
type sentinelConfig struct {
	clusterNamespace string
	clusterName      string
	debounceWindow   time.Duration

	debounceWindowBase   time.Duration
	debounceJitterRange  time.Duration
	debounceWindowJitter time.Duration
}

const (
	defaultSentinelDebounceWindowSeconds = 2
	defaultSentinelDebounceJitterSeconds = 5
)

func computeDeterministicDebounceJitter(clusterName string, jitterRangeSeconds int64) time.Duration {
	if clusterName == "" || jitterRangeSeconds <= 0 {
		return 0
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(clusterName))

	sum := h.Sum64()
	jitterSeconds := sum % uint64(jitterRangeSeconds)
	// time.Duration is int64 underneath; clamp to avoid overflow on conversion.
	if jitterSeconds > uint64(math.MaxInt64) {
		jitterSeconds = uint64(math.MaxInt64)
	}
	// #nosec G115 -- jitterSeconds is clamped to MaxInt64, so the conversion is safe.
	return time.Duration(int64(jitterSeconds)) * time.Second
}

// loadSentinelConfig loads configuration from environment variables.
// Returns an error if required environment variables are missing or invalid.
func loadSentinelConfig() (*sentinelConfig, error) {
	clusterNamespace := strings.TrimSpace(os.Getenv(constants.EnvPodNamespace))
	if clusterNamespace == "" {
		return nil, fmt.Errorf("POD_NAMESPACE environment variable is required")
	}

	clusterName := strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if clusterName == "" {
		return nil, fmt.Errorf("CLUSTER_NAME environment variable is required")
	}

	debounceWindowStr := strings.TrimSpace(os.Getenv(constants.EnvSentinelDebounceWindowSeconds))
	if debounceWindowStr == "" {
		debounceWindowStr = strconv.Itoa(defaultSentinelDebounceWindowSeconds)
	}
	debounceWindowSeconds, err := strconv.ParseInt(debounceWindowStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid SENTINEL_DEBOUNCE_WINDOW_SECONDS %q: %w", debounceWindowStr, err)
	}

	debounceJitterStr := strings.TrimSpace(os.Getenv(constants.EnvSentinelDebounceJitterSeconds))
	if debounceJitterStr == "" {
		debounceJitterStr = strconv.Itoa(defaultSentinelDebounceJitterSeconds)
	}
	debounceJitterRangeSeconds, err := strconv.ParseInt(debounceJitterStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid SENTINEL_DEBOUNCE_JITTER_SECONDS %q: %w", debounceJitterStr, err)
	}
	if debounceJitterRangeSeconds < 0 {
		return nil, fmt.Errorf("invalid SENTINEL_DEBOUNCE_JITTER_SECONDS %q: must be >= 0", debounceJitterStr)
	}

	base := time.Duration(debounceWindowSeconds) * time.Second
	jitter := computeDeterministicDebounceJitter(clusterName, debounceJitterRangeSeconds)
	effective := base + jitter

	return &sentinelConfig{
		clusterNamespace:     clusterNamespace,
		clusterName:          clusterName,
		debounceWindow:       effective,
		debounceWindowBase:   base,
		debounceJitterRange:  time.Duration(debounceJitterRangeSeconds) * time.Second,
		debounceWindowJitter: jitter,
	}, nil
}

// hasRecentNonOperatorUpdate checks if any managed field was updated by a non-operator
// within the specified time window, indicating potential drift.
func hasRecentNonOperatorUpdate(managedFields []metav1.ManagedFieldsEntry, window time.Duration) bool {
	for _, field := range managedFields {
		isOperator := false
		for _, name := range operatorManagerNames {
			if field.Manager == name {
				isOperator = true
				break
			}
		}
		if !isOperator && time.Since(field.Time.Time) < window {
			return true
		}
	}
	return false
}

// ignoredAnnotationPrefixes lists annotation prefixes that should not trigger drift detection.
var ignoredAnnotationPrefixes = []string{
	"openbao.org/",           // Operator-managed annotations (except maintenance, handled separately)
	"kubectl.kubernetes.io/", // kubectl annotations
	"deployment.kubernetes.io/",
	"service.beta.kubernetes.io/",
	"service.kubernetes.io/",
	"cloud.google.com/",
	"eks.amazonaws.com/",
	"azure.workload.identity/",
}

// hasNonOperatorAnnotationChange checks if any non-operator annotation was added or changed.
// Returns true if a drift-indicating annotation change was detected.
func hasNonOperatorAnnotationChange(
	oldAnnotations, newAnnotations map[string]string,
	managedFields []metav1.ManagedFieldsEntry,
) bool {
	if len(newAnnotations) == 0 || len(managedFields) == 0 {
		return false
	}

	// Check if the most recent update was recent (within last 10 seconds)
	mostRecentTime := managedFields[len(managedFields)-1].Time.Time
	if time.Since(mostRecentTime) >= 10*time.Second {
		return false
	}

	for key, newVal := range newAnnotations {
		// Ignore test annotations (e2e.*)
		if strings.HasPrefix(key, "e2e.") {
			continue
		}
		// Ignore maintenance-allowed annotation
		if key == constants.AnnotationMaintenanceAllowed {
			continue
		}

		// Check if annotation was added or changed
		oldVal, existed := oldAnnotations[key]
		wasAddedOrChanged := !existed || oldVal != newVal
		if !wasAddedOrChanged {
			continue
		}

		// Maintenance annotation should trigger drift detection
		if key == constants.AnnotationMaintenance {
			return true
		}

		// Check if annotation matches any ignored prefix
		isIgnored := false
		for _, prefix := range ignoredAnnotationPrefixes {
			if strings.HasPrefix(key, prefix) {
				isIgnored = true
				break
			}
		}
		if isIgnored {
			continue
		}

		// Found a non-operator annotation that was added or changed
		return true
	}

	return false
}

// getMostRecentManager returns the manager name from the most recent managed field entry.
func getMostRecentManager(managedFields []metav1.ManagedFieldsEntry) string {
	if len(managedFields) > 0 {
		return managedFields[len(managedFields)-1].Manager
	}
	return unknownManager
}

// buildDriftPredicate creates the predicate for drift detection on managed resources.
func buildDriftPredicate(clusterName string) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isSentinelManagedResource(e.Object, clusterName)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return evaluateUpdateForDrift(e, clusterName)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false // Ignore deletions
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// evaluateUpdateForDrift determines if an update event indicates drift that needs correction.
func evaluateUpdateForDrift(e event.UpdateEvent, clusterName string) bool {
	if !isSentinelManagedResource(e.ObjectNew, clusterName) {
		return false
	}

	mostRecentIsOperator := isOperatorUpdate(e.ObjectNew, operatorManagerNames)

	// If the most recent update is from the operator, apply additional checks
	if mostRecentIsOperator {
		return checkOperatorUpdateForDrift(e)
	}

	// Most recent update is from non-operator, this is drift
	setupLog.V(1).Info("Update event passed predicate filter: non-operator update",
		"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
		"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind)
	return true
}

// checkOperatorUpdateForDrift performs additional drift detection when the most recent
// update was from the operator. This catches cases where drift was quickly reconciled.
func checkOperatorUpdateForDrift(e event.UpdateEvent) bool {
	managedFields := e.ObjectNew.GetManagedFields()
	if len(managedFields) == 0 {
		return false
	}

	mostRecentTime := managedFields[len(managedFields)-1].Time.Time

	// Grace period: Ignore updates within 5 seconds of operator reconciliation
	gracePeriod := 5 * time.Second
	if time.Since(mostRecentTime) < gracePeriod {
		setupLog.V(1).Info("Update event filtered: within grace period after operator reconciliation",
			"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
			"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
			"time_since_operator_update", time.Since(mostRecentTime))
		return false
	}

	// Check for recent non-operator updates (within 15 seconds)
	if hasRecentNonOperatorUpdate(managedFields, 15*time.Second) {
		setupLog.V(1).Info("Update event passed predicate: recent non-operator update detected",
			"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
			"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
			"most_recent_manager", getMostRecentManager(managedFields))
		return true
	}

	// Check for annotation changes
	oldAnnotations := e.ObjectOld.GetAnnotations()
	newAnnotations := e.ObjectNew.GetAnnotations()
	if oldAnnotations == nil {
		oldAnnotations = make(map[string]string)
	}
	if newAnnotations == nil {
		newAnnotations = make(map[string]string)
	}

	if hasNonOperatorAnnotationChange(oldAnnotations, newAnnotations, managedFields) {
		setupLog.V(1).Info("Update event passed predicate: non-operator annotation detected",
			"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
			"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
			"most_recent_manager", getMostRecentManager(managedFields))
		return true
	}

	setupLog.V(1).Info("Update event filtered: operator update",
		"resource", fmt.Sprintf("%s/%s", e.ObjectNew.GetNamespace(), e.ObjectNew.GetName()),
		"kind", e.ObjectNew.GetObjectKind().GroupVersionKind().Kind,
		"manager", getMostRecentManager(managedFields))
	return false
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

	// Load configuration from environment using the extracted helper
	cfg, err := loadSentinelConfig()
	if err != nil {
		setupLog.Error(err, "Failed to load configuration")
		os.Exit(1)
	}

	setupLog.Info("Starting Sentinel",
		"cluster_namespace", cfg.clusterNamespace,
		"cluster_name", cfg.clusterName,
		"debounce_window", cfg.debounceWindow,
		"debounce_base", cfg.debounceWindowBase,
		"debounce_jitter_range", cfg.debounceJitterRange,
		"debounce_jitter", cfg.debounceWindowJitter)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       fmt.Sprintf("openbao-sentinel-%s-%s", cfg.clusterNamespace, cfg.clusterName),
		// Restrict cache to the cluster namespace to align with namespace-scoped RBAC permissions.
		// This prevents the cache from requiring cluster-wide list/watch permissions.
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.clusterNamespace: {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	reconciler := &SentinelReconciler{
		client: mgr.GetClient(),
		logger: ctrl.Log.WithName("sentinel").WithValues(
			"cluster_namespace", cfg.clusterNamespace,
			"cluster_name", cfg.clusterName,
			"component", openbaocluster.ComponentSentinel,
		),
		clusterName:      cfg.clusterName,
		clusterNamespace: cfg.clusterNamespace,
		debounceWindow:   cfg.debounceWindow,
		pendingTrigger:   &pendingTrigger{},
		healthStatus:     &healthStatus{},
	}

	// Create a single controller that watches multiple resource types
	// Use StatefulSet as the primary resource and add watches for others
	// Use the extracted drift predicate builder
	driftPredicate := buildDriftPredicate(cfg.clusterName)

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
