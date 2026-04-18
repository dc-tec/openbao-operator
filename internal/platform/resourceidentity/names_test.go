package resourceidentity

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLabels(t *testing.T) {
	cluster := newTestCluster()

	got := Labels(cluster)

	if got[constants.LabelAppName] != constants.LabelValueAppNameOpenBao {
		t.Fatalf("expected app name label, got %#v", got)
	}
	if got[constants.LabelAppInstance] != cluster.Name {
		t.Fatalf("expected app instance label %q, got %#v", cluster.Name, got)
	}
	if got[constants.LabelOpenBaoCluster] != cluster.Name {
		t.Fatalf("expected cluster label %q, got %#v", cluster.Name, got)
	}
}

func TestPodSelectorLabelsWithRevision(t *testing.T) {
	cluster := newTestCluster()

	got := PodSelectorLabelsWithRevision(cluster, "blue123")

	if got[constants.LabelOpenBaoRevision] != "blue123" {
		t.Fatalf("expected revision label, got %#v", got)
	}
	if got[constants.LabelOpenBaoCluster] != cluster.Name {
		t.Fatalf("expected cluster label, got %#v", got)
	}
}

func TestServiceAccountName(t *testing.T) {
	cluster := newTestCluster()
	if got := ServiceAccountName(cluster); got != cluster.Name+constants.SuffixServiceAccount {
		t.Fatalf("expected default service account name, got %q", got)
	}

	cluster.Spec.ServiceAccount = &openbaov1alpha1.ServiceAccountConfig{Name: "custom-sa"}
	if got := ServiceAccountName(cluster); got != "custom-sa" {
		t.Fatalf("expected explicit service account name, got %q", got)
	}
}

func TestSharedNames(t *testing.T) {
	cluster := newTestCluster()

	if got := HeadlessServiceName(cluster); got != cluster.Name {
		t.Fatalf("expected headless service name %q, got %q", cluster.Name, got)
	}
	if got := ConfigMapName(cluster); got != cluster.Name+constants.SuffixConfigMap {
		t.Fatalf("expected config map name, got %q", got)
	}
	if got := ConfigMapNameWithRevision(cluster, "green123"); got != cluster.Name+constants.SuffixConfigMap+"-green123" {
		t.Fatalf("expected revision config map name, got %q", got)
	}
	if got := ConfigInitMapName(cluster); got != cluster.Name+"-config-init" {
		t.Fatalf("expected config init map name, got %q", got)
	}
	if got := UnsealSecretName(cluster); got != cluster.Name+constants.SuffixUnsealKey {
		t.Fatalf("expected unseal secret name, got %q", got)
	}
	if got := TLSServerSecretName(cluster); got != cluster.Name+constants.SuffixTLSServer {
		t.Fatalf("expected tls server secret name, got %q", got)
	}
}

func newTestCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
}
