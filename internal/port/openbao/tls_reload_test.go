package openbao

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestUsesNativeTLSAutoReload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version string
		tls     openbaov1alpha1.TLSConfig
		want    bool
	}{
		{name: "older version", version: "2.6.3", tls: openbaov1alpha1.TLSConfig{Enabled: true}},
		{name: "prerelease", version: "2.7.0-rc.1", tls: openbaov1alpha1.TLSConfig{Enabled: true}},
		{name: "first supported version", version: "2.7.0", tls: openbaov1alpha1.TLSConfig{Enabled: true}, want: true},
		{name: "later version with external TLS", version: "v2.7.1", tls: openbaov1alpha1.TLSConfig{
			Enabled: true, Mode: openbaov1alpha1.TLSModeExternal,
		}, want: true},
		{name: "ACME", version: "2.7.0", tls: openbaov1alpha1.TLSConfig{
			Enabled: true, Mode: openbaov1alpha1.TLSModeACME,
		}},
		{name: "TLS disabled", version: "2.7.0"},
		{name: "invalid version", version: "invalid", tls: openbaov1alpha1.TLSConfig{Enabled: true}},
	}

	if UsesNativeTLSAutoReload(nil) {
		t.Fatal("nil cluster must not use native TLS reload")
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cluster := &openbaov1alpha1.OpenBaoCluster{Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Version: tt.version, TLS: tt.tls,
			}}
			if got := UsesNativeTLSAutoReload(cluster); got != tt.want {
				t.Fatalf("UsesNativeTLSAutoReload() = %t, want %t", got, tt.want)
			}
		})
	}
}
