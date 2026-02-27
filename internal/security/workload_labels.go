package security

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	labelOpenBaoProfile           = "openbao.org/profile"
	labelOpenBaoDigestEnforcement = "openbao.org/digest-enforcement"
	labelValueDigestRequired      = "required"
)

// AddManagedWorkloadSecurityLabels applies optional security posture labels to
// operator-managed workload resources (StatefulSets and Jobs).
func AddManagedWorkloadSecurityLabels(labels map[string]string, cluster *openbaov1alpha1.OpenBaoCluster) {
	if labels == nil || cluster == nil {
		return
	}

	profile := strings.TrimSpace(string(cluster.Spec.Profile))
	if profile != "" {
		labels[labelOpenBaoProfile] = profile
	}

	if ManagedWorkloadDigestEnforcementRequired(cluster) {
		labels[labelOpenBaoDigestEnforcement] = labelValueDigestRequired
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
