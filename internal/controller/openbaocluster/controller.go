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
	"strings"
	"time"

	"github.com/go-logr/logr"
	"golang.org/x/time/rate"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/admission"
	backupmanager "github.com/openbao/operator/internal/backup"
	certmanager "github.com/openbao/operator/internal/certs"
	"github.com/openbao/operator/internal/constants"
	controllerutil "github.com/openbao/operator/internal/controller"
	operatorerrors "github.com/openbao/operator/internal/errors"
	inframanager "github.com/openbao/operator/internal/infra"
	initmanager "github.com/openbao/operator/internal/init"
	openbaolabels "github.com/openbao/operator/internal/openbao"
	"github.com/openbao/operator/internal/revision"
	security "github.com/openbao/operator/internal/security"
	upgrademanager "github.com/openbao/operator/internal/upgrade"
	"github.com/openbao/operator/internal/upgrade/bluegreen"
)

// failurePolicyBlock is the default failure policy for image verification.
const failurePolicyBlock = "Block"

// unknownResource is used when the resource kind or info is not available.
const unknownResource = "unknown"

const unsealTypeStatic = "static"

// SubReconciler is a standardized interface for sub-reconcilers that handle
// specific aspects of OpenBaoCluster reconciliation.
// Implementations should return (shouldRequeue, error) where shouldRequeue
// indicates whether the reconciliation should be requeued immediately.
type SubReconciler interface {
	// Reconcile performs reconciliation for the given cluster.
	// Returns (shouldRequeue, error) where shouldRequeue indicates if
	// reconciliation should be requeued immediately.
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error)
}

// infraReconciler wraps InfraManager to implement SubReconciler interface.
// It handles image verification and injects the verified digest into InfraManager.
type infraReconciler struct {
	client            client.Client
	scheme            *runtime.Scheme
	operatorNamespace string
	oidcIssuer        string
	oidcJWTKeys       []string
	verifyImageFunc   func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error)
	recorder          record.EventRecorder
	admissionStatus   *admission.Status
}

