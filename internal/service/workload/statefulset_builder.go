package workload

import (
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

const (
	statefulSetPlacementPreferredWeight = 100
	statefulSetTopologyKeyHostname      = "kubernetes.io/hostname"
	statefulSetTopologyKeyZone          = "topology.kubernetes.io/zone"
)

// buildStatefulSetWithRevision constructs a StatefulSet for the given OpenBaoCluster.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the resolved init container image to use.
// When operator image verification is enabled, this should be a digest.
// spec identifies the target workload pool and rollout shape.
func buildStatefulSetForSpec(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool, spec StatefulSetSpec, platform string) (*appsv1.StatefulSet, error) {
	if spec.Pool == "" {
		spec.Pool = constants.LabelValueOpenBaoWorkloadPoolVoter
	}

	labels := resourceidentity.PodSelectorLabelsForPoolWithRevision(cluster, spec.Pool, spec.Revision)

	replicas := desiredStatefulSetReplicas(cluster, initialized, spec)

	pvc, err := buildStatefulSetPVC(cluster, spec)
	if err != nil {
		return nil, err
	}

	podLabels, annotations := buildStatefulSetPodLabelsAndAnnotations(cluster, spec, configContent)

	probes := buildStatefulSetProbeExecActions(cluster)
	renderedConfigDir := path.Dir(openBaoRenderedConfig)
	volumes := buildStatefulSetVolumes(cluster, spec)

	initContainers, err := buildInitContainers(cluster, spec.InitContainerImage, spec.DisableSelfInit)
	if err != nil {
		return nil, err
	}

	statefulSetName := statefulSetNameForSpec(cluster, spec)
	var statefulSetAnnotations map[string]string
	if cluster.Spec.Maintenance != nil && cluster.Spec.Maintenance.Enabled {
		statefulSetAnnotations = map[string]string{
			constants.AnnotationMaintenance: maintenanceAnnotationEnabledValue,
		}
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        statefulSetName,
			Namespace:   cluster.Namespace,
			Labels:      resourceidentity.Labels(cluster),
			Annotations: statefulSetAnnotations,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: resourceidentity.HeadlessServiceName(cluster),
			Replicas:    int32Ptr(replicas),
			// Scale-down removes the departing Raft peer before shrinking the StatefulSet.
			// Reusing that ordinal's old data directory on a later scale-up resurrects
			// stale Raft membership, so scaled-down PVCs must be deleted.
			PersistentVolumeClaimRetentionPolicy: buildStatefulSetPVCRetentionPolicy(),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UpdateStrategy: buildStatefulSetUpdateStrategy(cluster, spec),
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
					// Use soft spread defaults so production clusters distribute Raft members
					// across nodes/zones without making small dev clusters unschedulable.
					Affinity: buildStatefulSetAffinity(cluster, spec),
					// SECURITY: Explicitly disable automount for all containers, then mount
					// ServiceAccount token only where needed (OpenBao container for Kubernetes Auth)
					AutomountServiceAccountToken: ptr.To(false),
					ServiceAccountName:           resourceidentity.ServiceAccountName(cluster),
					NodeSelector:                 buildStatefulSetNodeSelector(cluster, spec),
					SecurityContext:              buildStatefulSetPodSecurityContext(cluster, platform),
					InitContainers:               initContainers,
					Tolerations:                  buildStatefulSetTolerations(cluster, spec),
					Containers:                   buildContainers(cluster, spec, renderedConfigDir, probes),
					ImagePullSecrets:             cluster.Spec.ImagePullSecrets,
					TopologySpreadConstraints:    buildStatefulSetTopologySpreadConstraints(cluster, spec),
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

// buildStatefulSetWithRevision retains the blue/green-friendly voter path used by
// existing callers while delegating to the pool-aware builder.
func buildStatefulSetWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool, verifiedImageDigest string, verifiedInitContainerDigest string, revision string, platform string) (*appsv1.StatefulSet, error) {
	return buildStatefulSetForSpec(cluster, configContent, initialized, StatefulSetSpec{
		Name:               statefulSetNameWithRevision(cluster, revision),
		Pool:               constants.LabelValueOpenBaoWorkloadPoolVoter,
		Revision:           revision,
		Image:              verifiedImageDigest,
		InitContainerImage: verifiedInitContainerDigest,
		Replicas:           cluster.Spec.Replicas,
		DisableSelfInit:    false,
	}, platform)
}

func buildStatefulSetPVCRetentionPolicy() *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy {
	return &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
		WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
	}
}

func buildStatefulSetAffinity(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) *corev1.Affinity {
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Template != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling.Affinity != nil {
		return cluster.Spec.ReadReplicas.Template.Scheduling.Affinity.DeepCopy()
	}

	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: statefulSetPlacementPreferredWeight,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: statefulSetPlacementLabels(cluster, spec),
						},
						TopologyKey: statefulSetTopologyKeyHostname,
					},
				},
			},
		},
	}
}

func buildStatefulSetTopologySpreadConstraints(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) []corev1.TopologySpreadConstraint {
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Template != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling.TopologySpreadConstraints != nil {
		return append([]corev1.TopologySpreadConstraint(nil), cluster.Spec.ReadReplicas.Template.Scheduling.TopologySpreadConstraints...)
	}

	placementLabels := statefulSetPlacementLabels(cluster, spec)

	return []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       statefulSetTopologyKeyHostname,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: placementLabels,
			},
		},
		{
			MaxSkew:           1,
			TopologyKey:       statefulSetTopologyKeyZone,
			WhenUnsatisfiable: corev1.ScheduleAnyway,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: placementLabels,
			},
		},
	}
}

func buildStatefulSetNodeSelector(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) map[string]string {
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Template != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling.NodeSelector != nil {
		return mapClone(cluster.Spec.ReadReplicas.Template.Scheduling.NodeSelector)
	}
	return nil
}

func buildStatefulSetTolerations(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) []corev1.Toleration {
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Template != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling != nil &&
		cluster.Spec.ReadReplicas.Template.Scheduling.Tolerations != nil {
		return append([]corev1.Toleration(nil), cluster.Spec.ReadReplicas.Template.Scheduling.Tolerations...)
	}
	return nil
}

func mapClone(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func statefulSetPlacementLabels(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) map[string]string {
	labels := resourceidentity.PodSelectorLabelsForPool(cluster, spec.Pool)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[constants.LabelOpenBaoComponent] = constants.ComponentOpenBaoCluster
	return labels
}
