package auth

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestOperatorJWTBootstrapEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name:    "nil cluster",
			cluster: nil,
			want:    false,
		},
		{
			name: "self init disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: false,
						OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
					},
				},
			},
			want: false,
		},
		{
			name: "oidc disabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: false},
					},
				},
			},
			want: false,
		},
		{
			name: "self init and oidc enabled",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OperatorJWTBootstrapEnabled(tt.cluster); got != tt.want {
				t.Fatalf("OperatorJWTBootstrapEnabled() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestEffectiveJWTRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		configuredRole   string
		bootstrapEnabled bool
		defaultRole      string
		want             string
	}{
		{
			name:             "configured role wins",
			configuredRole:   "custom-role",
			bootstrapEnabled: true,
			defaultRole:      RoleNameBackup,
			want:             "custom-role",
		},
		{
			name:             "bootstrap enabled uses default role",
			configuredRole:   "",
			bootstrapEnabled: true,
			defaultRole:      RoleNameBackup,
			want:             RoleNameBackup,
		},
		{
			name:             "bootstrap disabled leaves role empty",
			configuredRole:   "",
			bootstrapEnabled: false,
			defaultRole:      RoleNameBackup,
			want:             "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EffectiveJWTRole(tt.configuredRole, tt.bootstrapEnabled, tt.defaultRole); got != tt.want {
				t.Fatalf("EffectiveJWTRole() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOperatorJWTAudience(t *testing.T) {
	t.Parallel()

	if got := OperatorJWTAudience("runtime-audience"); got != "runtime-audience" {
		t.Fatalf("OperatorJWTAudience() = %q, want runtime-audience", got)
	}

	if got := OperatorJWTAudience(""); got != TokenAudienceOpenBaoInternal {
		t.Fatalf("OperatorJWTAudience(\"\") = %q, want %q", got, TokenAudienceOpenBaoInternal)
	}
}

func TestBootstrapAudienceMatchesInstallation(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled:  true,
					Audience: "custom-audience",
				},
			},
		},
	}

	if BootstrapAudienceMatchesInstallation(cluster, "runtime-audience") {
		t.Fatal("BootstrapAudienceMatchesInstallation() = true, want false")
	}

	if !BootstrapAudienceMatchesInstallation(cluster, "custom-audience") {
		t.Fatal("BootstrapAudienceMatchesInstallation() = false, want true")
	}

	cluster.Spec.SelfInit.OIDC.Audience = ""
	if !BootstrapAudienceMatchesInstallation(cluster, "runtime-audience") {
		t.Fatal("BootstrapAudienceMatchesInstallation() = false, want true for empty override")
	}
}

func TestEffectiveBootstrapAudience(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
					Enabled:  true,
					Audience: "custom-audience",
				},
			},
		},
	}

	if got := EffectiveBootstrapAudience(cluster, "runtime-audience"); got != "runtime-audience" {
		t.Fatalf("EffectiveBootstrapAudience() = %q, want runtime-audience", got)
	}

	if got := EffectiveBootstrapAudience(nil, ""); got != TokenAudienceOpenBaoInternal {
		t.Fatalf("EffectiveBootstrapAudience(nil, \"\") = %q, want %q", got, TokenAudienceOpenBaoInternal)
	}
}
