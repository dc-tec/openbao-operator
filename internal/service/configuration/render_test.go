package configuration

import (
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRender_DefaultTopology(t *testing.T) {
	cluster := newRenderTestCluster()

	rendered, err := Render(cluster, RenderOptions{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, `provider=k8s namespace=default label_selector=\"openbao.org/cluster=test\"`) {
		t.Fatalf("expected default retry_join selector, got:\n%s", rendered)
	}
	if strings.Contains(rendered, `openbao.org/revision=`) {
		t.Fatalf("did not expect revision selector in default render, got:\n%s", rendered)
	}
}

func TestRender_TargetRevisionForJoin(t *testing.T) {
	cluster := newRenderTestCluster()

	rendered, err := Render(cluster, RenderOptions{TargetRevisionForJoin: "blue123"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !strings.Contains(rendered, `label_selector=\"openbao.org/cluster=test,openbao.org/revision=blue123\"`) {
		t.Fatalf("expected revision-aware retry_join selector, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, `retry_join_as_non_voter = true`) {
		t.Fatalf("expected non-voter join config, got:\n%s", rendered)
	}
}

func newRenderTestCluster() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			Profile:  openbaov1alpha1.ProfileDevelopment,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
		},
	}
}
