package upgrade

import (
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/auth"
	"github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func buildUpgradeExecutorJob(
	cluster *openbaov1alpha1.OpenBaoCluster,
	jobName string,
	action ExecutorAction,
	runID string,
	blueRevision string,
	greenRevision string,
	verifiedExecutorDigest string,
	clientConfig portopenbao.ClientConfig,
	platform string,
) (*batchv1.Job, error) {
	image, err := resolveUpgradeExecutorImage(cluster, verifiedExecutorDigest)
	if err != nil {
		return nil, err
	}

	jwtRole := resolveUpgradeJWTAuthRole(cluster)
	if jwtRole == "" {
		return nil, fmt.Errorf("upgrade Jobs require JWT auth: Configure JWT auth and set the role name in spec.upgrade.jwtAuthRole")
	}

	tlsTrust, err := portopenbao.ResolveClientTrustBundle(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve upgrade TLS trust source: %w", err)
	}

	backoffLimit := int32(0)
	ttlSecondsAfterFinished := int32(upgradeJobTTLSeconds)
	jobLabels := buildUpgradeExecutorLabels(cluster)
	podTemplateLabels := buildUpgradeExecutorLabels(cluster)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   cluster.Namespace,
			Labels:      jobLabels,
			Annotations: buildUpgradeExecutorJobAnnotations(action, runID),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podTemplateLabels},
				Spec: corev1.PodSpec{
					ServiceAccountName:           cluster.Name + constants.SuffixUpgradeServiceAccount,
					AutomountServiceAccountToken: ptr.To(false),
					SecurityContext:              buildUpgradeExecutorPodSecurityContext(cluster, platform),
					RestartPolicy:                corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "upgrade-executor",
							Image:           image,
							SecurityContext: buildUpgradeExecutorContainerSecurityContext(),
							Resources:       buildUpgradeExecutorResources(),
							Env: buildUpgradeExecutorEnv(
								cluster,
								action,
								jwtRole,
								blueRevision,
								greenRevision,
								clientConfig,
								tlsTrust,
							),
							VolumeMounts: buildUpgradeExecutorVolumeMounts(tlsTrust),
						},
					},
					Volumes: buildUpgradeExecutorVolumes(tlsTrust),
				},
			},
		},
	}

	return job, nil
}

func resolveUpgradeExecutorImage(cluster *openbaov1alpha1.OpenBaoCluster, verifiedExecutorDigest string) (string, error) {
	image := verifiedExecutorDigest
	if image == "" && cluster.Spec.Upgrade != nil {
		image = strings.TrimSpace(cluster.Spec.Upgrade.Image)
	}
	if image != "" {
		return image, nil
	}

	image, err := constants.DefaultUpgradeImage()
	if err != nil {
		return "", operatorerrors.WrapPermanentConfig(operatorerrors.WithReason(
			constants.ReasonHelperImageConfigurationInvalid,
			fmt.Errorf(
				"default upgrade executor image is unavailable; set spec.upgrade.image explicitly or configure OPERATOR_VERSION in the operator Deployment: %w",
				err,
			),
		))
	}
	return image, nil
}

func resolveUpgradeJWTAuthRole(cluster *openbaov1alpha1.OpenBaoCluster) string {
	jwtRole := ""
	if cluster.Spec.Upgrade != nil {
		jwtRole = strings.TrimSpace(cluster.Spec.Upgrade.JWTAuthRole)
	}
	return portauth.EffectiveJWTRole(jwtRole, portauth.OperatorJWTBootstrapEnabled(cluster), portauth.RoleNameUpgrade)
}

