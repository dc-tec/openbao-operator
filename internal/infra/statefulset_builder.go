package infra

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

const (
	openBaoLivenessProbeTimeout  = "4s"
	openBaoReadinessProbeTimeout = "10s"
	openBaoStartupProbeTimeout   = "5s"
)

var (
	// Use 127.0.0.1 instead of localhost to force IPv4, avoiding IPv6 resolution issues
	// where localhost might resolve to ::1 but OpenBao only listens on IPv4.
	openBaoProbeAddr = fmt.Sprintf("https://127.0.0.1:%d", constants.PortAPI)

	openBaoProbeCAFile = constants.PathTLSCACert
)

type probeExecActions struct {
	startup   *corev1.ExecAction
	liveness  *corev1.ExecAction
	readiness *corev1.ExecAction
}

// getInitContainerImage returns the init container image to use.
// If not specified in the cluster spec, returns the default image derived from
// OPERATOR_INIT_IMAGE_REPOSITORY and OPERATOR_VERSION environment variables.
func getInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.InitContainer != nil && cluster.Spec.InitContainer.Image != "" {
		return cluster.Spec.InitContainer.Image, nil
	}
	return constants.DefaultInitImage()
}

// getContainerImage returns the container image to use for the OpenBao container.
// If verifiedImageDigest is provided, it is used to prevent TOCTOU attacks.
// Otherwise, cluster.Spec.Image is used.
func getContainerImage(cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) string {
	if verifiedImageDigest != "" {
		return verifiedImageDigest
	}
	return cluster.Spec.Image
}

// getOpenBaoConfigPath returns the path to the OpenBao configuration file.
func getOpenBaoConfigPath(_ *openbaov1alpha1.OpenBaoCluster) string {
	return openBaoRenderedConfig
}

// computeConfigHash computes a SHA256 hash of the config content for change detection.
func computeConfigHash(configContent string) string {
	sum := sha256.Sum256([]byte(configContent))
	return hex.EncodeToString(sum[:])
}

func buildInitContainers(cluster *openbaov1alpha1.OpenBaoCluster, verifiedInitContainerDigest string, disableSelfInit bool) ([]corev1.Container, error) {
	renderedConfigDir := path.Dir(openBaoRenderedConfig)

	args := []string{
		"--template", configTemplatePath,
		"--output", openBaoRenderedConfig,
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      configVolumeName,
			MountPath: openBaoConfigMountPath,
			ReadOnly:  true,
		},
		{
			Name:      configRenderedVolumeName,
			MountPath: renderedConfigDir,
		},
		{
			Name:      utilsVolumeName,
			MountPath: "/utils",
		},
	}

	// If self-init is enabled, mount the self-init ConfigMap and pass the path to the init container
	// But check disableSelfInit first (e.g. for Green pods which should join instead of ensure self-init)
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled && !disableSelfInit {
		selfInitPath := configInitTemplatePath
		args = append(args, "--self-init", selfInitPath)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      configInitVolumeName,
			MountPath: path.Dir(configInitTemplatePath),
			ReadOnly:  true,
		})
	}

	initImage, err := getInitContainerImage(cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get init container image: %w", err)
	}
	if verifiedInitContainerDigest != "" {
		initImage = verifiedInitContainerDigest
	}

	return []corev1.Container{
		{
			Name:  "bao-config-init",
			Image: initImage,
			// The init container is responsible for rendering the final
			// config.hcl from the template using environment variables
			// such as HOSTNAME and POD_IP. It writes the result to a
			// shared volume mounted at openBaoRenderedConfig.
			// If self-init is enabled, it also appends self-init blocks
			// for pod-0.
			SecurityContext: &corev1.SecurityContext{
				// Prevent privilege escalation (sudo, setuid binaries)
				AllowPrivilegeEscalation: ptr.To(false),
				// Drop ALL capabilities.
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				// Read-only root filesystem to prevent runtime modification
				ReadOnlyRootFilesystem: ptr.To(true),
				// Run as non-root (inherited from PodSecurityContext, but explicit here is safe)
				RunAsNonRoot: ptr.To(true),
			},
			// Use bao-init-config to copy wrapper and render config (no shell needed)
			Command: []string{"/bao-init-config"},
			Args: append([]string{
				"-copy-wrapper=/bao-wrapper",
				"-copy-probe=/bao-probe",
			}, args...),
			Env: []corev1.EnvVar{
				{
					Name: constants.EnvHostname,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
				{
					Name: constants.EnvPodIP,
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "status.podIP",
						},
					},
				},
			},
			VolumeMounts: volumeMounts,
		},
	}, nil
}

