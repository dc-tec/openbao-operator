package workload

import (
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

const readReplicaMarker = "true"

func TestBuildStatefulSetForSpec_ReadReplicaPoolUsesOverrides(t *testing.T) {
	cluster := newMinimalCluster("read-cluster", "default")
	cluster.Spec.PodMetadata = &openbaov1alpha1.PodMetadataConfig{
		Labels: map[string]string{
			"base-label": "base",
		},
		Annotations: map[string]string{
			"base-annotation": "base",
		},
	}
	storageClassName := "fast-ssd"
	readSize := resource.MustParse("20Gi")
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{
		Replicas: 2,
		Template: &openbaov1alpha1.ReadReplicaTemplateConfig{
			Metadata: &openbaov1alpha1.PodMetadataConfig{
				Labels: map[string]string{
					"read-label": readReplicaMarker,
				},
				Annotations: map[string]string{
					"read-annotation": readReplicaMarker,
				},
			},
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("250m"),
				},
			},
			Scheduling: &openbaov1alpha1.ReadReplicaSchedulingConfig{
				NodeSelector: map[string]string{"topology.kubernetes.io/zone": "eu-west-1a"},
				Tolerations: []corev1.Toleration{
					{Key: "workload", Operator: corev1.TolerationOpEqual, Value: "read"},
				},
			},
		},
		Storage: &openbaov1alpha1.ReadReplicaStorageConfig{
			Size:             &readSize,
			StorageClassName: &storageClassName,
		},
	}

	spec := StatefulSetSpec{
		Name:               resourceidentity.ReadReplicaStatefulSetName(cluster),
		Pool:               constants.LabelValueOpenBaoWorkloadPoolReadReplica,
		Image:              "openbao/openbao@sha256:main",
		InitContainerImage: "ghcr.io/dc-tec/openbao-init@sha256:init",
		Replicas:           2,
		DisableSelfInit:    true,
	}

	sts, err := buildStatefulSetForSpec(cluster, "test-config", true, spec, constants.PlatformKubernetes)
	if err != nil {
		t.Fatalf("buildStatefulSetForSpec() error = %v", err)
	}

	if sts.Name != resourceidentity.ReadReplicaStatefulSetName(cluster) {
		t.Fatalf("StatefulSet name = %q, want %q", sts.Name, resourceidentity.ReadReplicaStatefulSetName(cluster))
	}
	if got := sts.Spec.Selector.MatchLabels[constants.LabelOpenBaoWorkloadPool]; got != constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		t.Fatalf("selector workload pool = %q, want %q", got, constants.LabelValueOpenBaoWorkloadPoolReadReplica)
	}
	if got := sts.Spec.Template.Labels["base-label"]; got != "base" {
		t.Fatalf("base pod label = %q, want %q", got, "base")
	}
	if got := sts.Spec.Template.Labels["read-label"]; got != readReplicaMarker {
		t.Fatalf("read pod label = %q, want %q", got, readReplicaMarker)
	}
	if got := sts.Spec.Template.Annotations["read-annotation"]; got != readReplicaMarker {
		t.Fatalf("read pod annotation = %q, want %q", got, readReplicaMarker)
	}
	if got := sts.Spec.Template.Spec.NodeSelector["topology.kubernetes.io/zone"]; got != "eu-west-1a" {
		t.Fatalf("node selector = %q, want %q", got, "eu-west-1a")
	}
	if len(sts.Spec.Template.Spec.Tolerations) != 1 {
		t.Fatalf("tolerations len = %d, want 1", len(sts.Spec.Template.Spec.Tolerations))
	}

	openBaoContainer := sts.Spec.Template.Spec.Containers[0]
	if got := openBaoContainer.Resources.Requests.Cpu().String(); got != "250m" {
		t.Fatalf("CPU request = %q, want %q", got, "250m")
	}

	pvc := sts.Spec.VolumeClaimTemplates[0]
	if got := pvc.Spec.Resources.Requests.Storage().String(); got != "20Gi" {
		t.Fatalf("PVC size = %q, want %q", got, "20Gi")
	}
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != storageClassName {
		t.Fatalf("PVC storageClassName = %v, want %q", pvc.Spec.StorageClassName, storageClassName)
	}

	volumes := sts.Spec.Template.Spec.Volumes
	if !hasVolume(volumes, configVolumeName) {
		t.Fatalf("expected %q volume to be present", configVolumeName)
	}
	if hasVolume(volumes, configInitVolumeName) {
		t.Fatalf("did not expect %q volume for read replicas", configInitVolumeName)
	}

	configVolume, ok := getVolume(volumes, configVolumeName)
	if !ok || configVolume.ConfigMap == nil {
		t.Fatalf("expected config volume to use ConfigMap")
	}
	if got := configVolume.ConfigMap.Name; got != resourceidentity.ReadReplicaConfigMapName(cluster) {
		t.Fatalf("config ConfigMap name = %q, want %q", got, resourceidentity.ReadReplicaConfigMapName(cluster))
	}
}

func TestBuildStatefulSetForSpec_ReadReplicaPoolUsesSharedHeadlessDNS(t *testing.T) {
	cluster := newMinimalCluster("read-headless", "default")
	cluster.Spec.ReadReplicas = &openbaov1alpha1.ReadReplicaConfig{Replicas: 1}

	spec := StatefulSetSpec{
		Name:               resourceidentity.ReadReplicaStatefulSetName(cluster),
		Pool:               constants.LabelValueOpenBaoWorkloadPoolReadReplica,
		Image:              "openbao/openbao@sha256:main",
		InitContainerImage: "ghcr.io/dc-tec/openbao-init@sha256:init",
		Replicas:           1,
		DisableSelfInit:    true,
	}

	sts, err := buildStatefulSetForSpec(cluster, "test-config", true, spec, constants.PlatformKubernetes)
	if err != nil {
		t.Fatalf("buildStatefulSetForSpec() error = %v", err)
	}

	if got := sts.Spec.ServiceName; got != resourceidentity.HeadlessServiceName(cluster) {
		t.Fatalf("StatefulSet serviceName = %q, want %q", got, resourceidentity.HeadlessServiceName(cluster))
	}

	mounts := buildContainerVolumeMounts(cluster, path.Dir(openBaoRenderedConfig))
	if !hasVolumeMountWithPath(mounts, dataVolumeName, openBaoDataPath) {
		t.Fatalf("expected data volume mount at %q", openBaoDataPath)
	}
}
