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
)

func desiredStatefulSetReplicas(cluster *openbaov1alpha1.OpenBaoCluster, initialized bool) int32 {
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

func buildStatefulSetUpdateStrategy(cluster *openbaov1alpha1.OpenBaoCluster) appsv1.StatefulSetUpdateStrategy {
	// For blue/green deployments, use OnDelete update strategy to preventing
	// automatic rolling updates. The BlueGreenManager controls when pods are created/updated.
	// For standard rolling upgrades, use RollingUpdate (the Kubernetes default behavior).
	//
	// Important: The rolling upgrade manager controls the RollingUpdate.Partition field to
	// orchestrate upgrades. The infra Manager strips updateStrategy from SSA patches for
	// non-BlueGreen clusters to avoid clearing/overriding that partition value.
	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		return appsv1.StatefulSetUpdateStrategy{
			Type: appsv1.OnDeleteStatefulSetStrategyType,
		}
	}
	return appsv1.StatefulSetUpdateStrategy{
		Type: appsv1.RollingUpdateStatefulSetStrategyType,
	}
}

func buildStatefulSetPVC(cluster *openbaov1alpha1.OpenBaoCluster) (corev1.PersistentVolumeClaim, error) {
	size, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
	if err != nil {
		return corev1.PersistentVolumeClaim{}, fmt.Errorf("invalid storage size %q: %w", cluster.Spec.Storage.Size, err)
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

	if cluster.Spec.Storage.StorageClassName != nil && *cluster.Spec.Storage.StorageClassName != "" {
		className := *cluster.Spec.Storage.StorageClassName
		pvc.Spec.StorageClassName = &className
	}

	return pvc, nil
}

func buildStatefulSetPodLabelsAndAnnotations(cluster *openbaov1alpha1.OpenBaoCluster, revision string, configContent string) (map[string]string, map[string]string) {
	podLabels := resourceidentity.PodSelectorLabelsWithRevision(cluster, revision)

	if podLabels == nil {
		podLabels = make(map[string]string)
	}
	if cluster.Spec.PodMetadata != nil {
		for key, value := range cluster.Spec.PodMetadata.Labels {
			if _, exists := podLabels[key]; exists {
				continue
			}
			podLabels[key] = value
		}
	}
	podLabels[constants.LabelOpenBaoComponent] = constants.ComponentOpenBaoCluster

	annotations := map[string]string{}
	if cluster.Spec.PodMetadata != nil {
		for key, value := range cluster.Spec.PodMetadata.Annotations {
			annotations[key] = value
		}
	}

	// Compute config hash and add to annotations to trigger rollout on config changes
	annotations[configHashAnnotation] = computeConfigHash(configContent)

	restartAt := ""
	if cluster.Spec.Runtime != nil {
		restartAt = strings.TrimSpace(cluster.Spec.Runtime.RestartAt)
	}
	if restartAt == "" && cluster.Spec.Maintenance != nil {
		restartAt = strings.TrimSpace(cluster.Spec.Maintenance.RestartAt)
	}
	if restartAt != "" {
		annotations[constants.AnnotationRestartAt] = restartAt
	}

	return podLabels, annotations
}