// Reconcile implements SubReconciler for infrastructure reconciliation.
func (r *infraReconciler) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (bool, error) {
	logger.Info("Reconciling infrastructure for OpenBaoCluster")

	// Perform image verification if enabled and get verified digest
	var verifiedImageDigest string
	if cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled {
		// Use a strict timeout for image verification to prevent blocking the reconcile loop.
		// Network I/O to OCI registries/Rekor can hang, which would stall the worker.
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		digest, err := r.verifyImageFunc(verifyCtx, logger, cluster)
		if err != nil {
			now := metav1.Now()
			failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
			if failurePolicy == "" {
				failurePolicy = failurePolicyBlock // Default to Block
			}

			if failurePolicy == failurePolicyBlock {
				// Block reconciliation and set Degraded condition
				meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
					Type:               string(openbaov1alpha1.ConditionDegraded),
					Status:             metav1.ConditionTrue,
					ObservedGeneration: cluster.Generation,
					LastTransitionTime: now,
					Reason:             ReasonImageVerificationFailed,
					Message:            fmt.Sprintf("Image verification failed: %v", err),
				})

				// Note: Status update will be handled by the controller
				return false, fmt.Errorf("image verification failed (policy=Block): %w", err)
			}

			// Warn policy: log error and emit event, but proceed
			logger.Error(err, "Image verification failed but proceeding due to Warn policy", "image", cluster.Spec.Image)
			if r.recorder != nil {
				r.recorder.Eventf(cluster, corev1.EventTypeWarning, ReasonImageVerificationFailed, "Image verification failed but proceeding due to Warn policy: %v", err)
			}
		} else {
			verifiedImageDigest = digest
			logger.Info("Image verified successfully, using digest", "digest", digest)
		}
	}

	var verifiedSentinelDigest string
	if cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled {
		sentinelImage := cluster.Spec.Sentinel.Image
		if sentinelImage == "" {
			// Derive from operator version (handled in getSentinelImage)
			sentinelImage = "openbao/operator-sentinel:latest" // Will be resolved in infra manager
		}

		// Reuse the image verification logic if enabled
		if cluster.Spec.ImageVerification != nil && cluster.Spec.ImageVerification.Enabled {
			var trustedRootConfig *security.TrustedRootConfig = nil
			verifier := security.NewImageVerifier(logger, r.client, trustedRootConfig)
			config := security.VerifyConfig{
				PublicKey:        cluster.Spec.ImageVerification.PublicKey,
				Issuer:           cluster.Spec.ImageVerification.Issuer,
				Subject:          cluster.Spec.ImageVerification.Subject,
				IgnoreTlog:       cluster.Spec.ImageVerification.IgnoreTlog,
				ImagePullSecrets: cluster.Spec.ImageVerification.ImagePullSecrets,
				Namespace:        cluster.Namespace,
			}
			// Use a strict timeout for image verification to prevent blocking the reconcile loop.
			verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			digest, err := verifier.Verify(verifyCtx, sentinelImage, config)
			if err != nil {
				// Handle error based on failure policy
				failurePolicy := cluster.Spec.ImageVerification.FailurePolicy
				if failurePolicy == "" {
					failurePolicy = failurePolicyBlock // Default to Block
				}
				if failurePolicy == failurePolicyBlock {
					now := metav1.Now()
					meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
						Type:               string(openbaov1alpha1.ConditionDegraded),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: cluster.Generation,
						LastTransitionTime: now,
						Reason:             ReasonSentinelImageVerificationFailed,
						Message:            fmt.Sprintf("Sentinel image verification failed: %v", err),
					})
					return false, fmt.Errorf("sentinel image verification failed (policy=Block): %w", err)
				}
				// Warn policy: log error but proceed
				logger.Error(err, "Sentinel image verification failed but proceeding due to Warn policy", "image", sentinelImage)
			} else {
				verifiedSentinelDigest = digest
				logger.Info("Sentinel image verified successfully", "digest", digest)
			}
		}
	}
	// --------------------------------

	sentinelAdmissionReady := r.admissionStatus == nil || r.admissionStatus.SentinelReady
	sentinelEnabled := cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Enabled

	if sentinelEnabled && !sentinelAdmissionReady {
		now := metav1.Now()
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonAdmissionPoliciesNotReady,
			Message:            "Sentinel is enabled but required admission policies are missing or misbound; Sentinel will not be deployed. " + r.admissionStatus.SummaryMessage(),
		})
		if r.recorder != nil {
			r.recorder.Eventf(cluster, corev1.EventTypeWarning, ReasonAdmissionPoliciesNotReady,
				"Sentinel is enabled but admission policies are not ready; Sentinel will be disabled: %s", r.admissionStatus.SummaryMessage())
		}
	} else if sentinelEnabled && sentinelAdmissionReady {
		degradedCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
		if degradedCond != nil && degradedCond.Reason == ReasonAdmissionPoliciesNotReady {
			now := metav1.Now()
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonAdmissionPoliciesReady,
				Message:            "Admission policies are ready; Sentinel can be deployed safely",
			})
		}
	}

	manager := inframanager.NewManagerWithSentinelAdmission(r.client, r.scheme, r.operatorNamespace, r.oidcIssuer, r.oidcJWTKeys, sentinelAdmissionReady)
	if err := manager.Reconcile(ctx, logger, cluster, verifiedImageDigest, verifiedSentinelDigest); err != nil {
		if errors.Is(err, inframanager.ErrGatewayAPIMissing) {
			now := metav1.Now()
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonGatewayAPIMissing,
				Message:            "Gateway API CRDs are not installed but spec.gateway.enabled is true; install Gateway API CRDs or disable spec.gateway to clear this condition.",
			})

			// Note: Status update will be handled by the controller
			return false, nil
		}
		if errors.Is(err, inframanager.ErrStatefulSetPrerequisitesMissing) {
			// Prerequisites are missing (ConfigMap or TLS Secret). Set a condition and requeue
			// so reconciliation can retry once prerequisites are ready.
			now := metav1.Now()
			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionDegraded),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             ReasonPrerequisitesMissing,
				Message:            fmt.Sprintf("StatefulSet prerequisites missing: %v. Waiting for ConfigMap or TLS Secret to be created.", err),
			})

			// Note: Status update will be handled by the controller
			// Requeue with a short delay to retry once prerequisites are ready
			logger.Info("StatefulSet prerequisites missing; requeuing reconciliation", "error", err)
			return true, nil // Request immediate requeue
		}
		return false, err
	}

	// Prerequisites are ready and infrastructure reconciliation succeeded.
	// Clear any PrerequisitesMissing condition that might have been set earlier.
	degradedCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
	if degradedCond != nil && degradedCond.Reason == "PrerequisitesMissing" {
		now := metav1.Now()
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonPrerequisitesReady,
			Message:            "StatefulSet prerequisites are now available",
		})
		logger.Info("StatefulSet prerequisites are ready; cleared PrerequisitesMissing condition")
	}

	return false, nil
}

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	TLSReload         certmanager.ReloadSignaler
	InitManager       *initmanager.Manager
	OperatorNamespace string
	OIDCIssuer        string // OIDC issuer URL discovered at startup
	OIDCJWTKeys       []string
	AdmissionStatus   *admission.Status
	Recorder          record.EventRecorder
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
//
// complex status persistence, error classification, and sentinel trigger handling.
// Refactoring would fragment the control flow without reducing actual complexity.
//
//nolint:gocyclo // Orchestrator function coordinating multiple sub-reconcilers with
func (r *OpenBaoClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileMetrics := controllerutil.NewReconcileMetrics(req.Namespace, req.Name, constants.ControllerNameOpenBaoCluster)
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
		"controller", constants.ControllerNameOpenBaoCluster,
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

	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

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

	// Emit periodic warning events for insecure or invalid configurations.
	// Best-effort only: failures must not block reconciliation.
	if err := r.emitSecurityWarningEvents(ctx, logger, cluster); err != nil {
		logger.Error(err, "Failed to emit security warning events")
	}

	// Fail closed on implicit defaults: require an explicit Profile selection.
	// This prevents accidental production-by-default behavior.
	if cluster.Spec.Profile == "" {
		if err := r.updateStatusForProfileNotSet(ctx, logger, cluster); err != nil {
			reconcileErr = err
			return ctrl.Result{}, reconcileErr
		}

		jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
		requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Detect if this is a "Sentinel Trigger" event
	// The Sentinel updates the annotation: openbao.org/sentinel-trigger: <timestamp>
	// If the annotation exists, it means the Sentinel detected drift and triggered reconciliation
	isSentinelTrigger := false
	sentinelFastPathAllowed := r.AdmissionStatus == nil || r.AdmissionStatus.SentinelReady
	triggerAnnotation := constants.AnnotationSentinelTrigger
	sentinelResourceInfo := unknownResource // Captured for drift status update after reconciliation

	if val, ok := cluster.Annotations[triggerAnnotation]; ok && val != "" {
		if sentinelFastPathAllowed {
			isSentinelTrigger = true
		} else {
			logger.Info("Sentinel trigger annotation present, but Sentinel fast-path is disabled because admission policies are not ready",
				"trigger_timestamp", val,
				"annotation", triggerAnnotation,
			)
		}

		// Extract resource info from annotation if available
		if resourceVal, hasResource := cluster.Annotations[constants.AnnotationSentinelTriggerResource]; hasResource {
			sentinelResourceInfo = resourceVal
		}

		logger.Info("Sentinel trigger detected; entering Fast Path (Drift Correction)",
			"trigger_timestamp", val,
			"resource", sentinelResourceInfo)

		// Record metrics for drift detection (status update happens after updateStatus())
		resourceKind := unknownResource
		if sentinelResourceInfo != "" && sentinelResourceInfo != unknownResource {
			parts := strings.Split(sentinelResourceInfo, "/")
			if len(parts) > 0 {
				resourceKind = parts[0]
			}
		}
		clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
		clusterMetrics.RecordDriftDetected(resourceKind)
		clusterMetrics.SetDriftLastDetectedTimestamp(float64(time.Now().Unix()))
	}

	// Build sub-reconcilers in order of execution
	reconcilers := []SubReconciler{
		certmanager.NewManagerWithReloader(r.Client, r.Scheme, r.TLSReload),
		&infraReconciler{
			client:            r.Client,
			scheme:            r.Scheme,
			operatorNamespace: r.OperatorNamespace,
			oidcIssuer:        r.OIDCIssuer,
			oidcJWTKeys:       r.OIDCJWTKeys,
			verifyImageFunc:   r.verifyImage,
			recorder:          r.Recorder,
			admissionStatus:   r.AdmissionStatus,
		},
	}

	// Add InitManager if configured
	if r.InitManager != nil {
		reconcilers = append(reconcilers, r.InitManager)
	}

	// Conditional Execution: Only run expensive managers if we are NOT in Fast Path mode
	// SECURITY: Rate limit Sentinel triggers to prevent DoS of admin operations (backups/upgrades)
	// Force full reconciliation if:
	// 1. Too many consecutive fast paths (exceeds SentinelMaxConsecutiveFastPaths), OR
	// 2. Too much time since last full reconcile (exceeds SentinelForceFullReconcileInterval)
	forceFullReconcile := false
	if isSentinelTrigger {
		// Initialize drift status if needed
		if cluster.Status.Drift == nil {
			cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
		}

		// Check if we should force a full reconcile
		consecutiveFastPaths := cluster.Status.Drift.ConsecutiveFastPaths
		lastFullReconcile := cluster.Status.Drift.LastFullReconcileTime

		if consecutiveFastPaths >= constants.SentinelMaxConsecutiveFastPaths {
			logger.Info("Forcing full reconcile due to consecutive fast paths limit",
				"consecutiveFastPaths", consecutiveFastPaths,
				"maxAllowed", constants.SentinelMaxConsecutiveFastPaths)
			forceFullReconcile = true
		} else if lastFullReconcile != nil {
			timeSinceFullReconcile := time.Since(lastFullReconcile.Time)
			if timeSinceFullReconcile >= constants.SentinelForceFullReconcileInterval {
				logger.Info("Forcing full reconcile due to time since last full reconcile",
					"timeSinceFullReconcile", timeSinceFullReconcile,
					"forceInterval", constants.SentinelForceFullReconcileInterval)
				forceFullReconcile = true
			}
		}
	}

	if !isSentinelTrigger || forceFullReconcile {
		// Choose upgrade strategy: BlueGreen or RollingUpdate
		if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
			// Use bluegreen.Manager for blue/green upgrades
			infraMgr := inframanager.NewManagerWithSentinelAdmission(r.Client, r.Scheme, r.OperatorNamespace, r.OIDCIssuer, r.OIDCJWTKeys, sentinelFastPathAllowed)
			reconcilers = append(reconcilers,
				bluegreen.NewManager(r.Client, r.Scheme, infraMgr),
				backupmanager.NewManager(r.Client, r.Scheme),
			)
		} else {
			// Use UpgradeManager for rolling updates (default)
			reconcilers = append(reconcilers,
				upgrademanager.NewManager(r.Client, r.Scheme),
				backupmanager.NewManager(r.Client, r.Scheme),
			)
		}

		// Track full reconcile for rate limiting
		if cluster.Status.Drift == nil {
			cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
		}
		now := metav1.Now()
		cluster.Status.Drift.LastFullReconcileTime = &now
		cluster.Status.Drift.ConsecutiveFastPaths = 0

		if forceFullReconcile {
			logger.Info("Reset consecutive fast paths counter after forced full reconcile")
		}
	} else {
		// Fast path - increment consecutive counter
		if cluster.Status.Drift == nil {
			cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
		}
		cluster.Status.Drift.ConsecutiveFastPaths++
		logger.Info("Skipping Upgrade and Backup managers due to Sentinel Trigger priority",
			"consecutiveFastPaths", cluster.Status.Drift.ConsecutiveFastPaths,
			"maxBeforeForce", constants.SentinelMaxConsecutiveFastPaths)
	}

	// Execute sub-reconcilers in order
	for _, rec := range reconcilers {
		// Capture state before reconciler runs to detect status changes
		statusBeforeReconcile := cluster.Status.DeepCopy()

		shouldRequeue, err := rec.Reconcile(ctx, logger, cluster)
		if err != nil {
			// Special handling for CertManager errors - set TLSReady condition
			if _, isCertManager := rec.(*certmanager.Manager); isCertManager {
				now := metav1.Now()
				meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
					Type:               string(openbaov1alpha1.ConditionTLSReady),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: cluster.Generation,
					LastTransitionTime: now,
					Reason:             constants.ReasonUnknown,
					Message:            fmt.Sprintf("failed to reconcile TLS assets: %v", err),
				})

				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					reconcileErr = fmt.Errorf("failed to update TLSReady condition for OpenBaoCluster %s/%s after TLS error: %w", cluster.Namespace, cluster.Name, statusErr)
					return ctrl.Result{}, reconcileErr
				}
			}

			// Handle structured errors: transient errors should requeue, permanent errors should not
			if operatorerrors.IsTransient(err) {
				shouldRequeue, requeueAfter := operatorerrors.ShouldRequeue(err)
				if shouldRequeue {
					logger.Info("Transient error encountered; will requeue", "error", err, "requeueAfter", requeueAfter)
					if requeueAfter > 0 {
						return ctrl.Result{RequeueAfter: requeueAfter}, nil
					}
					return ctrl.Result{Requeue: true}, nil
				}
			}

			// Permanent errors should not requeue - wait for user intervention
			if operatorerrors.IsPermanent(err) {
				logger.Info("Permanent error encountered; not requeuing", "error", err)
				reconcileErr = err
				return ctrl.Result{}, reconcileErr
			}

			// For unknown errors, default to returning the error (controller-runtime will handle backoff)
			reconcileErr = err
			return ctrl.Result{}, reconcileErr
		}

		// For BackupManager, persist status if it was initialized (even if preconditions fail)
		// The backup manager initializes Status.Backup before checking preconditions, so we need
		// to persist it even if the manager returns early due to failed preconditions.
		if _, isBackupManager := rec.(*backupmanager.Manager); isBackupManager {
			// Check if Status.Backup was initialized (was nil, now not nil)
			wasNil := statusBeforeReconcile.Backup == nil
			isInitialized := cluster.Status.Backup != nil
			if wasNil && isInitialized {
				// Status.Backup was just initialized - persist it immediately
				// This ensures the status is available even if preconditions fail
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					logger.Error(statusErr, "Failed to persist backup status initialization")
					// Don't fail the reconcile - status will be persisted on next reconcile
				} else {
					logger.V(1).Info("Persisted backup status initialization")
					// Update original for subsequent status patches
					original = cluster.DeepCopy()
				}
			}
		}

		// For bluegreen.Manager, persist status if it was initialized (even if no upgrade is needed),
		// OR if the phase changed (to ensure finalization changes like Phase=Idle are persisted).
		// bluegreen.Manager initializes Status.BlueGreen on the "no-op" path (PhaseIdle) which would
		// otherwise be lost because updateStatus() snapshots "original" after sub-reconcilers have run.
		if _, isBlueGreenManager := rec.(*bluegreen.Manager); isBlueGreenManager {
			wasNil := statusBeforeReconcile.BlueGreen == nil
			isInitialized := cluster.Status.BlueGreen != nil
			phaseChanged := false
			if statusBeforeReconcile.BlueGreen != nil && cluster.Status.BlueGreen != nil {
				phaseChanged = statusBeforeReconcile.BlueGreen.Phase != cluster.Status.BlueGreen.Phase
			}
			versionChanged := statusBeforeReconcile.CurrentVersion != cluster.Status.CurrentVersion
			breakGlassChanged := breakGlassStatusChanged(statusBeforeReconcile.BreakGlass, cluster.Status.BreakGlass)

			if (wasNil && isInitialized) || phaseChanged || versionChanged || breakGlassChanged {
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					logger.Error(statusErr, "Failed to persist blue/green status changes")
				} else {
					if phaseChanged || versionChanged || breakGlassChanged {
						logger.Info("Persisted blue/green status changes",
							"phaseChanged", phaseChanged,
							"versionChanged", versionChanged,
							"breakGlassChanged", breakGlassChanged,
							"newPhase", cluster.Status.BlueGreen.Phase,
							"newVersion", cluster.Status.CurrentVersion)
					} else {
						logger.V(1).Info("Persisted blue/green status initialization")
					}
					original = cluster.DeepCopy()
				}
			}
		}

		// If a reconciler requests requeue, stop the chain and return
		if shouldRequeue {
			// For infra reconciler, handle special requeue cases
			if _, isInfraReconciler := rec.(*infraReconciler); isInfraReconciler {
				// Update status before requeuing
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					reconcileErr = fmt.Errorf("failed to update status before requeue: %w", statusErr)
					return ctrl.Result{}, reconcileErr
				}
				return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
			}
			// For InitManager, update status before requeuing so Initialized status is persisted
			// This ensures InfraReconciler sees the updated status on the next reconcile
			if _, isInitManager := rec.(*initmanager.Manager); isInitManager {
				// Update status before requeuing so Initialized=true is persisted
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					reconcileErr = fmt.Errorf("failed to update status before requeue: %w", statusErr)
					return ctrl.Result{}, reconcileErr
				}
				return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
			}
			// For BackupManager, requeue with a short delay to check if backup is due
			// This ensures timely backup execution even with frequent schedules
			if _, isBackupManager := rec.(*backupmanager.Manager); isBackupManager {
				// Update status before requeuing to persist any status changes
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					reconcileErr = fmt.Errorf("failed to update status before requeue: %w", statusErr)
					return ctrl.Result{}, reconcileErr
				}
				return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
			}
			// For bluegreen.Manager, update status before requeuing
			if _, isBlueGreenManager := rec.(*bluegreen.Manager); isBlueGreenManager {
				if statusErr := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); statusErr != nil {
					reconcileErr = fmt.Errorf("failed to update status before requeue: %w", statusErr)
					return ctrl.Result{}, reconcileErr
				}
				return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Set TLSReady condition to true after successful cert reconciliation
	// (CertManager is the first reconciler)
	now := metav1.Now()
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonReady,
		Message:            "TLS assets are provisioned",
	})

	statusUpdateResult, err := r.updateStatus(ctx, logger, cluster)
	if err != nil {
		reconcileErr = err
		return ctrl.Result{}, reconcileErr
	}

	// After successful reconciliation, clear the Sentinel trigger annotation
	// to prevent re-triggering on the next reconcile loop
	// NOTE: This will cause a second reconciliation (Normal Path) which is DESIRABLE:
	// 1. Fast Path fixes the immediate drift
	// 2. Normal Path (triggered by annotation deletion) acts as a safety check
	//    to ensure full consistency including backups/upgrades now that emergency is over
	// Do NOT optimize this away - it adds safety.
	// Clear the annotation even if we're requeuing, as the fast path reconciliation has completed.
	if isSentinelTrigger {
		// Record ALL drift fields together (detection + correction) so they're persisted atomically
		// This happens AFTER updateStatus() to avoid being overwritten by that status patch
		now := metav1.Now()
		if cluster.Status.Drift == nil {
			cluster.Status.Drift = &openbaov1alpha1.DriftStatus{}
		}
		cluster.Status.Drift.LastDriftDetected = &now
		cluster.Status.Drift.DriftCorrectionCount++
		cluster.Status.Drift.LastDriftResource = sentinelResourceInfo
		cluster.Status.Drift.LastCorrectionTime = &now

		// Record metrics for drift correction
		clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
		clusterMetrics.RecordDriftCorrected()

		// Update status to persist drift tracking
		if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
			logger.Error(err, "Failed to update drift status after correction")
			// Non-fatal: status will be updated on next reconcile
		}

		original := cluster.DeepCopy()
		if cluster.Annotations != nil {
			delete(cluster.Annotations, triggerAnnotation)
			// Also clear the resource annotation
			delete(cluster.Annotations, constants.AnnotationSentinelTriggerResource)
			if err := r.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
				logger.Error(err, "Failed to clear Sentinel trigger annotation")
				// Non-fatal: annotation will be cleared on next successful reconcile
			} else {
				logger.Info("Cleared Sentinel trigger annotation after successful reconciliation",
					"drift_correction_count", cluster.Status.Drift.DriftCorrectionCount)
			}
		}
	}

	// If status update indicates we should requeue (e.g., waiting for replicas to become ready),
	// return that result to trigger a timely reconciliation.
	if statusUpdateResult.RequeueAfter > 0 {
		return statusUpdateResult, nil
	}

	// If initialization is still in progress, requeue so that the init manager
	// can retry once the first pod is running and the API is reachable.
	if r.InitManager != nil && !cluster.Status.Initialized {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: constants.RequeueShort,
		}, nil
	}

	// Safety net: periodically requeue to detect and correct drift that may
	// have bypassed admission control. The interval is long to minimize API
	// load and does not change the core reconciliation semantics.
	jitterNanos := time.Now().UnixNano() % int64(constants.RequeueSafetyNetJitter)
	requeueAfter := constants.RequeueSafetyNetBase + time.Duration(jitterNanos)

	logger.V(1).Info("Reconciliation complete; scheduling safety net requeue", "requeueAfter", requeueAfter)

	return ctrl.Result{RequeueAfter: requeueAfter}, nil
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

