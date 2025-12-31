package infra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

const (
	openBaoLivenessProbeTimeout  = "4s"
	openBaoReadinessProbeTimeout = "10s"
	openBaoStartupProbeTimeout   = "5s"

	unsealTypeStatic = "static"
)

var (
	// Use 127.0.0.1 instead of localhost to force IPv4, avoiding IPv6 resolution issues
	// where localhost might resolve to ::1 but OpenBao only listens on IPv4.
	openBaoProbeAddr = fmt.Sprintf("https://127.0.0.1:%d", constants.PortAPI)

	openBaoProbeCAFile = constants.PathTLSCACert
)

// ErrStatefulSetPrerequisitesMissing indicates that required prerequisites (ConfigMap or TLS Secret)
// are missing and the StatefulSet cannot be created. Callers can use this error to set a condition
// and requeue instead of failing reconciliation.
var ErrStatefulSetPrerequisitesMissing = errors.New("StatefulSet prerequisites missing")

// checkStatefulSetPrerequisites verifies that all required resources exist before creating or updating the StatefulSet.
// This prevents pods from failing to start due to missing ConfigMaps or Secrets.
// Returns ErrStatefulSetPrerequisitesMissing if prerequisites are not found (callers should handle this
// by setting a condition and requeuing). Returns other errors for unexpected failures.
func (m *Manager) checkStatefulSetPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, revision string) error {
	// Always check for the config ConfigMap
	configMapName := configMapNameWithRevision(cluster, revision)
	configMap := &corev1.ConfigMap{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      configMapName,
	}, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: config ConfigMap %s/%s not found; cannot create StatefulSet", ErrStatefulSetPrerequisitesMissing, cluster.Namespace, configMapName)
		}
		return fmt.Errorf("failed to get config ConfigMap %s/%s: %w", cluster.Namespace, configMapName, err)
	}

	// Check for TLS secret if TLS is enabled and not in ACME mode
	// In ACME mode, OpenBao manages certificates internally, so no secret is needed
	if cluster.Spec.TLS.Enabled && !usesACMEMode(cluster) {
		tlsSecretName := tlsServerSecretName(cluster)
		tlsSecret := &corev1.Secret{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      tlsSecretName,
		}, tlsSecret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: TLS server Secret %s/%s not found; cannot create StatefulSet (waiting for TLS reconciliation or external provider)", ErrStatefulSetPrerequisitesMissing, cluster.Namespace, tlsSecretName)
			}
			return fmt.Errorf("failed to get TLS server Secret %s/%s: %w", cluster.Namespace, tlsSecretName, err)
		}
	}

	return nil
}