// buildContainerEnv builds the environment variables for the OpenBao container.
// It includes standard variables and conditionally adds GCP credentials path
// when using GCP Cloud KMS seal.
func buildContainerEnv(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name: constants.EnvHostname,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			// Required for OpenBao Kubernetes service registration.
			Name: constants.EnvBaoK8sPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			// Required for OpenBao Kubernetes service registration.
			Name: constants.EnvBaoK8sNamespace,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name: constants.EnvPodIP,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		{
			Name:  constants.EnvBaoAPIAddr,
			Value: fmt.Sprintf("https://$(%s):%d", constants.EnvPodIP, constants.PortAPI),
		},
		{
			// Set umask to 0077 to ensure Raft FSM database files are created
			// with 0600 permissions (owner read/write only) instead of 0660.
			// This matches OpenBao's security expectations for sensitive data files.
			Name:  "UMASK",
			Value: "0077",
		},
	}

	env = append(env, newSealWiringProvider(cluster).EnvVars()...)

	return env
}

// usesACMEMode returns true if the cluster is configured to use ACME for TLS.
func usesACMEMode(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.TLS.Enabled && cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeACME
}

// buildContainerVolumeMounts builds the volume mounts for the OpenBao container.
// It conditionally includes the unseal volume mount only when using static seal.
// It conditionally excludes the TLS volume mount when using ACME mode.
func buildContainerVolumeMounts(cluster *openbaov1alpha1.OpenBaoCluster, renderedConfigDir string) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{
			Name:      configVolumeName,
			MountPath: openBaoConfigMountPath,
			ReadOnly:  true,
		},
		{
			Name:      configRenderedVolumeName,
			MountPath: renderedConfigDir,
		},
		{
			Name:      dataVolumeName,
			MountPath: openBaoDataPath,
		},
		{
			Name:      tmpVolumeName,
			MountPath: "/tmp",
		},
	}

	// Mount the ServiceAccount token only into the OpenBao container. We disable
	// automounting at the Pod level and instead use an explicit projected volume
	// to minimize token exposure.
	mounts = append(mounts, corev1.VolumeMount{
		Name:      kubeAPIAccessVolumeName,
		MountPath: serviceAccountMountPath,
		ReadOnly:  true,
	})

	// Only mount TLS volume when not using ACME mode (ACME stores certs in /bao/data)
	if !usesACMEMode(cluster) {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      tlsVolumeName,
			MountPath: openBaoTLSMountPath,
			ReadOnly:  true,
		})
	}

	mounts = append(mounts, newSealWiringProvider(cluster).VolumeMounts()...)

	// Add utils volume mount (Read-Only for security)
	mounts = append(mounts, corev1.VolumeMount{
		Name:      utilsVolumeName,
		MountPath: "/utils",
		ReadOnly:  true,
	})

	return mounts
}

// buildContainers builds the container list for the OpenBao pod.
// The OpenBao container uses a wrapper binary as the entrypoint that manages
// the OpenBao process and watches for TLS certificate changes.
func buildContainers(cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string, renderedConfigDir string, probes probeExecActions) []corev1.Container {
	// Add utils volume mount (Read-Only for security)
	mainVolumeMounts := buildContainerVolumeMounts(cluster, renderedConfigDir)

	// Construct the wrapper command
	// We pass the actual OpenBao command as arguments to the wrapper
	cmd := []string{constants.PathWrapperBinary}

	// Configure wrapper args
	args := []string{}

	// If not using ACME, watch the TLS certificate
	if !usesACMEMode(cluster) {
		args = append(args, fmt.Sprintf("-watch-file=%s/tls.crt", constants.PathTLS))
	}

	// Separator for the child command
	args = append(args, "--")

	// The actual OpenBao command
	args = append(args, openBaoBinaryName, "server", fmt.Sprintf("-config=%s", getOpenBaoConfigPath(cluster)))

	containers := []corev1.Container{
		{
			Name:  constants.ContainerBao,
			Image: getContainerImage(cluster, verifiedImageDigest),
			SecurityContext: &corev1.SecurityContext{
				// Prevent privilege escalation (sudo, setuid binaries)
				AllowPrivilegeEscalation: ptr.To(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				// Read-only root filesystem. Attackers cannot write tools/scripts to the container disk.
				// OpenBao writes to mounted volumes (/bao/data, /etc/bao/config, etc.) which are already mounted.
				ReadOnlyRootFilesystem: ptr.To(true),
			},
			Command: cmd,
			Args:    args,
			Env:     buildContainerEnv(cluster),
			Ports: []corev1.ContainerPort{
				{
					Name:          "api",
					ContainerPort: int32(constants.PortAPI),
					Protocol:      corev1.ProtocolTCP,
				},
				{
					Name:          "cluster",
					ContainerPort: int32(constants.PortCluster),
					Protocol:      corev1.ProtocolTCP,
				},
			},
			VolumeMounts: mainVolumeMounts,
			StartupProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: probes.startup,
				},
				TimeoutSeconds:   10,
				PeriodSeconds:    5,
				FailureThreshold: 60,
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: probes.liveness,
				},
				TimeoutSeconds:   5,
				PeriodSeconds:    10,
				FailureThreshold: 6,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: probes.readiness,
				},
				InitialDelaySeconds: 5,
				TimeoutSeconds:      10,
				PeriodSeconds:       10,
				FailureThreshold:    6,
			},
		},
	}

	return containers
}

