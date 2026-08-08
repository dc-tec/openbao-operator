package workload

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestBuildContainerResourcesUsesPoolSpecificRequirements(t *testing.T) {
	voterResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
	}
	readResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("250m"),
		},
	}

	tests := []struct {
		name          string
		pool          string
		resources     *corev1.ResourceRequirements
		readResources *corev1.ResourceRequirements
		want          corev1.ResourceRequirements
	}{
		{
			name:      "voter uses top-level resources",
			pool:      constants.LabelValueOpenBaoWorkloadPoolVoter,
			resources: voterResources,
			want:      *voterResources.DeepCopy(),
		},
		{
			name:          "read replica uses template resources",
			pool:          constants.LabelValueOpenBaoWorkloadPoolReadReplica,
			resources:     voterResources,
			readResources: readResources,
			want:          *readResources.DeepCopy(),
		},
		{
			name:      "read replica does not inherit voter resources",
			pool:      constants.LabelValueOpenBaoWorkloadPoolReadReplica,
			resources: voterResources,
		},
		{
			name: "voter resources remain optional",
			pool: constants.LabelValueOpenBaoWorkloadPoolVoter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalCluster("resource-cluster", "default")
			cluster.Spec.Resources = tt.resources
			if tt.readResources != nil {
				cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{
					Template: &openbaov1alpha1.ReadReplicaTemplateConfig{
						Resources: tt.readResources,
					},
				}
			}

			statefulSet, err := buildStatefulSetForSpec(cluster, "test-config", true, StatefulSetSpec{
				Pool:               tt.pool,
				Image:              "openbao/openbao@sha256:main",
				InitContainerImage: "ghcr.io/dc-tec/openbao-init@sha256:init",
				Replicas:           1,
			}, constants.PlatformKubernetes)
			if err != nil {
				t.Fatalf("buildStatefulSetForSpec() error = %v", err)
			}

			got := statefulSet.Spec.Template.Spec.Containers[0].Resources
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resources = %#v, want %#v", got, tt.want)
			}
		})
	}
}
