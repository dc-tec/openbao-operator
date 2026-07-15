package openbao

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestRequiresSharedACMECache(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cluster *openbaov1alpha1.OpenBaoCluster
		want    bool
	}{
		{
			name: "single replica rolling ACME does not require shared cache",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 1,
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
					},
				},
			},
			want: false,
		},
		{
			name: "multi replica ACME requires shared cache",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 3,
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
					},
				},
			},
			want: true,
		},
		{
			name: "blue green ACME requires shared cache even with one replica",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 1,
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				},
			},
			want: true,
		},
		{
			name: "pending blue green transition keeps rolling cache requirements",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 1,
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyBlueGreen},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					AcceptedUpgradeStrategy: openbaov1alpha1.UpdateStrategyRollingUpdate,
				},
			},
			want: false,
		},
		{
			name: "pending rolling transition keeps blue green cache requirements",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Replicas: 1,
					TLS: openbaov1alpha1.TLSConfig{
						Enabled: true,
						Mode:    openbaov1alpha1.TLSModeACME,
						ACME:    &openbaov1alpha1.ACMEConfig{DirectoryURL: "https://acme.example/directory"},
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{Strategy: openbaov1alpha1.UpdateStrategyRollingUpdate},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					AcceptedUpgradeStrategy: openbaov1alpha1.UpdateStrategyBlueGreen,
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := RequiresSharedACMECache(tt.cluster); got != tt.want {
				t.Fatalf("RequiresSharedACMECache()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestACMESharedCacheClaimNameAndPath(t *testing.T) {
	t.Parallel()

	managed := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeACME,
				ACME: &openbaov1alpha1.ACMEConfig{
					DirectoryURL: "https://acme.example/directory",
					SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
						Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
						Size: "1Gi",
					},
				},
			},
		},
	}
	if got := ACMESharedCacheClaimName(managed); got != "example-acme-cache" {
		t.Fatalf("ACMESharedCacheClaimName(managed)=%q, want %q", got, "example-acme-cache")
	}
	if got := ACMESharedCachePath(managed); got != "/bao/acme-cache/certmagic" {
		t.Fatalf("ACMESharedCachePath(managed)=%q, want %q", got, "/bao/acme-cache/certmagic")
	}

	existing := managed.DeepCopy()
	existing.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
		Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
		ExistingClaimName: "shared-cache",
	}
	if got := ACMESharedCacheClaimName(existing); got != "shared-cache" {
		t.Fatalf("ACMESharedCacheClaimName(existing)=%q, want %q", got, "shared-cache")
	}

	noCache := managed.DeepCopy()
	noCache.Spec.TLS.ACME.SharedCache = nil
	if got := ACMESharedCacheClaimName(noCache); got != "" {
		t.Fatalf("ACMESharedCacheClaimName(noCache)=%q, want empty", got)
	}
	if got := ACMESharedCachePath(noCache); got != "/bao/data/certmagic" {
		t.Fatalf("ACMESharedCachePath(noCache)=%q, want %q", got, "/bao/data/certmagic")
	}
}

func TestUsesManagedAndExistingACMESharedCache(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{
				Enabled: true,
				Mode:    openbaov1alpha1.TLSModeACME,
				ACME: &openbaov1alpha1.ACMEConfig{
					DirectoryURL: "https://acme.example/directory",
					SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
						Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
						Size: "1Gi",
					},
				},
			},
			Unseal: &openbaov1alpha1.UnsealConfig{CredentialsSecretRef: &corev1.LocalObjectReference{Name: "seal-creds"}},
		},
	}
	if !UsesManagedACMESharedCache(cluster) {
		t.Fatal("expected managed ACME shared cache to be detected")
	}

	cluster.Spec.TLS.ACME.SharedCache = &openbaov1alpha1.ACMESharedCacheConfig{
		Mode:              openbaov1alpha1.ACMESharedCacheModeExistingPVC,
		ExistingClaimName: "shared-cache",
	}
	if !UsesExistingACMESharedCache(cluster) {
		t.Fatal("expected existing ACME shared cache to be detected")
	}
}