// EnsureStatefulSetWithRevision manages the StatefulSet for the OpenBaoCluster using Server-Side Apply.
// This is exported for use by BlueGreenManager.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the verified init container image digest to use (if provided, overrides cluster.Spec.InitContainer.Image).
// revision is an optional revision identifier for blue/green deployments (e.g., "blue-v1hash" or "green-v2hash").
// disableSelfInit prevents the pod from attempting to initialize itself (used for Green pods that must join).
// If revision is empty, uses the cluster name (backward compatible behavior).
//
// Note: UpdateStrategy is intentionally not set here to allow UpgradeManager to manage it.
// SSA will preserve fields not specified in the desired object.
func (m *Manager) EnsureStatefulSetWithRevision(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string, verifiedImageDigest string, verifiedInitContainerDigest string, revision string, disableSelfInit bool) error {
	name := statefulSetNameWithRevision(cluster, revision)

	if err := m.ensureConfigMapWithRevision(ctx, cluster, revision, configContent); err != nil {
		return fmt.Errorf("failed to ensure config ConfigMap for StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	// Before creating/updating the StatefulSet, verify all prerequisites exist
	// This is important for External TLS mode where secrets might be deleted/recreated
	if err := m.checkStatefulSetPrerequisites(ctx, cluster, revision); err != nil {
		return err
	}

	initialized := cluster.Status.Initialized
	desiredReplicas := cluster.Spec.Replicas

	// If not initialized, keep at 1 replica until initialization completes
	if !initialized {
		desiredReplicas = 1
		logger.Info("Cluster not yet initialized; keeping StatefulSet at 1 replica", "statefulset", name)
	} else {
		logger.Info("Cluster initialized; ensuring StatefulSet has desired replicas",
			"statefulset", name,
			"desiredReplicas", desiredReplicas)
	}

	desired, buildErr := buildStatefulSetWithRevision(cluster, configContent, initialized, verifiedImageDigest, verifiedInitContainerDigest, revision, disableSelfInit)
	if buildErr != nil {
		return fmt.Errorf("failed to build StatefulSet for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, buildErr)
	}

	// Set the desired replica count (SSA will handle create/update)
	desired.Spec.Replicas = int32Ptr(desiredReplicas)

	// Set TypeMeta for SSA
	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}

	// Check if the StatefulSet already exists to preserve fields managed by other controllers (UpgradeManager)
	existing := &appsv1.StatefulSet{}
	err := m.client.Get(ctx, types.NamespacedName{Name: name, Namespace: cluster.Namespace}, existing)
	if err == nil {
		// If existing STS has a partition set, preserve it in our desired object.
		// The UpgradeManager sets this field via Patch/SSA, but if we don't include it here,
		// our SSA apply might interfere or try to reset it to nil (depending on ownership).
		// By explicitly including the current value, we tell SSA we are okay with this state.
		if existing.Spec.UpdateStrategy.RollingUpdate != nil && existing.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			if desired.Spec.UpdateStrategy.RollingUpdate == nil {
				desired.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
			}
			desired.Spec.UpdateStrategy.RollingUpdate.Partition = existing.Spec.UpdateStrategy.RollingUpdate.Partition
		}
	} else if !apierrors.IsNotFound(err) {
		// If error is something other than NotFound, return it
		return fmt.Errorf("failed to get existing StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	// Note: We intentionally do NOT set UpdateStrategy here. The UpgradeManager manages
	// UpdateStrategy (including RollingUpdate.Partition) for rollout orchestration.
	// SSA will preserve the existing UpdateStrategy if it's not specified in our desired object.

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

// getInitContainerImage returns the init container image to use.
func getInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.InitContainer != nil && cluster.Spec.InitContainer.Image != "" {
		return cluster.Spec.InitContainer.Image
	}
	return ""
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

func buildInitContainers(cluster *openbaov1alpha1.OpenBaoCluster, verifiedInitContainerDigest string, disableSelfInit bool) []corev1.Container {
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

	initImage := getInitContainerImage(cluster)
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
	}
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

	// Add GCP credentials environment variable when using GCP Cloud KMS
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type == "gcpckms" {
		if cluster.Spec.Unseal.CredentialsSecretRef != nil {
			// Mount GCP credentials JSON file and set GOOGLE_APPLICATION_CREDENTIALS
			// The credentials secret must contain a key named "credentials.json" with the
			// GCP service account JSON credentials. This will be mounted at
			// /etc/bao/seal-creds/credentials.json and referenced by the environment variable.
			env = append(env, corev1.EnvVar{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: "/etc/bao/seal-creds/credentials.json",
			})
		}
	}

	// Add VAULT_TOKEN environment variable when using transit seal with credentials
	// This allows the seal to use the "token" parameter instead of "token_file",
	// avoiding issues with trailing newlines in mounted Secret files.
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type == "transit" {
		if cluster.Spec.Unseal.CredentialsSecretRef != nil {
			// Read token from the mounted secret file and set as VAULT_TOKEN
			// The token will be read from /etc/bao/seal-creds/token
			env = append(env, corev1.EnvVar{
				Name: "VAULT_TOKEN",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "token",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: cluster.Spec.Unseal.CredentialsSecretRef.Name,
						},
					},
				},
			})

			// If the credentials Secret provides a CA bundle, surface it via VAULT_CACERT
			// so transit seal HTTP calls verify infra-bao TLS.
			env = append(env, corev1.EnvVar{
				Name:  "VAULT_CACERT",
				Value: "/etc/bao/seal-creds/ca.crt",
			})
		}
	}

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

	// Only mount unseal volume when using static seal
	if usesStaticSeal(cluster) {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      unsealVolumeName,
			MountPath: openBaoUnsealMountPath,
			ReadOnly:  true,
		})
	}

	// Mount seal credentials volume when using external KMS with credentials
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type != "" && cluster.Spec.Unseal.Type != unsealTypeStatic {
		if cluster.Spec.Unseal.CredentialsSecretRef != nil {
			mounts = append(mounts, corev1.VolumeMount{
				Name:      "seal-creds",
				MountPath: "/etc/bao/seal-creds",
				ReadOnly:  true,
			})
		}
	}

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
func buildContainers(cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string, renderedConfigDir string, startupProbeExec *corev1.ExecAction, livenessProbeExec *corev1.ExecAction, readinessProbeExec *corev1.ExecAction) []corev1.Container {
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
					Exec: startupProbeExec,
				},
				TimeoutSeconds:   10,
				PeriodSeconds:    5,
				FailureThreshold: 60,
			},
			LivenessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: livenessProbeExec,
				},
				TimeoutSeconds:   5,
				PeriodSeconds:    10,
				FailureThreshold: 6,
			},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					Exec: readinessProbeExec,
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
	// Use the helper function from manager.go
	labels := podSelectorLabelsWithRevision(cluster, revision)

	replicas := cluster.Spec.Replicas
	// During initial cluster creation, start with 1 replica for initialization
	// The init manager will scale up after initialization completes
	if !initialized {
		replicas = 1
	}

	size, err := resource.ParseQuantity(cluster.Spec.Storage.Size)
	if err != nil {
		return nil, fmt.Errorf("invalid storage size %q: %w", cluster.Spec.Storage.Size, err)
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

	podLabels := podSelectorLabelsWithRevision(cluster, revision)

	// Compute config hash and add to annotations to trigger rollout on config changes
	configHash := computeConfigHash(configContent)
	if podLabels == nil {
		podLabels = make(map[string]string)
	}
	annotations := map[string]string{
		configHashAnnotation: configHash,
	}

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

	renderedConfigDir := path.Dir(openBaoRenderedConfig)

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

	// Only add unseal volume if using static seal
	if usesStaticSeal(cluster) {
		volumes = append(volumes, corev1.Volume{
			Name: unsealVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  unsealSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		})
	}

	// Add seal credentials volume if using external KMS with credentials
	if cluster.Spec.Unseal != nil && cluster.Spec.Unseal.Type != "" && cluster.Spec.Unseal.Type != unsealTypeStatic {
		if cluster.Spec.Unseal.CredentialsSecretRef != nil {
			volumes = append(volumes, corev1.Volume{
				Name: "seal-creds",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  cluster.Spec.Unseal.CredentialsSecretRef.Name,
						DefaultMode: ptr.To(secretFileMode),
					},
				},
			})
		}
	}

	// If self-init is enabled, add the self-init ConfigMap volume
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

	// Use the helper function from manager.go
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
			// For blue/green deployments, use OnDelete update strategy to preventing
			// automatic rolling updates. The BlueGreenManager controls when pods are created/updated.
			// For standard rolling upgrades, use RollingUpdate (default behavior).
			UpdateStrategy: func() appsv1.StatefulSetUpdateStrategy {
				if cluster.Spec.UpdateStrategy.Type == openbaov1alpha1.UpdateStrategyBlueGreen {
					return appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.OnDeleteStatefulSetStrategyType,
					}
				}
				return appsv1.StatefulSetUpdateStrategy{
					Type: appsv1.RollingUpdateStatefulSetStrategyType,
				}
			}(),
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
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(openBaoUserID),
						RunAsGroup:   ptr.To(openBaoGroupID),
						FSGroup:      ptr.To(openBaoGroupID),
						// Enforce a secure seccomp profile to limit available system calls
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					InitContainers: buildInitContainers(cluster, verifiedInitContainerDigest, disableSelfInit),
					Containers:     buildContainers(cluster, verifiedImageDigest, renderedConfigDir, startupProbeExec, livenessProbeExec, readinessProbeExec),
					Volumes:        volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
	}

	if cluster.Spec.WorkloadHardening != nil && cluster.Spec.WorkloadHardening.AppArmorEnabled {
		statefulSet.Spec.Template.Spec.SecurityContext.AppArmorProfile = &corev1.AppArmorProfile{
			Type: corev1.AppArmorProfileTypeRuntimeDefault,
		}
	}

	return statefulSet, nil
}

