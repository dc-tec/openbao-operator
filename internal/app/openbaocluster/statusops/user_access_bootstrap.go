package statusops

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const operatorJWTAuthMount = "jwt-operator"

// buildUserAccessBootstrapCondition reports whether the operator can recognize
// a likely user-facing authentication bootstrap path in self-init requests.
// This is intentionally best-effort and never blocks reconciliation.
func buildUserAccessBootstrapCondition(cluster *openbaov1alpha1.OpenBaoCluster) metav1.Condition {
	selfInitEnabled := cluster != nil && cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled
	if !selfInitEnabled {
		return metav1.Condition{
			Type:    string(openbaov1alpha1.ConditionUserAccessBootstrap),
			Status:  metav1.ConditionFalse,
			Reason:  ReasonDisabled,
			Message: "Self-init is disabled; user access bootstrap heuristics are not evaluated",
		}
	}

	mounts := recognizedUserAccessBootstrapMounts(cluster.Spec.SelfInit.Requests)
	if len(mounts) == 0 {
		return metav1.Condition{
			Type:   string(openbaov1alpha1.ConditionUserAccessBootstrap),
			Status: metav1.ConditionUnknown,
			Reason: ReasonUserAccessUnverified,
			Message: "Self-init is enabled, but the operator could not verify a user-facing authentication bootstrap path from spec.selfInit.requests. " +
				"spec.selfInit.oidc only bootstraps operator authentication. Verify that self-init requests create a user-facing auth method plus the roles, mappings, or accounts your operators need before relying on self-init.",
		}
	}

	return metav1.Condition{
		Type:   string(openbaov1alpha1.ConditionUserAccessBootstrap),
		Status: metav1.ConditionTrue,
		Reason: ReasonUserAccessConfigured,
		Message: fmt.Sprintf(
			"The operator recognized self-init requests that appear to bootstrap user authentication on %s. This is a best-effort heuristic; verify the resulting roles, mappings, and credentials before relying on self-init.",
			strings.Join(mounts, ", "),
		),
	}
}

// ApplyUserAccessBootstrapCondition evaluates and applies the best-effort
// user-access bootstrap condition with the supplied reconciliation timestamp.
func ApplyUserAccessBootstrapCondition(cluster *openbaov1alpha1.OpenBaoCluster, now metav1.Time) {
	condition := buildUserAccessBootstrapCondition(cluster)
	condition.ObservedGeneration = cluster.Generation
	condition.LastTransitionTime = now
	meta.SetStatusCondition(&cluster.Status.Conditions, condition)
}

func recognizedUserAccessBootstrapMounts(requests []openbaov1alpha1.SelfInitRequest) []string {
	if len(requests) == 0 {
		return nil
	}

	mounts := make(map[string]struct{}, len(requests))
	for _, req := range requests {
		mount, ok := recognizedUserAccessBootstrapMount(req)
		if !ok {
			continue
		}
		mounts[mount] = struct{}{}
	}

	if len(mounts) == 0 {
		return nil
	}

	out := make([]string, 0, len(mounts))
	for mount := range mounts {
		out = append(out, mount)
	}
	sort.Strings(out)
	return out
}

func recognizedUserAccessBootstrapMount(req openbaov1alpha1.SelfInitRequest) (string, bool) {
	path := strings.Trim(strings.ToLower(req.Path), "/")
	if path == "" {
		return "", false
	}

	if strings.HasPrefix(path, "auth/") {
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			return "", false
		}
		mount := parts[1]
		if mount == "" || mount == operatorJWTAuthMount || mount == "token" {
			return "", false
		}
		return "auth/" + mount, true
	}

	if strings.HasPrefix(path, "sys/auth/") {
		parts := strings.Split(path, "/")
		if len(parts) < 3 {
			return "", false
		}
		mount := parts[2]
		if mount == "" || mount == operatorJWTAuthMount || req.AuthMethod == nil {
			return "", false
		}
		if !isLikelyUserAuthMethod(req.AuthMethod.Type) {
			return "", false
		}
		return "auth/" + mount, true
	}

	return "", false
}

func isLikelyUserAuthMethod(methodType string) bool {
	switch strings.TrimSpace(strings.ToLower(methodType)) {
	case "jwt", "kubernetes", "userpass", "ldap", "oidc", "approle", "cert":
		return true
	default:
		return false
	}
}