func buildUpgradeExecutorEnv(
	cluster *openbaov1alpha1.OpenBaoCluster,
	action ExecutorAction,
	jwtRole string,
	blueRevision string,
	greenRevision string,
	clientConfig portopenbao.ClientConfig,
	tlsTrust portopenbao.TrustBundleSource,
) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: constants.EnvClusterNamespace, Value: cluster.Namespace},
		{Name: constants.EnvClusterName, Value: cluster.Name},
		{Name: constants.EnvClusterReplicas, Value: fmt.Sprintf("%d", cluster.Spec.Replicas)},
		{Name: constants.EnvUpgradeAction, Value: string(action)},
		{Name: constants.EnvUpgradeJWTAuthRole, Value: jwtRole},
	}
	if tlsServerName := portopenbao.ComputeTLSServerName(cluster); tlsServerName != "" {
		env = append(env, corev1.EnvVar{Name: constants.EnvTLSServerName, Value: tlsServerName})
	}
	if !tlsTrust.UseSystemRoots {
		env = append(env, corev1.EnvVar{Name: constants.EnvTLSCAPath, Value: constants.PathTLSCACert})
	}
	if blueRevision != "" {
		env = append(env, corev1.EnvVar{Name: constants.EnvUpgradeBlueRevision, Value: blueRevision})
	}
	if greenRevision != "" {
		env = append(env, corev1.EnvVar{Name: constants.EnvUpgradeGreenRevision, Value: greenRevision})
	}
	if clientConfig.RateLimitQPS > 0 {
		env = append(env, corev1.EnvVar{Name: constants.EnvClientQPS, Value: fmt.Sprintf("%f", clientConfig.RateLimitQPS)})
	}
	if clientConfig.RateLimitBurst > 0 {
		env = append(env, corev1.EnvVar{Name: constants.EnvClientBurst, Value: fmt.Sprintf("%d", clientConfig.RateLimitBurst)})
	}
	if clientConfig.CircuitBreakerFailureThreshold > 0 {
		env = append(env, corev1.EnvVar{Name: constants.EnvClientCircuitBreakerFailureThreshold, Value: fmt.Sprintf("%d", clientConfig.CircuitBreakerFailureThreshold)})
	}
	if clientConfig.CircuitBreakerOpenDuration > 0 {
		env = append(env, corev1.EnvVar{Name: constants.EnvClientCircuitBreakerOpenDuration, Value: clientConfig.CircuitBreakerOpenDuration.String()})
	}
	return env
}

func buildUpgradeExecutorLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	labels := map[string]string{
		constants.LabelAppName:          constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:      cluster.Name,
		constants.LabelAppManagedBy:     constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster:   cluster.Name,
		constants.LabelOpenBaoComponent: ComponentUpgrade,
	}
	security.AddManagedWorkloadSecurityLabels(labels, cluster)
	return labels
}

func buildUpgradeExecutorPodSecurityContext(cluster *openbaov1alpha1.OpenBaoCluster, platform string) *corev1.PodSecurityContext {
	podSecurityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	if platform != constants.PlatformOpenShift {
		podSecurityContext.RunAsUser = ptr.To(constants.UserBackup)
		podSecurityContext.RunAsGroup = ptr.To(constants.GroupBackup)
		podSecurityContext.FSGroup = ptr.To(constants.GroupBackup)
	}
	if cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled {
		podSecurityContext.AppArmorProfile = &corev1.AppArmorProfile{
			Type: corev1.AppArmorProfileTypeRuntimeDefault,
		}
	}

	return podSecurityContext
}

func buildUpgradeExecutorContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		ReadOnlyRootFilesystem: ptr.To(true),
		RunAsNonRoot:           ptr.To(true),
	}
}

func buildUpgradeExecutorResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

func buildUpgradeExecutorVolumeMounts(tlsTrust portopenbao.TrustBundleSource) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      upgradeTokenVolumeName,
			MountPath: upgradeTokenMountPath,
			ReadOnly:  true,
		},
	}
	if !tlsTrust.UseSystemRoots {
		mounts = append([]corev1.VolumeMount{
			{
				Name:      upgradeTLSCAVolumeName,
				MountPath: constants.PathTLS,
				ReadOnly:  true,
			},
		}, mounts...)
	}
	return mounts
}

func buildUpgradeExecutorVolumes(tlsTrust portopenbao.TrustBundleSource) []corev1.Volume {
	tokenFileMode := int32(0400)
	volumes := []corev1.Volume{
		{
			Name: upgradeTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              upgradeTokenFileRelativePath,
								ExpirationSeconds: ptr.To(int64(3600)),
								Audience:          auth.OpenBaoJWTAudience(),
							},
						},
					},
					DefaultMode: &tokenFileMode,
				},
			},
		},
	}
	if !tlsTrust.UseSystemRoots {
		volumes = append([]corev1.Volume{
			{
				Name: upgradeTLSCAVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tlsTrust.SecretName,
						Items: []corev1.KeyToPath{
							{
								Key:  tlsTrust.SecretKey,
								Path: "ca.crt",
							},
						},
					},
				},
			},
		}, volumes...)
	}
	return volumes
}