func desiredStatefulSetReplicas(cluster *openbaov1alpha1.OpenBaoCluster, initialized bool) int32 {
	replicas := cluster.Spec.Replicas
	if !initialized {
		replicas = 1
	}
	return replicas
}

func buildStatefulSetPodSecurityContext(cluster *openbaov1alpha1.OpenBaoCluster) *corev1.PodSecurityContext {
	securityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: ptr.To(true),
		RunAsUser:    ptr.To(openBaoUserID),
		RunAsGroup:   ptr.To(openBaoGroupID),
		FSGroup:      ptr.To(openBaoGroupID),
		// Enforce a secure seccomp profile to limit available system calls
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
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
	// For standard rolling upgrades, use RollingUpdate (default behavior).
	if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
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
			Labels: infraLabels(cluster),
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
	podLabels := podSelectorLabelsWithRevision(cluster, revision)

	// Compute config hash and add to annotations to trigger rollout on config changes
	configHash := computeConfigHash(configContent)
	if podLabels == nil {
		podLabels = make(map[string]string)
	}
	annotations := map[string]string{
		configHashAnnotation: configHash,
	}

	return podLabels, annotations
}

func buildStatefulSetProbeExecActions(cluster *openbaov1alpha1.OpenBaoCluster) probeExecActions {
	// Probe target/CA: by default use loopback and the per-cluster TLS CA.
	probeAddr := openBaoProbeAddr
	probeCAFile := openBaoProbeCAFile
	var probeServerName string
	if usesACMEMode(cluster) && cluster.Spec.TLS.ACME != nil && cluster.Spec.TLS.ACME.Domain != "" {
		// In ACME mode, keep probes on loopback but set SNI to the ACME domain.
		// This prevents OpenBao from attempting ACME for "localhost" while avoiding
		// DNS/service dependencies for probes.
		probeServerName = cluster.Spec.TLS.ACME.Domain
		// In ACME mode, probes need to verify the ACME-obtained certificate, which is signed
		// by the ACME CA (PKI CA), not the ACME directory server's TLS CA.
		// If tls_acme_ca_root is configured, derive the PKI CA path from it (same directory,
		// filename pki-ca.crt). This allows users to provide the PKI CA in the same volume.
		// If not available, use system roots (for public ACME CAs like Let's Encrypt).
		if cluster.Spec.Configuration != nil && cluster.Spec.Configuration.ACMECARoot != "" {
			// Derive PKI CA path from tls_acme_ca_root: same directory, filename pki-ca.crt
			// e.g., /etc/bao/seal-creds/ca.crt -> /etc/bao/seal-creds/pki-ca.crt
			acmeCARootDir := path.Dir(cluster.Spec.Configuration.ACMECARoot)
			probeCAFile = path.Join(acmeCARootDir, "pki-ca.crt")
		} else {
			// No tls_acme_ca_root configured - use system roots for public ACME CAs
			probeCAFile = ""
		}
	}

	// Startup probe only does TCP dial, so it doesn't need a CA file.
	// In ACME mode, the default CA file (/etc/bao/tls/ca.crt) doesn't exist, so
	// explicitly set -ca-file="" to avoid trying to read it.
	startupProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=startup",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoStartupProbeTimeout,
	}
	// Only set empty CA file for ACME mode where the default CA file won't exist
	if usesACMEMode(cluster) {
		startupProbeCmd = append(startupProbeCmd, "-ca-file=")
	}
	startupProbeExec := &corev1.ExecAction{
		Command: startupProbeCmd,
	}

	livenessProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=liveness",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoLivenessProbeTimeout,
	}
	if probeServerName != "" {
		livenessProbeCmd = append(livenessProbeCmd, "-servername="+probeServerName)
	}
	if probeCAFile != "" {
		livenessProbeCmd = append(livenessProbeCmd, "-ca-file="+probeCAFile)
	}
	livenessProbeExec := &corev1.ExecAction{
		Command: livenessProbeCmd,
	}

	readinessProbeCmd := []string{
		constants.PathProbeBinary,
		"-mode=readiness",
		"-addr=" + probeAddr,
		"-timeout=" + openBaoReadinessProbeTimeout,
	}
	if probeServerName != "" {
		readinessProbeCmd = append(readinessProbeCmd, "-servername="+probeServerName)
	}
	if probeCAFile != "" {
		readinessProbeCmd = append(readinessProbeCmd, "-ca-file="+probeCAFile)
	}
	readinessProbeExec := &corev1.ExecAction{
		Command: readinessProbeCmd,
	}

	return probeExecActions{
		startup:   startupProbeExec,
		liveness:  livenessProbeExec,
		readiness: readinessProbeExec,
	}
}

