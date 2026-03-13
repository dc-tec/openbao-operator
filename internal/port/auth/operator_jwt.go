package auth

import (
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	// TokenAudienceOpenBaoInternal is the default Kubernetes projected
	// ServiceAccount token audience used for OpenBao JWT authentication.
	TokenAudienceOpenBaoInternal = "openbao-internal"

	// JWT auth role names used by the operator and helper executors.
	RoleNameOperator = "openbao-operator"
	RoleNameBackup   = "openbao-operator-backup"
	RoleNameUpgrade  = "openbao-operator-upgrade"
	RoleNameRestore  = "openbao-operator-restore"

	// Policy names used by the operator and helper executors.
	PolicyNameOperator = "openbao-operator"
	PolicyNameBackup   = "openbao-operator-backup"
	PolicyNameUpgrade  = "openbao-operator-upgrade"
	PolicyNameRestore  = "openbao-operator-restore"
)

// OperatorJWTBootstrapEnabled reports whether the cluster is configured to let
// the operator bootstrap JWT auth and the default executor roles through
// self-initialization.
func OperatorJWTBootstrapEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster != nil &&
		cluster.Spec.SelfInit != nil &&
		cluster.Spec.SelfInit.Enabled &&
		cluster.Spec.SelfInit.OIDC != nil &&
		cluster.Spec.SelfInit.OIDC.Enabled
}

// EffectiveJWTRole returns the configured role or a default when operator JWT
// bootstrap is enabled for the cluster.
func EffectiveJWTRole(configuredRole string, bootstrapEnabled bool, defaultRole string) string {
	role := strings.TrimSpace(configuredRole)
	if role != "" {
		return role
	}
	if bootstrapEnabled {
		return defaultRole
	}
	return ""
}

// OperatorJWTAudience returns the install-scoped audience used for projected
// OpenBao auth tokens. It defaults to TokenAudienceOpenBaoInternal when unset.
func OperatorJWTAudience(configuredAudience string) string {
	audience := strings.TrimSpace(configuredAudience)
	if audience == "" {
		audience = TokenAudienceOpenBaoInternal
	}
	return audience
}

// BootstrapAudienceOverride returns the audience explicitly configured on the
// cluster self-init OIDC stanza. It is retained for compatibility, but the
// effective JWT audience is installation-scoped.
func BootstrapAudienceOverride(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster == nil || cluster.Spec.SelfInit == nil || cluster.Spec.SelfInit.OIDC == nil {
		return ""
	}

	return strings.TrimSpace(cluster.Spec.SelfInit.OIDC.Audience)
}

// BootstrapAudienceMatchesInstallation reports whether the optional
// cluster-scoped self-init audience agrees with the install-scoped operator JWT
// audience. A blank override is always accepted.
func BootstrapAudienceMatchesInstallation(cluster *openbaov1alpha1.OpenBaoCluster, operatorAudience string) bool {
	override := BootstrapAudienceOverride(cluster)
	return override == "" || override == OperatorJWTAudience(operatorAudience)
}

// EffectiveBootstrapAudience returns the audience that self-init bootstrap
// should render into operator JWT roles. This is installation-scoped and does
// not vary per cluster.
func EffectiveBootstrapAudience(_ *openbaov1alpha1.OpenBaoCluster, operatorAudience string) string {
	return OperatorJWTAudience(operatorAudience)
}
