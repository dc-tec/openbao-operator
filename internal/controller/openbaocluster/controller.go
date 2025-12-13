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

package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	backupmanager "github.com/openbao/operator/internal/backup"
	certmanager "github.com/openbao/operator/internal/certs"
	controllermetrics "github.com/openbao/operator/internal/controller"
	inframanager "github.com/openbao/operator/internal/infra"
	initmanager "github.com/openbao/operator/internal/init"
	openbaolabels "github.com/openbao/operator/internal/openbao"
	security "github.com/openbao/operator/internal/security"
	upgrademanager "github.com/openbao/operator/internal/upgrade"
)

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	TLSReload         certmanager.ReloadSignaler
	InitManager       *initmanager.Manager
	OperatorNamespace string
	OIDCIssuer        string // OIDC issuer URL discovered at startup
	OIDCJWTKeys       []string
}

// SECURITY: RBAC is manually maintained via namespace-scoped Roles created by the Provisioner.
// We do NOT use kubebuilder annotations because:
// 1. The controller has NO cluster-wide permissions - it only uses namespace-scoped Roles
// 2. Permissions are granted dynamically per-namespace by the Provisioner in tenant namespaces
// 3. The controller receives permissions via RoleBindings created by the Provisioner in each tenant namespace
// 4. This ensures the controller cannot access resources outside of provisioned tenant namespaces
// The tenant Role (openbao-operator-tenant-role) includes permissions for:
// - OpenBaoCluster CRUD operations (namespace-scoped)
// - Workload resources (StatefulSets, Secrets, Services, etc.) in tenant namespaces only
// - Events (namespace-scoped)

// Reconcile is part of the main Kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *OpenBaoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileMetrics := controllermetrics.NewReconcileMetrics(req.Namespace, req.Name, "openbaocluster")
	startTime := time.Now()
	var reconcileErr error
	defer func() {
		durationSeconds := time.Since(startTime).Seconds()
		reconcileMetrics.ObserveDuration(durationSeconds)
		if reconcileErr != nil {
			// Use a low-cardinality reason for now; additional classification
			// can be added later without changing the metric shape.
			reconcileMetrics.IncrementError("Error")
		}
	}()

	baseLogger := log.FromContext(ctx)
	logger := baseLogger.WithValues(
		"cluster_namespace", req.Namespace,
		"cluster_name", req.Name,
		"controller", "openbaocluster",
		"reconcile_id", time.Now().UnixNano(),
	)

	logger.Info("Reconciling OpenBaoCluster")

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("OpenBaoCluster resource not found; assuming it was deleted")
			return ctrl.Result{}, nil
		}

		reconcileErr = fmt.Errorf("failed to get OpenBaoCluster %s/%s: %w", req.Namespace, req.Name, err)
		return ctrl.Result{}, reconcileErr
	}

	if !cluster.DeletionTimestamp.IsZero() {
		logger.Info("OpenBaoCluster is marked for deletion")
		if containsFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
			if err := r.handleDeletion(ctx, logger, cluster); err != nil {
				reconcileErr = err
				return ctrl.Result{}, reconcileErr
			}

			cluster.Finalizers = removeFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
			if err := r.Update(ctx, cluster); err != nil {
				reconcileErr = fmt.Errorf("failed to remove finalizer from OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
				return ctrl.Result{}, reconcileErr
			}
		}

		return ctrl.Result{}, nil
	}

	if !containsFinalizer(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer) {
		cluster.Finalizers = append(cluster.Finalizers, openbaov1alpha1.OpenBaoClusterFinalizer)
		if err := r.Update(ctx, cluster); err != nil {
			reconcileErr = fmt.Errorf("failed to add finalizer to OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
			return ctrl.Result{}, reconcileErr
		}

		// Requeue to observe the resource with the finalizer attached.
		return ctrl.Result{}, nil
	}

	if cluster.Spec.Paused {
		logger.Info("Reconciliation is paused for OpenBaoCluster")
		if err := r.updateStatusForPaused(ctx, logger, cluster); err != nil {
			reconcileErr = err
			return ctrl.Result{}, reconcileErr
		}
		return ctrl.Result{}, nil
	}

	if err := r.reconcileCerts(ctx, logger, cluster); err != nil {
		now := metav1.Now()
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionTLSReady),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Error",
			Message:            fmt.Sprintf("failed to reconcile TLS assets: %v", err),
		})

		if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
			reconcileErr = fmt.Errorf("failed to update TLSReady condition for OpenBaoCluster %s/%s after TLS error: %w", cluster.Namespace, cluster.Name, statusErr)
			return ctrl.Result{}, reconcileErr
		}

		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Ready",
		Message:            "TLS assets are provisioned",
	})

	if err := r.reconcileInfra(ctx, logger, cluster); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	initResult, err := r.reconcileInit(ctx, logger, cluster)
	if err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	if err := r.reconcileUpgrade(ctx, logger, cluster); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	if err := r.reconcileBackup(ctx, logger, cluster); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	if err := r.updateStatus(ctx, logger, cluster); err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// If initialization is still in progress, requeue so that the init manager
	// can retry once the first pod is running and the API is reachable.
	if initResult.RequeueAfter > 0 {
		return initResult, nil
	}

	// Safety net: periodically requeue to detect and correct drift that may
	// have bypassed admission controls (for example, if the resource locking
	// webhook is temporarily disabled). The interval is long to minimize API
	// load and does not change the core reconciliation semantics.
	const safetyNetBase = 20 * time.Minute
	const safetyNetJitter = 5 * time.Minute

	jitterNanos := time.Now().UnixNano() % int64(safetyNetJitter)
	requeueAfter := safetyNetBase + time.Duration(jitterNanos)

	logger.V(1).Info("Reconciliation complete; scheduling safety net requeue", "requeueAfter", requeueAfter)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

