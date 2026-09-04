package backup

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
)

func validateBackupHardenedConfiguration(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil || cluster.Spec.Backup == nil {
		return nil
	}
	if cluster.Spec.Profile != openbaov1alpha1.ProfileHardened {
		return nil
	}
	if cluster.Spec.Network == nil || len(cluster.Spec.Network.EgressRules) == 0 {
		return operatorerrors.WithReason(
			constants.ReasonNetworkEgressRulesRequired,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with backups enabled requires explicit spec.network.egressRules so backup Jobs can reach the object storage endpoint",
			)),
		)
	}
	if !hardenedcontract.EgressRulesExplicit(cluster.Spec.Network.EgressRules) {
		return operatorerrors.WithReason(
			constants.ReasonSecurityViolation,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(
				"hardened profile with backups enabled requires spec.network.egressRules entries to be port-scoped and target explicit non-wildcard peers",
			)),
		)
	}
	if violation := hardenedcontract.EvaluateStorageTarget("Backup", cluster.Spec.Backup.Target); violation != nil {
		return operatorerrors.WithReason(
			violation.Reason,
			operatorerrors.WrapPermanentConfig(fmt.Errorf("%s", violation.Message)),
		)
	}
	return nil
}
