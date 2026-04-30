package security

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// AddManagedWorkloadSecurityLabels applies optional security posture labels to
// operator-managed workload resources (StatefulSets and Jobs).
func AddManagedWorkloadSecurityLabels(labels map[string]string, cluster *openbaov1alpha1.OpenBaoCluster) {
	if labels == nil || cluster == nil {
		return
	}

	profile := strings.TrimSpace(string(cluster.Spec.Profile))
	if profile != "" {
		labels[constants.LabelOpenBaoProfile] = profile
	}

	if ManagedWorkloadDigestEnforcementRequired(cluster) {
		labels[constants.LabelOpenBaoDigestEnforcement] = constants.LabelValueDigestEnforcementRequired
	}
}

// ManagedWorkloadDigestEnforcementRequired reports whether managed workload
// resources should be restricted to digest-only image references.
func ManagedWorkloadDigestEnforcementRequired(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil {
		return false
	}
	return cluster.Spec.Profile == openbaov1alpha1.ProfileHardened
}
