package bluegreen

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestPreUpgradeSnapshotJobName(t *testing.T) {
	tests := []struct {
		name       string
		cluster    *openbaov1alpha1.OpenBaoCluster
		wantChange func(c *openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name: "stable for same inputs",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot-cluster"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
		},
		{
			name: "changes when target version changes",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "snapshot-cluster"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
			wantChange: func(c *openbaov1alpha1.OpenBaoCluster) { c.Spec.Version = "2.4.5" },
		},
		{
			name: "stays within DNS label limit for long cluster names",
			cluster: &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "this-is-a-very-long-cluster-name-that-must-be-trimmed-to-fit"},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "example.com/openbao:2.4.4",
					Replicas: 3,
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					CurrentVersion: "2.4.3",
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						BlueRevision: "blue",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := preUpgradeSnapshotJobName(tt.cluster)
			if first == "" {
				t.Fatalf("expected non-empty job name")
			}
			second := preUpgradeSnapshotJobName(tt.cluster)
			if first != second {
				t.Fatalf("expected stable job name, got %q then %q", first, second)
			}
			if len(first) > 63 {
				t.Fatalf("expected job name <= 63 chars, got %d: %q", len(first), first)
			}

			if tt.wantChange != nil {
				changed := tt.cluster.DeepCopy()
				tt.wantChange(changed)
				third := preUpgradeSnapshotJobName(changed)
				if third == first {
					t.Fatalf("expected job name to change after input change, got %q", third)
				}
			}
		})
	}
}