func (r *OpenBaoClusterReconciler) handleDeletion(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	policy := cluster.Spec.DeletionPolicy
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger.Info("Applying DeletionPolicy for OpenBaoCluster", "deletionPolicy", string(policy))

	// Clear per-cluster metrics to avoid leaving stale series after deletion.
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
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
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	now := metav1.Now()

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Reconciliation is paused; availability is not being evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "Cluster is paused; no new degradation has been evaluated",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonPaused,
		Message:            "TLS readiness is not being evaluated while reconciliation is paused",
	})

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for paused OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for paused OpenBaoCluster")

	return nil
}

func (r *OpenBaoClusterReconciler) updateStatusForProfileNotSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	original := cluster.DeepCopy()

	now := metav1.Now()
	if cluster.Status.Phase == "" {
		cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionAvailable),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile must be explicitly set to Hardened or Development; reconciliation is blocked until set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionDegraded),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "spec.profile is not set; defaults may be inappropriate for production and could lead to insecure deployment",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionTLSReady),
		Status:             metav1.ConditionUnknown,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "TLS readiness is not being evaluated until spec.profile is set",
	})

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             ReasonProfileNotSet,
		Message:            "Cluster cannot be considered production-ready until spec.profile is explicitly set",
	})

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update status for missing profile on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster missing profile")
	return nil
}

