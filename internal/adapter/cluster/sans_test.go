package cluster

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestComputeRequiredDNSSANs_NoBlueGreen(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
	}

	dnsNames := ComputeRequiredDNSSANs(cluster)
	if len(dnsNames) != 0 {
		t.Errorf("Expected no DNS names for cluster without Blue/Green status, got %d", len(dnsNames))
	}
}

func TestComputeRequiredDNSSANs_BlueRevisionOnly(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision: "abc123",
			},
		},
	}

	dnsNames := ComputeRequiredDNSSANs(cluster)
	expectedCount := 6 // 3 replicas * 2 DNS suffixes (.svc and .svc.cluster.local)
	if len(dnsNames) != expectedCount {
		t.Errorf("Expected %d DNS names, got %d", expectedCount, len(dnsNames))
	}

	// Verify expected DNS names are present
	expectedDNS := map[string]bool{
		"test-cluster-abc123-0.test-cluster.default.svc":               false,
		"test-cluster-abc123-0.test-cluster.default.svc.cluster.local": false,
		"test-cluster-abc123-1.test-cluster.default.svc":               false,
		"test-cluster-abc123-1.test-cluster.default.svc.cluster.local": false,
		"test-cluster-abc123-2.test-cluster.default.svc":               false,
		"test-cluster-abc123-2.test-cluster.default.svc.cluster.local": false,
	}

	for _, dnsName := range dnsNames {
		if _, ok := expectedDNS[dnsName]; ok {
			expectedDNS[dnsName] = true
		}
	}

	for dnsName, found := range expectedDNS {
		if !found {
			t.Errorf("Expected DNS name %q not found in results", dnsName)
		}
	}
}

func TestComputeRequiredDNSSANs_BothRevisions(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "production",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 2,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision:  "blue-rev",
				GreenRevision: "green-rev",
			},
		},
	}

	dnsNames := ComputeRequiredDNSSANs(cluster)
	expectedCount := 8 // 2 revisions * 2 replicas * 2 DNS suffixes
	if len(dnsNames) != expectedCount {
		t.Errorf("Expected %d DNS names, got %d", expectedCount, len(dnsNames))
	}

	// Verify both revisions are included
	hasBlue := false
	hasGreen := false
	for _, dnsName := range dnsNames {
		if strings.Contains(dnsName, "blue-rev") {
			hasBlue = true
		}
		if strings.Contains(dnsName, "green-rev") {
			hasGreen = true
		}
	}

	if !hasBlue {
		t.Error("Expected Blue revision DNS names not found")
	}
	if !hasGreen {
		t.Error("Expected Green revision DNS names not found")
	}
}

func TestComputeRequiredDNSSANs_EmptyRevisions(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen: &openbaov1alpha1.BlueGreenStatus{
				BlueRevision:  "",
				GreenRevision: "",
			},
		},
	}

	dnsNames := ComputeRequiredDNSSANs(cluster)
	if len(dnsNames) != 0 {
		t.Errorf("Expected no DNS names when revisions are empty, got %d", len(dnsNames))
	}
}

func TestComputeRequiredDNSSANs_NilCluster(t *testing.T) {
	dnsNames := ComputeRequiredDNSSANs(nil)
	if dnsNames != nil {
		t.Errorf("Expected nil for nil cluster, got %v", dnsNames)
	}
}