func (r *OpenBaoClusterReconciler) reconcileCerts(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling TLS assets for OpenBaoCluster")
	manager := certmanager.NewManagerWithReloader(r.Client, r.Scheme, r.TLSReload)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) reconcileInfra(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")

	// Perform image verification if enabled and get verified digest
	var verifiedImageDigest string
	if cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled {
		digest, err := r.verifyImage(ctx, logger, cluster)
		if err != nil {
			now := metav1.Now()
			failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
			if failurePolicy == "" {
				failurePolicy = "Block" // Default to Block
			}

			if failurePolicy == "Block" {
				// Block reconciliation and set Degraded condition
				meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
					Type:               string(openbaov1alpha1.ConditionDegraded),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: cluster.Generation,
					LastTransitionTime: now,
					Reason:             "ImageVerificationFailed",
					Message:            fmt.Sprintf("Image verification failed: %v", err),
				})

				if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
					return fmt.Errorf("failed to update Degraded condition for OpenBaoCluster %s/%s after image verification error: %w", cluster.Namespace, cluster.Name, statusErr)
				}

				return fmt.Errorf("image verification failed (policy=Block): %w", err)
			}

			// Warn policy: log error and emit event, but proceed
			logger.Error(err, "Image verification failed but proceeding due to Warn policy", "image", cluster.Spec.Image)
			// TODO: Emit Kubernetes Event here
		} else {
			verifiedImageDigest = digest
			logger.Info("Image verified successfully, using digest", "digest", digest)
		}
	}

	manager := inframanager.NewManager(r.Client, r.Scheme, r.OperatorNamespace, r.OIDCIssuer, r.OIDCJWTKeys)
	if err := manager.Reconcile(ctx, logger, cluster, verifiedImageDigest); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			now := metav1.Now()
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             "GatewayAPIMissing",
				Message:            "Gateway API CRDs are not installed but spec.gateway.enabled is true; install Gateway API CRDs or disable spec.gateway to clear this condition.",
			})

			if statusErr := r.Status().Update(ctx, cluster); statusErr != nil {
				return fmt.Errorf("failed to update Degraded condition for OpenBaoCluster %s/%s after Gateway API error: %w", cluster.Namespace, cluster.Name, statusErr)
			}

			return nil
		}
		return err
	}

	return nil
}

// verifyImage verifies the container image signature using the ImageVerifier.
// Returns the verified image digest that should be used in the StatefulSet.
// Supports both static key verification (PublicKey) and keyless verification (Issuer + Subject).
func (r *OpenBaoClusterReconciler) verifyImage(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.ImageVerification == nil || !cluster.Spec.ImageVerification.Enabled {
		return "", nil
	}

	// Validate that either PublicKey OR (Issuer and Subject) are provided
	// This matches the validation in ImageVerifier.Verify()
	if cluster.Spec.ImageVerification.PublicKey == "" && (cluster.Spec.ImageVerification.Issuer == "" || cluster.Spec.ImageVerification.Subject == "") {
		return "", fmt.Errorf("image verification is enabled but neither public key nor keyless configuration (issuer and subject) is provided")
	}

	// ImageVerifier can optionally load trusted_root.json from a ConfigMap.
	// For now, we use nil to use the embedded version. In the future, this could
	// be configured via operator flags or environment variables.
	var trustedRootConfig *security.TrustedRootConfig = nil
	verifier := security.NewImageVerifier(logger, r.Client, trustedRootConfig)
	config := security.VerifyConfig{
		PublicKey:        cluster.Spec.ImageVerification.PublicKey,
		Issuer:           cluster.Spec.ImageVerification.Issuer,
		Subject:          cluster.Spec.ImageVerification.Subject,
		IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
		ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
		Namespace:        cluster.Namespace,
	}
	digest, err := verifier.Verify(ctx, cluster.Spec.Image, config)
	if err != nil {
		return "", err
	}

	return digest, nil
}