func buildStatefulSetVolumes(cluster *openbaov1alpha1.OpenBaoCluster, revision string, disableSelfInit bool) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapNameWithRevision(cluster, revision),
					},
				},
			},
		},
		{
			Name: configRenderedVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: tmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: utilsVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: kubeAPIAccessVolumeName,
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					DefaultMode: ptr.To(serviceAccountFileMode),
					Sources: []corev1.VolumeProjection{
						{
							ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
								Path:              "token",
								ExpirationSeconds: ptr.To(serviceAccountTokenExpirationSeconds),
							},
						},
						{
							ConfigMap: &corev1.ConfigMapProjection{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: kubeRootCAConfigMapName,
								},
								Items: []corev1.KeyToPath{
									{
										Key:  "ca.crt",
										Path: "ca.crt",
									},
								},
							},
						},
						{
							DownwardAPI: &corev1.DownwardAPIProjection{
								Items: []corev1.DownwardAPIVolumeFile{
									{
										Path: "namespace",
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Only add TLS volume when not using ACME mode (ACME stores certs in /bao/data)
	if !usesACMEMode(cluster) {
		volumes = append(volumes, corev1.Volume{
			Name: tlsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsServerSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		})
	}

	volumes = append(volumes, newSealWiringProvider(cluster).Volumes()...)

	// If self-init is enabled, add the self-init ConfigMap volume, unless disabled (Green pods)
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled && !disableSelfInit {
		volumes = append(volumes, corev1.Volume{
			Name: configInitVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configInitMapName(cluster),
					},
				},
			},
		})
	}

	return volumes
}

// buildStatefulSet constructs a StatefulSet for the given OpenBaoCluster.
// This is a convenience wrapper that calls buildStatefulSetWithRevision with an empty revision.
//
//nolint:unparam // configContent varies in production with actual config values
func buildStatefulSet(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool, verifiedImageDigest string, verifiedInitContainerDigest string) (*appsv1.StatefulSet, error) {
	return buildStatefulSetWithRevision(cluster, configContent, initialized, verifiedImageDigest, verifiedInitContainerDigest, "", false)
}

// buildStatefulSetWithRevision constructs a StatefulSet for the given OpenBaoCluster.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the verified init container image digest to use (if provided, overrides cluster.Spec.InitContainer.Image).
// revision is an optional revision identifier for blue/green deployments.
// disableSelfInit prevents adding self-init logic (used for Green pods).
func buildStatefulSetWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool, verifiedImageDigest string, verifiedInitContainerDigest string, revision string, disableSelfInit bool) (*appsv1.StatefulSet, error) {
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
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
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
					SecurityContext:              buildStatefulSetPodSecurityContext(cluster),
					InitContainers:               initContainers,
					Containers:                   buildContainers(cluster, verifiedImageDigest, renderedConfigDir, probes),
					Volumes:                      volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
	}

	return statefulSet, nil
}
