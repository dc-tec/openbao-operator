package openbaocluster

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func evaluateProductionReady(cluster *openbaov1alpha1.OpenBaoCluster, admissionReady bool, admissionSummary string) (metav1.ConditionStatus, string, string) {
	if cluster.Spec.Profile == "" {
		return metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		return metav1.ConditionFalse, ReasonDevelopmentProfile, "Development profile is not suitable for production"
	}

	if !admissionReady {
		if admissionSummary != "" {
			return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready: " + admissionSummary
		}
		return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready"
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
