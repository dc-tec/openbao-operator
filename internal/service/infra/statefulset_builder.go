package infra

import (
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/security"
)

// buildStatefulSetWithRevision constructs a StatefulSet for the given OpenBaoCluster.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the verified init container image digest to use (if provided, overrides cluster.Spec.InitContainer.Image).
// revision is an optional revision identifier for blue/green deployments.
// disableSelfInit prevents adding self-init logic (used for Green pods).
func buildStatefulSetWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool, verifiedImageDigest string, verifiedInitContainerDigest string, revision string, disableSelfInit bool, platform string) (*appsv1.StatefulSet, error) {
	labels := podSelectorLabelsWithRevision(cluster, revision)

	replicas := desiredStatefulSetReplicas(cluster, initialized)

	pvc, err := buildStatefulSetPVC(cluster)
	if err != nil {
		return nil, err
	}

	podLabels, annotations := buildStatefulSetPodLabelsAndAnnotations(cluster, revision, configContent)

	probes := buildStatefulSetProbeExecActions(cluster)
	renderedConfigDir := path.Dir(openBaoRenderedConfig)
	volumes := buildStatefulSetVolumes(cluster, revision, disableSelfInit)

	initContainers, err := buildInitContainers(cluster, verifiedInitContainerDigest, disableSelfInit)
	if err != nil {
		return nil, err
	}

	statefulSetName := statefulSetNameWithRevision(cluster, revision)
	var statefulSetAnnotations map[string]string
	if cluster.Spec.Maintenance != nil && cluster.Spec.Maintenance.Enabled {
		statefulSetAnnotations = map[string]string{
			constants.AnnotationMaintenance: "true",
		}
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        statefulSetName,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: statefulSetAnnotations,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: headlessServiceName(cluster),
			Replicas:    int32Ptr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: buildStatefulSetUpdateStrategy(cluster),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					// RESTORE ISOLATION: Always false (safe default)
					// The wrapper binary runs as PID 1 in the OpenBao container and manages
					// the OpenBao process directly, eliminating the need for ShareProcessNamespace
					// and the tls-reloader sidecar. This restores container isolation.
					ShareProcessNamespace: ptr.To(false),
					// SECURITY: Explicitly disable automount for all containers, then mount
					// ServiceAccount token only where needed (OpenBao container for Kubernetes Auth)
					AutomountServiceAccountToken: ptr.To(false),
					ServiceAccountName:           serviceAccountName(cluster),
					SecurityContext:              buildStatefulSetPodSecurityContext(cluster, platform),
					InitContainers:               initContainers,
					Containers:                   buildContainers(cluster, verifiedImageDigest, renderedConfigDir, probes),
					ImagePullSecrets:             cluster.Spec.ImagePullSecrets,
					Volumes:                      volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
	}

	security.AddManagedWorkloadSecurityLabels(statefulSet.Labels, cluster)

	return statefulSet, nil
}