//nolint:gocyclo // Status computation is intentionally explicit to keep reconciliation readable and auditable.
func (r *OpenBaoClusterReconciler) updateStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (ctrl.Result, error) {
	// Capture original state for status patching to avoid optimistic locking conflicts
	original := cluster.DeepCopy()

	// Fetch the StatefulSet to get ready replicas count
	statefulSet := &appsv1.StatefulSet{}
	statefulSetName := types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	activeRevision := ""
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
		activeRevision = revision.OpenBaoClusterRevision(cluster.Spec.Version, cluster.Spec.Image, cluster.Spec.Replicas)
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			activeRevision = cluster.Status.BlueGreen.BlueRevision
			if cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseDemotingBlue ||
				cluster.Status.BlueGreen.Phase == openbaov1alpha1.PhaseCleanup {
				if cluster.Status.BlueGreen.GreenRevision != "" {
					activeRevision = cluster.Status.BlueGreen.GreenRevision
				}
			}
		}
		statefulSetName.Name = fmt.Sprintf("%s-%s", cluster.Name, activeRevision)
	}

	var readyReplicas int32
	var available bool
	err := r.Get(ctx, statefulSetName, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to get StatefulSet %s/%s for status update: %w", cluster.Namespace, cluster.Name, err)
		}
		// StatefulSet not found - cluster is initializing
		readyReplicas = 0
		available = false
	} else {
		readyReplicas = statefulSet.Status.ReadyReplicas
		desiredReplicas := cluster.Spec.Replicas
		available = readyReplicas == desiredReplicas && readyReplicas > 0

		// Check if StatefulSet controller is still processing (hasn't caught up with spec changes)
		// or if status fields are potentially stale (pods may be ready but status not updated yet)
		statefulSetStillScaling := statefulSet.Status.ObservedGeneration < statefulSet.Generation ||
			statefulSet.Status.Replicas < desiredReplicas

		// Even when StatefulSet controller has caught up, ReadyReplicas might be stale
		// if pods are ready but the status update hasn't propagated yet.
		// If we have the desired number of replicas but ReadyReplicas is less,
		// we should requeue to catch the status update.
		statusPotentiallyStale := statefulSet.Status.Replicas == desiredReplicas &&
			statefulSet.Status.ReadyReplicas < statefulSet.Status.Replicas &&
			statefulSet.Status.ReadyReplicas < desiredReplicas

		logger.Info("StatefulSet status read for ReadyReplicas calculation",
			"statefulSetReadyReplicas", statefulSet.Status.ReadyReplicas,
			"statefulSetReplicas", statefulSet.Status.Replicas,
			"statefulSetCurrentReplicas", statefulSet.Status.CurrentReplicas,
			"statefulSetUpdatedReplicas", statefulSet.Status.UpdatedReplicas,
			"statefulSetObservedGeneration", statefulSet.Status.ObservedGeneration,
			"statefulSetGeneration", statefulSet.Generation,
			"desiredReplicas", desiredReplicas,
			"calculatedReadyReplicas", readyReplicas,
			"available", available,
			"statefulSetStillScaling", statefulSetStillScaling,
			"statusPotentiallyStale", statusPotentiallyStale)

		// If StatefulSet is still scaling or status is potentially stale, set phase and persist status before requeue
		if statefulSetStillScaling || statusPotentiallyStale {
			logger.V(1).Info("StatefulSet status may be stale; setting phase and requeuing to check status",
				"observedGeneration", statefulSet.Status.ObservedGeneration,
				"generation", statefulSet.Generation,
				"currentReplicas", statefulSet.Status.Replicas,
				"readyReplicas", statefulSet.Status.ReadyReplicas,
				"desiredReplicas", desiredReplicas,
				"stillScaling", statefulSetStillScaling,
				"statusStale", statusPotentiallyStale)

			// Set phase and ready replicas before requeuing to ensure status is persisted
			cluster.Status.ReadyReplicas = readyReplicas

			// Update per-cluster metrics for ready replicas
			clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
			clusterMetrics.SetReadyReplicas(readyReplicas)

			// Only update phase if no upgrade is in progress
			if cluster.Status.Upgrade == nil {
				if cluster.Status.Phase == "" {
					cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
				} else if readyReplicas == 0 {
					cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
				} else if !available {
					// Some replicas are ready but not all
					cluster.Status.Phase = openbaov1alpha1.ClusterPhaseInitializing
				}
			}

			clusterMetrics.SetPhase(cluster.Status.Phase)

			// Set basic conditions before requeuing to ensure they're persisted
			now := metav1.Now()
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

			// Only set Degraded=False if not already set by another component
			degradedCond := meta.FindStatusCondition(cluster.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			if degradedCond == nil || (degradedCond.Status != metav1.ConditionTrue) {
				meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
					Type:               string(openbaov1alpha1.ConditionDegraded),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: cluster.Generation,
					LastTransitionTime: now,
					Reason:             constants.ReasonReconciling,
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
					Reason:             constants.ReasonIdle,
					Message:            "No upgrade is currently in progress",
				})
			}

			meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
				Type:               string(openbaov1alpha1.ConditionBackingUp),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: cluster.Generation,
				LastTransitionTime: now,
				Reason:             constants.ReasonIdle,
				Message:            "No backup is currently in progress",
			})

			// Persist status before requeuing
			if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update status before requeue: %w", err)
			}

			return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
		}
	}

	cluster.Status.ReadyReplicas = readyReplicas

	// Update per-cluster metrics for ready replicas and phase. Metrics are
	// derived from the status view and do not influence reconciliation logic.
	clusterMetrics := controllerutil.NewClusterMetrics(cluster.Namespace, cluster.Name)
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
		constants.LabelAppInstance:  cluster.Name,
		constants.LabelAppName:      constants.LabelValueAppNameOpenBao,
		constants.LabelAppManagedBy: constants.LabelValueAppManagedByOpenBaoOperator,
	}
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen && activeRevision != "" {
		podSelector[constants.LabelOpenBaoRevision] = activeRevision
	}

	var pods corev1.PodList
	if err := r.List(ctx, &pods,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelector),
	); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	var leaderCount int
	var leaderName string
	var pod0 *corev1.Pod
	pod0Name := fmt.Sprintf("%s-0", cluster.Name)
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen && activeRevision != "" {
		pod0Name = fmt.Sprintf("%s-%s-0", cluster.Name, activeRevision)
	}

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
			return ctrl.Result{}, fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelInitialized, pod0.Name, err)
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
				Reason:             constants.ReasonUnknown,
				Message:            "OpenBao initialization state is not yet available via service registration",
			})
		}

		sealed, present, err := openbaolabels.ParseBoolLabel(pod0.Labels, openbaolabels.LabelSealed)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("invalid %q label on pod %s: %w", openbaolabels.LabelSealed, pod0.Name, err)
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
				Reason:             constants.ReasonUnknown,
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
			Reason:             ReasonLeaderUnknown,
			Message:            "No active leader label observed on pods",
		})
	case 1:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonLeaderFound,
			Message:            fmt.Sprintf("Leader is %s", leaderName),
		})
	default:
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionOpenBaoLeader),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonMultipleLeaders,
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
			Reason:             constants.ReasonReconciling,
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
			Reason:             constants.ReasonIdle,
			Message:            "No upgrade is currently in progress",
		})
	}

	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionBackingUp),
		Status:             metav1.ConditionFalse,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             constants.ReasonIdle,
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
		Reason:             ReasonEtcdEncryptionUnknown,
		Message:            "The operator cannot verify etcd encryption status. Ensure etcd encryption at rest is enabled in your Kubernetes cluster to protect Secrets (including unseal keys and root tokens) stored in etcd.",
	})

	// Set SecurityRisk condition for Development profile
	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionSecurityRisk),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonDevelopmentProfile,
			Message:            "Cluster is using Development profile with relaxed security. Not suitable for production.",
		})
	} else {
		// Remove condition if profile is Hardened or not set
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionSecurityRisk))
	}

	// Set ProductionReady condition based on security posture and bootstrap configuration.
	productionReadyStatus, productionReadyReason, productionReadyMessage := evaluateProductionReady(cluster)
	meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
		Type:               string(openbaov1alpha1.ConditionProductionReady),
		Status:             productionReadyStatus,
		ObservedGeneration: cluster.Generation,
		LastTransitionTime: now,
		Reason:             productionReadyReason,
		Message:            productionReadyMessage,
	})

	// SECURITY: Warn when SelfInit is disabled - the operator will store the root token
	// In a Zero Trust model, the operator should not possess the "Keys to the Kingdom"
	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		meta.SetStatusCondition(&cluster.Status.Conditions, metav1.Condition{
			Type:               string(openbaov1alpha1.ConditionDegraded),
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			Reason:             ReasonRootTokenStored,
			Message: "SelfInit is disabled. The operator is storing the root token in a Secret, which violates Zero Trust principles. " +
				"Anyone with Secret read access in this namespace can access the root token. " +
				"Strongly consider enabling SelfInit (spec.selfInit.enabled=true) for production deployments.",
		})
		logger.Info("SECURITY WARNING: SelfInit is disabled - root token will be stored in Secret",
			"cluster_namespace", cluster.Namespace,
			"cluster_name", cluster.Name,
			"secret_name", cluster.Name+"-root-token")
	}

	if err := r.Status().Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update status for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.Info("Updated status for OpenBaoCluster", "readyReplicas", readyReplicas, "phase", cluster.Status.Phase, "currentVersion", cluster.Status.CurrentVersion)

	// Requeue logic to ensure status updates promptly when replicas become ready.
	// Since we don't watch StatefulSets directly (for security reasons), we need to requeue
	// to catch when pods transition to ready state.
	previousReadyReplicas := original.Status.ReadyReplicas
	readyReplicasChanged := readyReplicas != previousReadyReplicas

	if !available && readyReplicas > 0 {
		// Not all replicas are ready yet - requeue to check again
		logger.V(1).Info("Not all replicas are ready; requeuing to check status", "readyReplicas", readyReplicas, "desiredReplicas", cluster.Spec.Replicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	if available && readyReplicasChanged {
		// All replicas just became ready - requeue once to ensure status is persisted and visible
		logger.V(1).Info("All replicas became ready; requeuing once to ensure status is persisted", "readyReplicas", readyReplicas, "previousReadyReplicas", previousReadyReplicas)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil
	}

	return ctrl.Result{}, nil
}

func breakGlassStatusChanged(before, after *openbaov1alpha1.BreakGlassStatus) bool {
	if before == nil && after == nil {
		return false
	}
	if (before == nil) != (after == nil) {
		return true
	}

	if before.Active != after.Active {
		return true
	}
	if before.Reason != after.Reason {
		return true
	}
	if before.Nonce != after.Nonce {
		return true
	}

	return false
}

func evaluateProductionReady(cluster *openbaov1alpha1.OpenBaoCluster) (metav1.ConditionStatus, string, string) {
	if cluster.Spec.Profile == "" {
		return metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		return metav1.ConditionFalse, ReasonDevelopmentProfile, "Development profile is not suitable for production"
	}

	if cluster.Spec.TLS.Mode == "" || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		return metav1.ConditionFalse, ReasonOperatorManagedTLS, "Hardened profile requires TLS mode External or ACME; OperatorManaged TLS is not considered production-ready"
	}

	if isStaticUnseal(cluster) {
		return metav1.ConditionFalse, ReasonStaticUnsealInUse, "Hardened profile requires a non-static unseal configuration (external KMS/Transit); static unseal is not considered production-ready"
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.ConditionFalse, ReasonRootTokenStored, "Hardened profile requires self-init; manual bootstrap stores a root token Secret and is not considered production-ready"
	}

	return metav1.ConditionTrue, ReasonProductionReady, "Cluster meets Hardened profile production-ready requirements"
}

func isStaticUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == unsealTypeStatic
}

func (r *OpenBaoClusterReconciler) emitSecurityWarningEvents(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if r.Recorder == nil {
		return nil
	}

	now := time.Now().UTC()
	annotationUpdates := make(map[string]string)

	maybeWarn := func(annotationKey, reason, message string) {
		if !shouldEmitSecurityWarning(cluster.Annotations, annotationKey, now) {
			return
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, reason, "%s", message)
		annotationUpdates[annotationKey] = now.Format(time.RFC3339Nano)
	}

	if cluster.Spec.Profile == "" {
		maybeWarn(constants.AnnotationLastProfileNotSetWarning, ReasonProfileNotSet, "spec.profile is not set; reconciliation is blocked until spec.profile is explicitly set to Hardened or Development")
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		maybeWarn(constants.AnnotationLastDevelopmentWarning, ReasonDevelopmentProfile, "Cluster is using Development profile; this is not suitable for production")
	}

	if isStaticUnseal(cluster) {
		maybeWarn(constants.AnnotationLastStaticUnsealWarning, ReasonStaticUnsealInUse, "Cluster is using static auto-unseal; the unseal key is stored in a Kubernetes Secret and is not suitable for production without etcd encryption at rest and strict RBAC")
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		maybeWarn(constants.AnnotationLastRootTokenWarning, ReasonRootTokenStored, "SelfInit is disabled; the operator will store a root token in a Kubernetes Secret during bootstrap, which is not suitable for production")
	}

	if len(annotationUpdates) == 0 {
		return nil
	}

	original := cluster.DeepCopy()
	if cluster.Annotations == nil {
		cluster.Annotations = make(map[string]string)
	}
	for k, v := range annotationUpdates {
		cluster.Annotations[k] = v
	}

	if err := r.Patch(ctx, cluster, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to persist security warning timestamps on OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	logger.V(1).Info("Emitted security warning events", "count", len(annotationUpdates))
	return nil
}

func shouldEmitSecurityWarning(annotations map[string]string, annotationKey string, now time.Time) bool {
	if len(annotations) == 0 {
		return true
	}

	raw, ok := annotations[annotationKey]
	if !ok || strings.TrimSpace(raw) == "" {
		return true
	}

	last, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return true
	}

	return now.Sub(last) >= constants.SecurityWarningInterval
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
	sentinelAwareHandler := handler.TypedFuncs[client.Object, ctrl.Request]{
		CreateFunc: func(ctx context.Context, evt event.TypedCreateEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
		DeleteFunc: func(ctx context.Context, evt event.TypedDeleteEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
		UpdateFunc: func(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.ObjectOld == nil || evt.ObjectNew == nil {
				return
			}
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.ObjectNew.GetNamespace(),
					Name:      evt.ObjectNew.GetName(),
				},
			}

			oldAnnotations := evt.ObjectOld.GetAnnotations()
			newAnnotations := evt.ObjectNew.GetAnnotations()
			oldTrigger := ""
			if oldAnnotations != nil {
				oldTrigger = oldAnnotations[constants.AnnotationSentinelTrigger]
			}
			newTrigger := ""
			if newAnnotations != nil {
				newTrigger = newAnnotations[constants.AnnotationSentinelTrigger]
			}

			// Rate limit Sentinel fast-path triggers to avoid a single cluster
			// generating a continuous high-rate reconcile loop.
			// Deletion of the trigger annotation is not rate limited so the
			// post-fast-path safety check reconcile happens promptly.
			if newTrigger != "" && newTrigger != oldTrigger {
				q.AddRateLimited(req)
				return
			}

			q.Add(req)
		},
		GenericFunc: func(ctx context.Context, evt event.TypedGenericEvent[client.Object], q workqueue.TypedRateLimitingInterface[ctrl.Request]) {
			if evt.Object == nil {
				return
			}
			q.Add(ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: evt.Object.GetNamespace(),
					Name:      evt.Object.GetName(),
				},
			})
		},
	}

	builder := ctrl.NewControllerManagedBy(mgr).
		WithEventFilter(controllerutil.OpenBaoClusterPredicate()).
		Watches(&openbaov1alpha1.OpenBaoCluster{}, sentinelAwareHandler)

	return builder.
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3,
			RateLimiter: workqueue.NewTypedMaxOfRateLimiter(
				workqueue.NewTypedItemExponentialFailureRateLimiter[ctrl.Request](1*time.Second, 60*time.Second),
				&workqueue.TypedBucketRateLimiter[ctrl.Request]{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
			),
		}).
		Named("openbaocluster").
		Complete(r)
}