// deleteStatefulSet removes the StatefulSet for the OpenBaoCluster.
func (m *Manager) deleteStatefulSet(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	statefulSet := &appsv1.StatefulSet{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, statefulSet)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := m.client.Delete(ctx, statefulSet); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// deletePods removes all Pods associated with the OpenBaoCluster.
// This ensures pods are cleaned up even if they become orphaned after StatefulSet deletion.
func (m *Manager) deletePods(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	var podList corev1.PodList
	if err := m.client.List(ctx, &podList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(podSelectorLabels(cluster)),
	); err != nil {
		// If RBAC does not permit listing pods (for example, tenant Roles have
		// been removed or were never provisioned), treat this as a best-effort
		// condition rather than blocking finalizer removal. At this point the
		// StatefulSet is being deleted and kubelet is allowed to delete pods via
		// the resource lock webhook, so lingering pods will still be cleaned up.
		if apierrors.IsForbidden(err) {
			logger.Info("Skipping pod cleanup during deletion due to missing list permission",
				"namespace", cluster.Namespace,
				"cluster", cluster.Name)
			return nil
		}
		return fmt.Errorf("failed to list pods for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}

	for i := range podList.Items {
		pod := &podList.Items[i]
		logger.Info("Deleting pod during cleanup", "pod", pod.Name)
		if err := m.client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	return nil
}

// deletePVCs removes all PersistentVolumeClaims associated with the OpenBaoCluster.
func (m *Manager) deletePVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := m.client.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelOpenBaoCluster: cluster.Name,
		}),
	); err != nil {
		return err
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := m.client.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