func (r *OpenBaoClusterReconciler) reconcileInit(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	if r.InitManager == nil {
		// InitManager is optional - if not set, skip initialization check
		return ctrl.Result{}, nil
	}

	// Skip initialization if cluster is already initialized
	if cluster.Status.Initialized {
		return ctrl.Result{}, nil
	}

	logger.Info("Reconciling initialization for OpenBaoCluster")
	if err := r.InitManager.Reconcile(ctx, logger, cluster); err != nil {
		return ctrl.Result{}, err
	}

	// If initialization is still in progress, requeue so that the init manager
	// can retry once the first pod is running and the API is reachable.
	if !cluster.Status.Initialized {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: 5 * time.Second,
		}, nil
	}

	return ctrl.Result{}, nil
}

func (r *OpenBaoClusterReconciler) reconcileUpgrade(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling upgrades for OpenBaoCluster")
	manager := upgrademanager.NewManager(r.Client, r.Scheme)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) reconcileBackup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	logger.Info("Reconciling backups for OpenBaoCluster")
	manager := backupmanager.NewManager(r.Client, r.Scheme)
	return manager.Reconcile(ctx, logger, cluster)
}

func (r *OpenBaoClusterReconciler) handleDeletion(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	// Clear per-cluster metrics to avoid leaving stale series after deletion.
	clusterMetrics := controllermetrics.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.Clear()

	infra := inframanager.NewManager(r.Client, r.Scheme, r.OperatorNamespace, r.OIDCIssuer, r.OIDCJWTKeys)
	if err := infra.Cleanup(ctx, logger, cluster, policy); err != nil {
		return err
	}

	// Backup deletion for DeletionPolicyDeleteAll will be implemented alongside the BackupManager
	// data path. For now, backups are left untouched even when DeleteAll is specified.

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForPaused(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	now := metav1.Now()

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Paused",
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Paused",
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Paused",
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	if err := r.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Fetch the StatefulSet to observe actual ready replicas
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}

	var readyReplicas int32
	var available bool
	err := r.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, cluster.Name, err)
		}
		// StatefulSet not found - cluster is initializing
		readyReplicas = 0
		available = false
	} else {
		readyReplicas = statefulSet.Status.ReadyReplicas
		desiredReplicas := cluster.Spec.Replicas
		available = readyReplicas == desiredReplicas && readyReplicas > 0
	}

	cluster.Status.ReadyReplicas = readyReplicas

	// Update per-cluster metrics for ready replicas and phase. Metrics are
	// derived from the status view and do not influence reconciliation logic.
	clusterMetrics := controllermetrics.NewClusterMetrics(cluster.Namespace, cluster.Name)
	clusterMetrics.SetReadyReplicas(readyReplicas)

	// Set initial CurrentVersion if empty (first reconcile after initialization)
	if cluster.Status.CurrentVersion == "" && cluster.Status.Initialized {
		cluster.Status.CurrentVersion = cluster.Spec.Version
		logger.Info("Set initial CurrentVersion", "version", cluster.Spec.Version)
	}

	// Only update phase if no upgrade is in progress
	// The upgrade manager controls phase during upgrades
	if cluster.Status.Upgrade == nil {
		if cluster.Status.Phase == "" {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		}

		// Update phase based on ready replicas (when not upgrading)
		if readyReplicas == 0 {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		} else if available {
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
		} else {
			// Some replicas are ready but not all
			cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
		}
	}

	clusterMetrics.SetPhase(cluster.Status.Phase)

	now := metav1.Now()

	podSelector := map[string]string{
		"app.kubernetes.io/instance":   cluster.Name,
		"app.kubernetes.io/name":       "openbao",
		"app.kubernetes.io/managed-by": "openbao-operator",
	}

	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelector),
	); err != nil {
		return fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	var leaderCount int
	var leaderName string
	var pod0 *corev1.Pod
	pod0Name := fmt.Sprintf("%s-0", cluster.Name)

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Name == pod0Name {
			pod0 = pod
		}

		active, present, err := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao active label value", "pod", pod.Name, "error", err)
			continue
		}
		if present && active {
			leaderCount++
			leaderName = pod.Name
		}
	}

	cluster.Status.ActiveLeader = leaderName

	if pod0 != nil {
		initialized, present, err := openbaolabels.ParseBoolLabel(pod0.Labels, openbaolabels.LabelInitialized)
		if err != nil {
			return fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelInitialized, pod0.Name, err)
		}
		if present {
			status := metav1.ConditionFalse
			reason := "NotInitialized"
			message := "OpenBao reports not initialized"
			if initialized {
				status = metav1.ConditionTrue
				reason = "Initialized"
				message = "OpenBao reports initialized"
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoInitialized),
				Status:             status,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			})
		} else {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoInitialized),
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             "Unknown",
				Message:            "OpenBao initialization state is not yet available via service registration",
			})
		}

		sealed, present, err := openbaolabels.ParseBoolLabel(pod0.Labels, openbaolabels.LabelSealed)
		if err != nil {
			return fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelSealed, pod0.Name, err)
		}
		if present {
			status := metav1.ConditionFalse
			reason := "Unsealed"
			message := "OpenBao reports unsealed"
			if sealed {
				status = metav1.ConditionTrue
				reason = "Sealed"
				message = "OpenBao reports sealed"
			}
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoSealed),
				Status:             status,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			})
		} else {
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionOpenBaoSealed),
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             "Unknown",
				Message:            "OpenBao seal state is not yet available via service registration",
			})
		}
	}

	switch leaderCount {
	case 0:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionUnknown,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "LeaderUnknown",
			Message:            "No active leader label observed on pods",
		})
	case 1:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "LeaderFound",
			Message:            fmt.Sprintf("Leader is %s", leaderName),
		})
	default:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "MultipleLeaders",
			Message:            fmt.Sprintf("Multiple leaders detected via pod labels (%d)", leaderCount),
		})
	}

	availableStatus := metav1.ConditionFalse
	availableReason := "NotReady"
	availableMessage := fmt.Sprintf("Only %d/%d replicas are ready", readyReplicas, cluster.Spec.Replicas)
	if available {
		availableStatus = metav1.ConditionTrue
		availableReason = "AllReplicasReady"
		availableMessage = fmt.Sprintf("All %d replicas are ready", readyReplicas)
	} else if readyReplicas == 0 {
		availableStatus = metav1.ConditionFalse
		availableReason = "NoReplicasReady"
		availableMessage = "No replicas are ready yet"
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             availableStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             availableReason,
		Message:            availableMessage,
	})

	// Only set Degraded=False if not already set by another component (e.g., upgrade manager)
	degradedCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond == nil || (degradedCond.Status != metav1.ConditionTrue) {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Reconciling",
			Message:            "No degradation has been recorded by the controller",
		})
	}

	// Only set Upgrading=False if no upgrade is in progress
	// The upgrade manager manages this condition during upgrades
	if cluster.Status.Upgrade == nil {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionUpgrading),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "Idle",
			Message:            "No upgrade is currently in progress",
		})
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackingUp),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "Idle",
		Message:            "No backup is currently in progress",
	})

	// Set etcd encryption warning condition.
	// The operator cannot verify etcd encryption status, but it should warn users
	// that security relies on underlying K8s secret encryption at rest.
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             "EtcdEncryptionUnknown",
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	// Set SecurityRisk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionSecurityRisk),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "DevelopmentProfile",
			Message:            "Cluster is using Development profile with relaxed security. Not suitable for production.",
		})
	} else {
		// Remove condition if profile is Hardened or not set
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// SECURITY: Warn when SelfInit is disabled - the operator will store the root token
	// In a Zero Trust model, the operator should not possess the "Keys to the Kingdom"
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             "RootTokenStored",
			Message: "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. " +
				"Anyone with Secret read access in this namespace can access the root token. " +
				"Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments.",
		})
		logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name,
			"secret_name", cluster.Name+"-root-token")
	}

	if err := r.Status().Update(ctx, cluster); err != nil {
		return fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster", "readyReplicas", readyReplicas, "phase", cluster.Status.Phase, "currentVersion", cluster.Status.CurrentVersion)

	return nil
}

