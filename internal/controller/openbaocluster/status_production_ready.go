package openbaocluster

import (
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func evaluateProductionReady(cluster *openbaov1alpha1.OpenBaoCluster, admissionReady bool, admissionSummary string, unsafeAdmission bool) (metav1.ConditionStatus, string, string) {
	if cluster.Spec.Profile == "" {
		return metav1.ConditionFalse, ReasonProfileNotSet, "spec.profile must be explicitly set to Hardened or Development"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileDevelopment {
		return metav1.ConditionFalse, ReasonDevelopmentProfile, "Development profile is not suitable for production"
	}

	if unsafeAdmission {
		return metav1.ConditionFalse, ReasonUnsafeAdmissionDisabled, "Hardened profile requires enforced admission policies; unsafe admission mode is not considered production-ready"
	}

	if !admissionReady {
		if admissionSummary != "" {
			return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready: " + admissionSummary
		}
		return metav1.ConditionFalse, ReasonAdmissionPoliciesNotReady, "Required admission policies are not ready"
	}

	if status, reason, message, blocked := requireConditionFalseOnly(
		cluster,
		openbaov1alpha1.ConditionAPIServerNetworkReady,
		"Kubernetes API egress prerequisites are not ready",
	); blocked {
		return status, reason, message
	}

	if cluster.Spec.TLS.Mode == "" || cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		return metav1.ConditionFalse, ReasonOperatorManagedTLS, "Hardened profile requires TLS mode External or ACME; OperatorManaged TLS is not considered production-ready"
	}

	if isStaticUnseal(cluster) {
		return metav1.ConditionFalse, ReasonStaticUnsealInUse, "Hardened profile requires a non-static unseal configuration (external KMS/Transit); static unseal is not considered production-ready"
	}

	if unsealTLSSkipVerifyEnabled(cluster) {
		return metav1.ConditionFalse, ReasonUnsealTLSSkipVerify, "Hardened profile requires TLS verification for external unseal backends; tlsSkipVerify is not considered production-ready"
	}

	if transitInlineTokenConfigured(cluster) {
		return metav1.ConditionFalse, ReasonTransitInlineToken, "Hardened profile does not allow spec.unseal.transit.token; use spec.unseal.credentialsSecretRef instead"
	}

	if transitAddressRequiresHTTPS(cluster) {
		return metav1.ConditionFalse, ReasonTransitAddressNotHTTPS, "Hardened profile requires spec.unseal.transit.address to use a valid HTTPS URL"
	}

	if cluster.Spec.Profile == openbaov1alpha1.ProfileHardened && hardenedSecurityContextWeakensPodSecurity(cluster) {
		return metav1.ConditionFalse, ReasonSecurityContextWeakening, "Hardened profile does not allow spec.securityContext overrides that weaken non-root, seccomp, sysctl, or OS constraints"
	}

	if usesCloudKMSUnseal(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionCloudUnsealIdentityReady,
			"Cloud KMS unseal identity prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	if portopenbao.UsesACMEMode(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionACMEIntegrationReady,
			"ACME integration prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	if portopenbao.RequiresSharedACMECache(cluster) {
		if status, reason, message, blocked := requireConditionTrue(
			cluster,
			openbaov1alpha1.ConditionACMECacheReady,
			"ACME shared cache is not ready for this topology",
		); blocked {
			return status, reason, message
		}
	}

	if cluster.Spec.Gateway != nil && cluster.Spec.Gateway.Enabled {
		if status, reason, message, blocked := requireConditionNotFalse(
			cluster,
			openbaov1alpha1.ConditionGatewayIntegrationReady,
			"Gateway integration readiness has not been evaluated",
			"Gateway integration prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}
	if cluster.Spec.Ingress != nil && cluster.Spec.Ingress.Enabled {
		if status, reason, message, blocked := requireConditionNotFalse(
			cluster,
			openbaov1alpha1.ConditionIngressIntegrationReady,
			"Ingress integration readiness has not been evaluated",
			"Ingress integration prerequisites are not ready",
		); blocked {
			return status, reason, message
		}
	}

	selfInitEnabled := cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.ConditionFalse, ReasonRootTokenStored, "Hardened profile requires self-init; manual bootstrap stores a root token Secret and is not considered production-ready"
	}

	return metav1.ConditionTrue, ReasonProductionReady, "Cluster meets Hardened profile production-ready requirements"
}

func requireConditionTrue(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	defaultMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil || condition.Status != metav1.ConditionTrue {
		if condition != nil && condition.Reason != "" {
			return metav1.ConditionFalse, condition.Reason, condition.Message, true
		}
		return metav1.ConditionFalse, ReasonProductionNotReady, defaultMessage, true
	}
	return "", "", "", false
}

func requireConditionNotFalse(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	missingMessage string,
	notReadyMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil {
		return metav1.ConditionFalse, ReasonProductionNotReady, missingMessage, true
	}
	if condition.Status == metav1.ConditionFalse {
		if condition.Reason != "" {
			return metav1.ConditionFalse, condition.Reason, condition.Message, true
		}
		return metav1.ConditionFalse, ReasonProductionNotReady, notReadyMessage, true
	}
	return "", "", "", false
}

func requireConditionFalseOnly(
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	notReadyMessage string,
) (metav1.ConditionStatus, string, string, bool) {
	condition := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if condition == nil || condition.Status != metav1.ConditionFalse {
		return "", "", "", false
	}
	if condition.Reason != "" {
		return metav1.ConditionFalse, condition.Reason, condition.Message, true
	}
	return metav1.ConditionFalse, ReasonProductionNotReady, notReadyMessage, true
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

func unsealTLSSkipVerifyEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return false
	}
	if cluster.Spec.Unseal.Transit != nil && cluster.Spec.Unseal.Transit.TLSSkipVerify != nil && *cluster.Spec.Unseal.Transit.TLSSkipVerify {
		return true
	}
	return false
}

func transitInlineTokenConfigured(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.Unseal != nil &&
		cluster.Spec.Unseal.Transit != nil &&
		strings.TrimSpace(cluster.Spec.Unseal.Transit.Token) != ""
}

func transitAddressRequiresHTTPS(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.Transit == nil {
		return false
	}

	address := strings.TrimSpace(cluster.Spec.Unseal.Transit.Address)
	if address == "" {
		return true
	}

	u, err := url.Parse(address)
	if err != nil {
		return true
	}

	return !strings.EqualFold(u.Scheme, "https") || strings.TrimSpace(u.Host) == ""
}

func hardenedSecurityContextWeakensPodSecurity(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.SecurityContext == nil {
		return false
	}

	securityContext := cluster.Spec.SecurityContext
	if securityContext.RunAsNonRoot != nil && !*securityContext.RunAsNonRoot {
		return true
	}
	if securityContext.RunAsUser != nil && *securityContext.RunAsUser == 0 {
		return true
	}
	if securityContext.RunAsGroup != nil && *securityContext.RunAsGroup == 0 {
		return true
	}
	if securityContext.FSGroup != nil && *securityContext.FSGroup == 0 {
		return true
	}
	for _, supplementalGroup := range securityContext.SupplementalGroups {
		if supplementalGroup == 0 {
			return true
		}
	}
	if securityContext.SeccompProfile != nil &&
		securityContext.SeccompProfile.Type == corev1.SeccompProfileTypeUnconfined {
		return true
	}
	if len(securityContext.Sysctls) > 0 {
		return true
	}
	if securityContext.WindowsOptions != nil {
		return true
	}

	return false
}

func usesCloudKMSUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster == nil || cluster.Spec.Unseal == nil {
		return false
	}

	switch cluster.Spec.Unseal.Type {
	case "awskms", "gcpckms", "azurekeyvault", "ocikms":
		return true
	default:
		return false
	}
}
