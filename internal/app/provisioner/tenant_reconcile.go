package provisioner

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	provisionermanager "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

const (
	// ReasonSecurityViolation indicates self-service tenant provisioning guardrail failure.
	ReasonSecurityViolation = "SecurityViolation"

	admissionDependencyRequeueAfter = 10 * time.Second
)

// TenantRuntime captures dependencies needed for OpenBaoTenant provisioning orchestration.
type TenantRuntime struct {
	Client                   client.Client
	APIReader                client.Reader
	Recorder                 events.EventRecorder
	Provisioner              *provisionermanager.Manager
	OperatorNamespace        string
	ConditionTypeProvisioned string
	RequeueShort             time.Duration
	RequeueStandard          time.Duration
}

// ReconcileOpenBaoTenant runs the business flow for namespace provisioning.
func ReconcileOpenBaoTenant(ctx context.Context, key types.NamespacedName, logger logr.Logger, runtime TenantRuntime) (recon.Result, error) {
	if runtime.Client == nil {
		return recon.Result{}, fmt.Errorf("client is required")
	}
	if runtime.Provisioner == nil {
		return recon.Result{}, fmt.Errorf("provisioner manager is required")
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{}
	if err := runtime.Client.Get(ctx, key, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			// OpenBaoTenant deleted - nothing to do.
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoTenant %s: %w", key, err)
	}

	targetNS := tenant.Spec.TargetNamespace
	logger = logger.WithValues("target_namespace", targetNS)

	isTrustedNamespace := tenant.Namespace == runtime.OperatorNamespace
	isSelfTargeting := tenant.Namespace == targetNS
	if !isTrustedNamespace && !isSelfTargeting {
		err := fmt.Errorf("security violation: OpenBaoTenant in namespace %q cannot target namespace %q",
			tenant.Namespace, targetNS)
		logger.Error(err, "Blocking provisioning attempt")
		logging.LogAuditEvent(logger, logging.EventTenantSecurityViolationBlocked, map[string]string{
			"tenant_namespace": tenant.Namespace,
			"tenant_name":      tenant.Name,
			"target_namespace": targetNS,
			"reason":           ReasonSecurityViolation,
		})
		runtime.emitTenantWarningEvent(tenant, ReasonTenantProvisioningBlocked, fmt.Sprintf("Tenant provisioning blocked for namespace %s: %v", targetNS, err))

		original := tenant.DeepCopy()
		tenant.Status.Provisioned = false
		tenant.Status.LastError = err.Error()
		meta.SetStatusCondition(&tenant.Status.Conditions, metav1.Condition{
			Type:               conditionTypeProvisioned(runtime),
			Status:             metav1.ConditionFalse,
			ObservedGeneration: tenant.Generation,
			Reason:             ReasonSecurityViolation,
			Message:            err.Error(),
		})
		if patchErr := patchStatus(ctx, runtime.Client, tenant, original); patchErr != nil {
			return recon.Result{}, fmt.Errorf("failed to patch status for security violation: %w", patchErr)
		}

		// Do not requeue. User must fix the CR.
		return recon.Result{}, nil
	}

	if !tenant.DeletionTimestamp.IsZero() {
		return reconcileDeletion(ctx, logger, runtime, tenant, targetNS, key)
	}

	if !containsFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
		tenant.Finalizers = append(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer)
		if err := runtime.Client.Update(ctx, tenant); err != nil {
			return recon.Result{}, fmt.Errorf("failed to add finalizer to OpenBaoTenant %s: %w", key, err)
		}
		// Requeue to observe the resource with the finalizer attached.
		return recon.Result{RequeueAfter: resolveRequeueShort(runtime)}, nil
	}

	ns := &corev1.Namespace{}
	if err := runtime.Client.Get(ctx, types.NamespacedName{Name: targetNS}, ns); err != nil {
		if apierrors.IsNotFound(err) {
			original := tenant.DeepCopy()
			tenant.Status.Provisioned = false
			tenant.Status.LastError = fmt.Sprintf("target namespace %s not found", targetNS)
			runtime.emitTenantWarningEvent(tenant, ReasonTenantProvisioningBlocked, fmt.Sprintf("Tenant provisioning blocked because target namespace %s was not found", targetNS))
			if patchErr := patchStatus(ctx, runtime.Client, tenant, original); patchErr != nil {
				return recon.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w", patchErr)
			}
			logger.Info("Target namespace not found; will retry", "target_namespace", targetNS)
			return recon.Result{RequeueAfter: resolveRequeueStandard(runtime)}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get namespace %s: %w", targetNS, err)
	}

	ready, result := ensureAdmissionDependenciesReady(ctx, logger, runtime, tenant)
	if !ready {
		return result, nil
	}

	logger.Info("Provisioning tenant RBAC", "target_namespace", targetNS)
	if err := runtime.Provisioner.EnsureTenantRBAC(ctx, tenant); err != nil {
		runtime.emitTenantWarningEvent(tenant, ReasonTenantProvisioningFailed, fmt.Sprintf("Tenant provisioning failed for namespace %s: %v", targetNS, err))
		original := tenant.DeepCopy()
		tenant.Status.Provisioned = false
		tenant.Status.LastError = err.Error()
		if statusErr := patchStatus(ctx, runtime.Client, tenant, original); statusErr != nil {
			return recon.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w (original error: %w)", statusErr, err)
		}
		return recon.Result{}, fmt.Errorf("failed to ensure tenant RBAC for namespace %s: %w", targetNS, err)
	}

	original := tenant.DeepCopy()
	tenant.Status.Provisioned = true
	tenant.Status.LastError = ""
	if err := patchStatus(ctx, runtime.Client, tenant, original); err != nil {
		return recon.Result{}, fmt.Errorf("failed to update OpenBaoTenant status: %w", err)
	}

	logger.Info("Successfully provisioned tenant RBAC", "target_namespace", targetNS)
	logging.LogAuditEvent(logger, logging.EventTenantRBACProvisioned, map[string]string{
		"tenant_namespace": tenant.Namespace,
		"tenant_name":      tenant.Name,
		"target_namespace": targetNS,
	})
	runtime.emitTenantNormalEvent(tenant, ReasonTenantProvisioned, fmt.Sprintf("Provisioned tenant RBAC for namespace %s", targetNS))
	return recon.Result{}, nil
}

func reconcileDeletion(
	ctx context.Context,
	logger logr.Logger,
	runtime TenantRuntime,
	tenant *openbaov1alpha1.OpenBaoTenant,
	targetNS string,
	key types.NamespacedName,
) (recon.Result, error) {
	if !containsFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer) {
		return recon.Result{}, nil
	}

	logger.Info("OpenBaoTenant is being deleted", "target_namespace", targetNS)

	// Keep tenant RBAC while OpenBaoCluster finalizers may still need it.
	clusterList := &openbaov1alpha1.OpenBaoClusterList{}
	if err := runtime.Client.List(ctx, clusterList, client.InNamespace(targetNS)); err != nil {
		return recon.Result{}, fmt.Errorf("failed to list keys in namespace %s: %w", targetNS, err)
	}
	if len(clusterList.Items) > 0 {
		logger.Info("Waiting for OpenBaoClusters to be deleted before cleaning up RBAC",
			"target_namespace", targetNS,
			"cluster_count", len(clusterList.Items))
		return recon.Result{RequeueAfter: 5 * time.Second}, nil
	}

	logger.Info("No OpenBaoClusters found; cleaning up tenant RBAC", "target_namespace", targetNS)
	if err := runtime.Provisioner.CleanupTenantRBAC(ctx, targetNS); err != nil {
		return recon.Result{}, fmt.Errorf("failed to cleanup tenant RBAC for namespace %s: %w", targetNS, err)
	}
	logging.LogAuditEvent(logger, logging.EventTenantRBACCleaned, map[string]string{
		"tenant_namespace": tenant.Namespace,
		"tenant_name":      tenant.Name,
		"target_namespace": targetNS,
	})
	runtime.emitTenantNormalEvent(tenant, ReasonTenantRBACCleaned, fmt.Sprintf("Cleaned tenant RBAC for namespace %s", targetNS))

	tenant.Finalizers = removeFinalizer(tenant.Finalizers, openbaov1alpha1.OpenBaoTenantFinalizer)
	if err := runtime.Client.Update(ctx, tenant); err != nil {
		return recon.Result{}, fmt.Errorf("failed to remove finalizer from OpenBaoTenant %s: %w", key, err)
	}

	return recon.Result{}, nil
}

