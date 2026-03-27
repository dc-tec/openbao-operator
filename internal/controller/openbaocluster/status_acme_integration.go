package openbaocluster

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// setACMEIntegrationReadyCondition evaluates the operator-managed prerequisites
// around OpenBao's native ACME flow. This does not report certificate issuance
// success; it only reports whether the operator can verify the surrounding
// integration contract it owns.
func (r *OpenBaoClusterReconciler) setACMEIntegrationReadyCondition(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) {
	if !portopenbao.UsesACMEMode(cluster) {
		meta.RemoveStatusCondition(&cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
		return
	}

	result := appopenbaocluster.EvaluateACMEIntegration(
		ctx,
		r.acmeIntegrationDependencies(),
		acmeIntegrationReasonPolicy(),
		cluster,
	)
	setACMEIntegrationReadyEvaluatedCondition(cluster, result)
}
