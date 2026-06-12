package workload

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func desiredStatefulSetReplicas(cluster *openbaov1alpha1.OpenBaoCluster, initialized bool, spec StatefulSetSpec) int32 {
	if spec.Replicas > 0 || spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		if !initialized {
			if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica {
				return 0
			}
			return 1
		}
		return spec.Replicas
	}

	replicas := cluster.Spec.Replicas
	if !initialized {
		replicas = 1
	}
	return replicas
}

func buildStatefulSetPodSecurityContext(cluster *openbaov1alpha1.OpenBaoCluster, platform string) *corev1.PodSecurityContext {
	securityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		// Enforce a secure seccomp profile to limit available system calls
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	// For OpenShift, we must NOT set RunAsUser, RunAsGroup, or FSGroup.
	// OpenShift assigns these dynamically via Security Context Constraints (SCC).
	// For standard Kubernetes (default), we pin them to ensure file ownership matches the image.
	if platform != constants.PlatformOpenShift {
		securityContext.RunAsUser = ptr.To(openBaoUserID)
		securityContext.RunAsGroup = ptr.To(openBaoGroupID)
		securityContext.FSGroup = ptr.To(openBaoGroupID)
	}

	// Apply configured overrides from CRD if any.
	// This allows users to set specific IDs (e.g. for custom SCCs or specific security requirements)
	// regardless of the platform default.
	if cluster.Spec.SecurityContext != nil {
		override := cluster.Spec.SecurityContext
		if override.RunAsUser != nil {
			securityContext.RunAsUser = override.RunAsUser
		}
		if override.RunAsGroup != nil {
			securityContext.RunAsGroup = override.RunAsGroup
		}
		if override.FSGroup != nil {
			securityContext.FSGroup = override.FSGroup
		}
		if override.RunAsNonRoot != nil {
			securityContext.RunAsNonRoot = override.RunAsNonRoot
		}
		if override.SeccompProfile != nil {
			securityContext.SeccompProfile = override.SeccompProfile
		}
		if override.FSGroupChangePolicy != nil {
			securityContext.FSGroupChangePolicy = override.FSGroupChangePolicy
		}
		if override.SupplementalGroups != nil {
			securityContext.SupplementalGroups = override.SupplementalGroups
		}
		if override.Sysctls != nil {
			securityContext.Sysctls = override.Sysctls
		}
		if override.WindowsOptions != nil {
			securityContext.WindowsOptions = override.WindowsOptions
		}
	}

	if cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled {
		securityContext.AppArmorProfile = &corev1.AppArmorProfile{
			Type: corev1.AppArmorProfileTypeRuntimeDefault,
		}
	}

	return securityContext
}

func buildStatefulSetUpdateStrategy(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) appsv1.StatefulSetUpdateStrategy {
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		return appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.RollingUpdateStatefulSetStrategyType,
		}
	}
	// For blue/green deployments, use OnDelete update strategy to preventing
	// automatic rolling updates. The BlueGreenManager controls when pods are created/updated.
	// For standard rolling upgrades, use RollingUpdate (the Kubernetes default behavior).
	//
	// Important: The rolling upgrade manager controls the RollingUpdate.Partition field to
	// orchestrate upgrades. The workload manager strips updateStrategy from StatefulSet
	// SSA patches for non-BlueGreen clusters to avoid clearing/overriding that partition value.
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		return appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.OnDeleteStatefulSetStrategyType,
		}
	}
	return appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}
}

func buildStatefulSetPVC(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) (corev1.PersistentVolumeClaim, error) {
	var size resource.Quantity
	switch {
	case spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Storage != nil &&
		cluster.Spec.ReadReplicas.Storage.Size != nil:
		size = cluster.Spec.ReadReplicas.Storage.Size.DeepCopy()
	default:
		parsedSize, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
		if err != nil {
			return corev1.PersistentVolumeClaim{}, fmt.Errorf("invalid storage size %q: %w", cluster.Spec.Storage.Size, err)
		}
		size = parsedSize
	}

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   dataVolumeName,
			Labels: resourceidentity.Labels(cluster),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}

	var storageClassName *string
	switch {
	case spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica &&
		cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Storage != nil &&
		cluster.Spec.ReadReplicas.Storage.StorageClassName != nil &&
		*cluster.Spec.ReadReplicas.Storage.StorageClassName != "":
		className := *cluster.Spec.ReadReplicas.Storage.StorageClassName
		storageClassName = &className
	case cluster.Spec.Storage.StorageClassName != nil && *cluster.Spec.Storage.StorageClassName != "":
		className := *cluster.Spec.Storage.StorageClassName
		storageClassName = &className
	}

	if storageClassName != nil {
		className := *storageClassName
		pvc.Spec.StorageClassName = &className
	}
	if err := resourceownership.SetOwnerUIDAnnotation(&pvc, cluster); err != nil {
		return corev1.PersistentVolumeClaim{}, err
	}

	return pvc, nil
}

func buildStatefulSetPodLabelsAndAnnotations(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec, configContent string) (map[string]string, map[string]string) {
	podLabels := resourceidentity.PodSelectorLabelsForPoolWithRevision(cluster, spec.Pool, spec.Revision)

	if podLabels == nil {
		podLabels = make(map[string]string)
	}
	mergeLabels := func(metadata *openbaov1alpha1.PodMetadataConfig) {
		if metadata == nil {
			return
		}
		for key, value := range metadata.Labels {
			if isReservedOpenBaoPodLabel(key) {
				continue
			}
			if _, exists := podLabels[key]; exists {
				continue
			}
			podLabels[key] = value
		}
	}
	mergeLabels(cluster.Spec.PodMetadata)
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica && cluster.Spec.ReadReplicas != nil && cluster.Spec.ReadReplicas.Template != nil {
		mergeLabels(cluster.Spec.ReadReplicas.Template.Metadata)
	}
	podLabels[constants.LabelOpenBaoComponent] = constants.ComponentOpenBaoCluster

	annotations := map[string]string{}
	mergeAnnotations := func(metadata *openbaov1alpha1.PodMetadataConfig) {
		if metadata == nil {
			return
		}
		for key, value := range metadata.Annotations {
			annotations[key] = value
		}
	}
	mergeAnnotations(cluster.Spec.PodMetadata)
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica && cluster.Spec.ReadReplicas != nil && cluster.Spec.ReadReplicas.Template != nil {
		mergeAnnotations(cluster.Spec.ReadReplicas.Template.Metadata)
	}

	// Compute config hash and add to annotations to trigger rollout on config changes
	annotations[configHashAnnotation] = computeConfigHash(configContent)

	restartAt := effectiveRestartAt(cluster, spec)
	if restartAt != "" {
		annotations[constants.AnnotationRestartAt] = restartAt
	}

	return podLabels, annotations
}

func isReservedOpenBaoPodLabel(key string) bool {
	switch key {
	case portopenbao.LabelActive,
		portopenbao.LabelInitialized,
		portopenbao.LabelSealed,
		portopenbao.LabelVersion,
		"openbao-perf-standby":
		return true
	default:
		return false
	}
}

func effectiveRestartAt(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) string {
	if spec.RestartAt != nil {
		return strings.TrimSpace(*spec.RestartAt)
	}

	restartAt := ""
	if cluster.Spec.Runtime != nil {
		restartAt = strings.TrimSpace(cluster.Spec.Runtime.RestartAt)
	}
	if restartAt == "" && cluster.Spec.Maintenance != nil {
		restartAt = strings.TrimSpace(cluster.Spec.Maintenance.RestartAt)
	}
	return restartAt
}