func ensureAdmissionDependenciesReady(
	ctx context.Context,
	logger logr.Logger,
	runtime TenantRuntime,
	tenant *openbaov1alpha1.OpenBaoTenant,
) (bool, recon.Result) {
	// Fail-closed privileged actions when admission policies are not ready.
	if admission.UnsafeAdmissionDisabled() {
		admission.SetAdmissionDependenciesReady(true)
		return true, recon.Result{}
	}
	if admission.AdmissionDependenciesReady() {
		return true, recon.Result{}
	}

	reader := runtime.APIReader
	if reader == nil {
		reader = runtime.Client
	}

	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	status, err := admission.CheckDependencies(
		checkCtx,
		reader,
		admission.DefaultDependencies(),
		admission.DefaultNamePrefixes(),
	)
	cancel()
	if err != nil {
		admission.SetAdmissionDependenciesReady(false)
		logger.Info("Admission policy dependencies not ready; delaying tenant provisioning", "error", err)
		runtime.emitTenantWarningEvent(tenant, ReasonTenantProvisioningBlocked, fmt.Sprintf("Tenant provisioning blocked until admission dependencies are ready: %v", err))
		return false, recon.Result{RequeueAfter: admissionDependencyRequeueAfter}
	}

	admission.SetAdmissionDependenciesReady(status.OverallReady)
	if status.OverallReady {
		return true, recon.Result{}
	}

	original := tenant.DeepCopy()
	tenant.Status.Provisioned = false
	tenant.Status.LastError = status.SummaryMessage()
	// Keep existing behavior: best-effort status patch while requeueing.
	_ = patchStatus(ctx, runtime.Client, tenant, original)

	logger.Info("Admission policy dependencies not ready; delaying tenant provisioning", "summary", status.SummaryMessage())
	runtime.emitTenantWarningEvent(tenant, ReasonTenantProvisioningBlocked, fmt.Sprintf("Tenant provisioning blocked until admission dependencies are ready: %s", status.SummaryMessage()))
	return false, recon.Result{RequeueAfter: admissionDependencyRequeueAfter}
}

func patchStatus(ctx context.Context, c client.Client, tenant *openbaov1alpha1.OpenBaoTenant, original *openbaov1alpha1.OpenBaoTenant) error {
	return c.Status().Patch(ctx, tenant, client.MergeFrom(original))
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
		if f != value {
			result = append(result, f)
		}
	}
	return result
}

func conditionTypeProvisioned(runtime TenantRuntime) string {
	if runtime.ConditionTypeProvisioned != "" {
		return runtime.ConditionTypeProvisioned
	}
	return "Provisioned"
}

func resolveRequeueShort(runtime TenantRuntime) time.Duration {
	if runtime.RequeueShort > 0 {
		return runtime.RequeueShort
	}
	return 5 * time.Second
}

func resolveRequeueStandard(runtime TenantRuntime) time.Duration {
	if runtime.RequeueStandard > 0 {
		return runtime.RequeueStandard
	}
	return 1 * time.Minute
}