func containsFinalizer(finalizers []string, value string) bool {
	for _, f := range finalizers {
		if f == value {
			return true
		}
	}
	return false
}

func removeFinalizer(finalizers []string, value string) []string {
	result := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f == value {
			continue
		}
		result = append(result, f)
	}
	return result
}

// SetupWithManager sets up the controller with the Manager.
// It registers watches on the OpenBaoCluster CR and all owned resources (StatefulSet,
// Services, ConfigMaps, Secrets, ServiceAccounts, and Ingresses) so that changes
// to child resources trigger reconciliation of the parent OpenBaoCluster.
func (r *OpenBaoClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// SECURITY: To preserve the zero-trust model, the controller does not
	// register ownership watches (Owns) for child resources like StatefulSets,
	// Services, ConfigMaps, Jobs, Secrets, ServiceAccounts, or Ingresses. Ownership
	// watches require cluster-wide list/watch permissions for those resource
	// types, which conflicts with the design where the controller only has
	// namespace-scoped permissions via tenant Roles. Instead, the controller
	// reconciles child resources when the OpenBaoCluster itself changes or via
	// explicit requeues in the reconciliation logic.
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&openbaov1alpha1.OpenBaoCluster{})

	return builder.
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter:             workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
		}).
		Named("openbaocluster").
		Complete(r)
}
