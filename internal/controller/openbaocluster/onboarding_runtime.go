package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (r *OpenBaoClusterReconciler) pauseForTenantOnboarding(
	ctx context.Context,
	logger logr.Logger,
	controllerName string,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (ctrl.Result, error, bool) {
	if r.SingleTenantMode || cluster == nil {
		return ctrl.Result{}, nil, false
	}

	tenant, pendingMessage, err := r.resolveTenantOnboardingTenant(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to verify tenant onboarding for namespace %s: %w", cluster.Namespace, err), true
	}
	if tenant == nil {
		logger.V(1).Info(
			"Tenant onboarding not finished; pausing reconciliation",
			"controller", controllerName,
			"namespace", cluster.Namespace,
			"reason", pendingMessage,
		)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil, true
	}
	if !tenant.Status.Provisioned {
		logger.V(1).Info(
			"Tenant onboarding not provisioned yet; pausing reconciliation",
			"controller", controllerName,
			"tenantNamespace", tenant.Namespace,
			"tenantName", tenant.Name,
			"targetNamespace", tenant.Spec.TargetNamespace,
		)
		return ctrl.Result{RequeueAfter: constants.RequeueShort}, nil, true
	}

	return ctrl.Result{}, nil, false
}

func (r *OpenBaoClusterReconciler) resolveTenantOnboardingTenant(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*openbaov1alpha1.OpenBaoTenant, string, error) {
	if cluster == nil {
		return nil, "OpenBaoCluster is required.", nil
	}

	if tenant, pendingMessage, err := r.resolveClaimManagedTenantOnboarding(ctx, cluster); tenant != nil || pendingMessage != "" || err != nil {
		return tenant, pendingMessage, err
	}

	return r.resolveNamespaceTenantOnboarding(ctx, cluster.Namespace)
}

func (r *OpenBaoClusterReconciler) resolveClaimManagedTenantOnboarding(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*openbaov1alpha1.OpenBaoTenant, string, error) {
	if cluster == nil || cluster.Labels[constants.LabelOpenBaoOwnershipMode] != constants.LabelValueOpenBaoOwnershipClaimManaged {
		return nil, "", nil
	}

	claimNamespace := cluster.Labels[constants.LabelOpenBaoClaimNamespace]
	claimName := cluster.Labels[constants.LabelOpenBaoClaimName]
	if claimNamespace == "" || claimName == "" {
		return nil, "", fmt.Errorf("claim-managed OpenBaoCluster is missing required claim ownership labels")
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	claimKey := types.NamespacedName{Namespace: claimNamespace, Name: claimName}
	if err := r.tenantOnboardingReader().Get(ctx, claimKey, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, "Referenced OpenBaoClusterClaim does not exist yet.", nil
		}
		return nil, "", fmt.Errorf("get OpenBaoClusterClaim %s/%s: %w", claimKey.Namespace, claimKey.Name, err)
	}
	if claim.Spec.TenantRef.Name == "" {
		return nil, "", fmt.Errorf("referenced OpenBaoClusterClaim %s/%s is missing spec.tenantRef.name", claimKey.Namespace, claimKey.Name)
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{}
	tenantKey := types.NamespacedName{Namespace: claim.Namespace, Name: claim.Spec.TenantRef.Name}
	if err := r.tenantOnboardingReader().Get(ctx, tenantKey, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, "Referenced OpenBaoTenant does not exist yet.", nil
		}
		return nil, "", fmt.Errorf("get OpenBaoTenant %s/%s: %w", tenantKey.Namespace, tenantKey.Name, err)
	}
	if tenant.Spec.TargetNamespace != cluster.Namespace {
		return nil, "", fmt.Errorf(
			"OpenBaoTenant %s/%s targets namespace %q, want %q",
			tenant.Namespace,
			tenant.Name,
			tenant.Spec.TargetNamespace,
			cluster.Namespace,
		)
	}

	return tenant, "", nil
}

func (r *OpenBaoClusterReconciler) resolveNamespaceTenantOnboarding(
	ctx context.Context,
	targetNamespace string,
) (*openbaov1alpha1.OpenBaoTenant, string, error) {
	reader := r.tenantOnboardingReader()
	matches := make([]openbaov1alpha1.OpenBaoTenant, 0, 2)

	for _, namespace := range tenantLookupNamespaces(r.OperatorNamespace, targetNamespace) {
		list := &openbaov1alpha1.OpenBaoTenantList{}
		if err := reader.List(ctx, list, client.InNamespace(namespace)); err != nil {
			return nil, "", fmt.Errorf("list OpenBaoTenants in namespace %s: %w", namespace, err)
		}
		for i := range list.Items {
			tenant := list.Items[i]
			if tenant.Spec.TargetNamespace == targetNamespace {
				matches = append(matches, *tenant.DeepCopy())
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, "No governing OpenBaoTenant targets this namespace yet.", nil
	case 1:
		return &matches[0], "", nil
	default:
		return nil, "", fmt.Errorf("multiple OpenBaoTenants target namespace %s", targetNamespace)
	}
}

func (r *OpenBaoClusterReconciler) tenantOnboardingReader() client.Reader {
	if r.APIReader != nil {
		return r.APIReader
	}
	return r.Client
}

func tenantLookupNamespaces(operatorNamespace, targetNamespace string) []string {
	namespaces := []string{targetNamespace}
	if operatorNamespace != "" && operatorNamespace != targetNamespace {
		namespaces = append(namespaces, operatorNamespace)
	}
	return namespaces
}
