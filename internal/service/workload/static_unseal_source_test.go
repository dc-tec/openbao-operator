package workload

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/kubebao/openbao-operator/api/v1alpha1"
	"github.com/kubebao/openbao-operator/internal/platform/resourceidentity"
)

func TestStaticUnsealSourceKeepsGeneratedKey(t *testing.T) {
	tests := []struct {
		name        string
		ref         string
		generated   bool
		owned       bool
		wantMounted string
	}{
		{name: "generated key without reference", generated: true, owned: true},
		{name: "reference without generated key", ref: "original-key", wantMounted: "original-key"},
		{name: "legacy reference keeps generated key", ref: "stale-key", generated: true, owned: true},
		{name: "unowned generated name keeps reference", ref: "original-key", generated: true, wantMounted: "original-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := workloadOwnershipCluster()
			cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: "static"}
			if tt.ref != "" {
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: tt.ref}
			}
			generatedName := resourceidentity.UnsealSecretName(cluster)
			if tt.wantMounted == "" {
				tt.wantMounted = generatedName
			}

			var objects []client.Object
			if tt.generated {
				secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: generatedName, Namespace: cluster.Namespace}}
				if tt.owned {
					secret.OwnerReferences = []metav1.OwnerReference{*workloadOwnershipRef(cluster)}
				}
				objects = append(objects, secret)
			}
			mgr := workloadOwnershipManager(cluster, objects...)

			override, err := mgr.staticUnsealSecretName(context.Background(), logr.Discard(), cluster)
			if err != nil {
				t.Fatalf("staticUnsealSecretName() error = %v", err)
			}
			volumes := buildStatefulSetVolumes(cluster, StatefulSetSpec{staticUnsealSecret: override})
			mounted := ""
			for _, volume := range volumes {
				if volume.Name == unsealVolumeName && volume.Secret != nil {
					mounted = volume.Secret.SecretName
				}
			}
			if mounted != tt.wantMounted {
				t.Fatalf("mounted unseal Secret = %q, want %q", mounted, tt.wantMounted)
			}
		})
	}
}
