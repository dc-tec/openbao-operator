package config

import (
	"fmt"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
	platformsemver "github.com/kubebao/openbao-operator/internal/platform/semver"
)

func validateListenerTLSPolicy(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster.Spec.Configuration == nil || cluster.Spec.Configuration.Listener == nil {
		return nil
	}
	policy := cluster.Spec.Configuration.Listener
	if policy.TLSMinVersion == "" && policy.TLSMaxVersion == "" && policy.TLSKeyExchangePreferences == nil {
		return nil
	}
	if !cluster.Spec.TLS.Enabled || (policy.TLSDisable != nil && *policy.TLSDisable) {
		return fmt.Errorf("spec.configuration.listener TLS policy requires TLS enabled and tlsDisable!=true")
	}
	for _, version := range []openbaov1alpha1.TLSVersion{policy.TLSMinVersion, policy.TLSMaxVersion} {
		if version != "" && version != openbaov1alpha1.TLSVersion12 && version != openbaov1alpha1.TLSVersion13 {
			return fmt.Errorf("spec.configuration.listener TLS version %q is unsupported; use tls12 or tls13", version)
		}
	}
	if policy.TLSMinVersion == openbaov1alpha1.TLSVersion13 && policy.TLSMaxVersion == openbaov1alpha1.TLSVersion12 {
		return fmt.Errorf("spec.configuration.listener.tlsMaxVersion must be greater than or equal to tlsMinVersion")
	}
	if policy.TLSKeyExchangePreferences == nil {
		return nil
	}
	if err := validateListenerTLSGroups(policy); err != nil {
		return err
	}
	versions := []string{cluster.Spec.Version}
	if cluster.Status.CurrentVersion != "" {
		versions = append(versions, cluster.Status.CurrentVersion)
	}
	for _, version := range versions {
		supported, err := platformsemver.AtLeast(version, 2, 7, 0)
		if err != nil {
			return fmt.Errorf("validate listener TLS key exchange version: %w", err)
		}
		if !supported {
			return fmt.Errorf("spec.configuration.listener.tlsKeyExchangePreferences requires OpenBao >= 2.7.0; complete the upgrade before configuring key exchange groups (version=%q)", version)
		}
	}
	return nil
}

func validateListenerTLSGroups(policy *openbaov1alpha1.ListenerConfig) error {
	if len(policy.TLSKeyExchangePreferences) == 0 || len(policy.TLSKeyExchangePreferences) > 7 {
		return fmt.Errorf("spec.configuration.listener.tlsKeyExchangePreferences requires 1 to 7 groups")
	}
	seen := make(map[openbaov1alpha1.TLSKeyExchangeGroup]bool)
	classical := false
	for _, group := range policy.TLSKeyExchangePreferences {
		if seen[group] {
			return fmt.Errorf("spec.configuration.listener.tlsKeyExchangePreferences contains duplicate group %q", group)
		}
		seen[group] = true
		switch group {
		case openbaov1alpha1.TLSKeyExchangeCurveP256, openbaov1alpha1.TLSKeyExchangeCurveP384,
			openbaov1alpha1.TLSKeyExchangeCurveP521, openbaov1alpha1.TLSKeyExchangeX25519:
			classical = true
		case openbaov1alpha1.TLSKeyExchangeX25519MLKEM768, openbaov1alpha1.TLSKeyExchangeSecP256r1MLKEM768,
			openbaov1alpha1.TLSKeyExchangeSecP384r1MLKEM1024:
		default:
			return fmt.Errorf("spec.configuration.listener.tlsKeyExchangePreferences contains unsupported group %q", group)
		}
	}
	if !classical && policy.TLSMinVersion != openbaov1alpha1.TLSVersion13 {
		return fmt.Errorf("spec.configuration.listener PQ-only key exchange requires tlsMinVersion=tls13")
	}
	return nil
}
