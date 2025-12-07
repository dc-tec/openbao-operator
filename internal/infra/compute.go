package infra

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
)

// ensureStatefulSet manages the StatefulSet for the OpenBaoCluster.
func (m *Manager) ensureStatefulSet(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string) error {
	name := statefulSetName(cluster)

	statefulSet := &appsv1.StatefulSet{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      name,
	}, statefulSet)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get StatefulSet %s/%s: %w", cluster.Namespace, name, err)
		}

		logger.Info("StatefulSet not found; creating", "statefulset", name)

		// During initial cluster creation, start with 1 replica for initialization
		initialized := cluster.Status.Initialized
		desired, buildErr := buildStatefulSet(cluster, configContent, initialized)
		if buildErr != nil {
			return fmt.Errorf("failed to build StatefulSet for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, buildErr)
		}

		// Set OwnerReference for garbage collection when the OpenBaoCluster is deleted.
		if err := controllerutil.SetControllerReference(cluster, desired, m.scheme); err != nil {
			return fmt.Errorf("failed to set owner reference on StatefulSet %s/%s: %w", cluster.Namespace, name, err)
		}

		if err := m.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create StatefulSet %s/%s: %w", cluster.Namespace, name, err)
		}

		return nil
	}

	initialized := cluster.Status.Initialized
	desiredReplicas := cluster.Spec.Replicas
	currentReplicas := int32Ptr(1)
	if statefulSet.Spec.Replicas != nil {
		currentReplicas = statefulSet.Spec.Replicas
	}

	// If not initialized, keep at 1 replica until initialization completes
	if !initialized {
		desiredReplicas = 1
		logger.Info("Cluster not yet initialized; keeping StatefulSet at 1 replica", "statefulset", name)
	} else {
		logger.Info("Cluster initialized; scaling StatefulSet to desired replicas",
			"statefulset", name,
			"currentReplicas", *currentReplicas,
			"desiredReplicas", desiredReplicas)
	}

	desired, buildErr := buildStatefulSet(cluster, configContent, initialized)
	if buildErr != nil {
		return fmt.Errorf("failed to build desired StatefulSet for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, buildErr)
	}

	// The UpgradeManager owns StatefulSet update strategy (including
	// RollingUpdate.Partition) for rollout orchestration. The infra manager must
	// avoid mutating UpdateStrategy to prevent clobbering in-progress upgrades.
	updated := statefulSet.DeepCopy()
	updated.Labels = desired.Labels
	updated.Spec.Replicas = int32Ptr(desiredReplicas)
	updated.Spec.ServiceName = desired.Spec.ServiceName
	updated.Spec.Selector = desired.Spec.Selector
	updated.Spec.Template = desired.Spec.Template
	updated.Spec.VolumeClaimTemplates = desired.Spec.VolumeClaimTemplates

	if err := m.client.Update(ctx, updated); err != nil {
		return fmt.Errorf("failed to update StatefulSet %s/%s: %w", cluster.Namespace, name, err)
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

// getOpenBaoConfigPath returns the path to the OpenBao configuration file.
func getOpenBaoConfigPath(_ *openbaov1alpha1.OpenBaoCluster) string {
	return openBaoRenderedConfig
}

// computeConfigHash computes a SHA256 hash of the config content for change detection.
func computeConfigHash(configContent string) string {
	sum := sha256.Sum256([]byte(configContent))
	return hex.EncodeToString(sum[:])
}

func buildInitContainers(cluster *openbaov1alpha1.OpenBaoCluster) []corev1.Container {
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
	}

	// If self-init is enabled, mount the self-init ConfigMap and pass the path to the init container
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled {
		selfInitPath := configInitTemplatePath
		args = append(args, "--self-init", selfInitPath)
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      configInitVolumeName,
			MountPath: path.Dir(configInitTemplatePath),
			ReadOnly:  true,
		})
	}

	return []corev1.Container{
		{
			Name:  "bao-config-init",
			Image: getInitContainerImage(cluster),
			// The init container is responsible for rendering the final
			// config.hcl from the template using environment variables
			// such as HOSTNAME and POD_IP. It writes the result to a
			// shared volume mounted at openBaoRenderedConfig.
			// If self-init is enabled, it also appends self-init blocks
			// for pod-0.
			Command: []string{"/bao-init-config"},
			Args:    args,
			Env: []corev1.EnvVar{
				{
					Name: "HOSTNAME",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.name",
						},
					},
				},
				{
					Name: "POD_IP",
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

// buildStatefulSet constructs a StatefulSet for the given OpenBaoCluster.
func buildStatefulSet(cluster *openbaov1alpha1.OpenBaoCluster, configContent string, initialized bool) (*appsv1.StatefulSet, error) {
	labels := podSelectorLabels(cluster)

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

	podLabels := podSelectorLabels(cluster)

	// Compute config hash and add to annotations to trigger rollout on config changes
	configHash := computeConfigHash(configContent)
	if podLabels == nil {
		podLabels = make(map[string]string)
	}
	annotations := map[string]string{
		configHashAnnotation: configHash,
	}

	probeHTTPGet := &corev1.HTTPGetAction{
		Path:   openBaoHealthPath,
		Port:   intstr.FromInt(openBaoContainerPort),
		Scheme: corev1.URISchemeHTTPS,
	}

	renderedConfigDir := path.Dir(openBaoRenderedConfig)

	volumes := []corev1.Volume{
		{
			Name: tlsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  tlsServerSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		},
		{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName(cluster),
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
			Name: unsealVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  unsealSecretName(cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		},
		{
			Name: tmpVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// If self-init is enabled, add the self-init ConfigMap volume
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled {
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

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName(cluster),
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: headlessServiceName(cluster),
			Replicas:    int32Ptr(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels,
					Annotations: annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName(cluster),
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
					InitContainers: buildInitContainers(cluster),
					Containers: []corev1.Container{
						{
							Name:  openBaoContainerName,
							Image: cluster.Spec.Image,
							SecurityContext: &corev1.SecurityContext{
								// Prevent privilege escalation (sudo, setuid binaries)
								AllowPrivilegeEscalation: ptr.To(false),
								// Drop ALL capabilities. OpenBao does not need them if mlock is disabled.
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// Read-only root filesystem. Attackers cannot write tools/scripts to the container disk.
								// OpenBao writes to mounted volumes (/bao/data, /etc/bao/config, etc.) which are already mounted.
								ReadOnlyRootFilesystem: ptr.To(true),
							},
							Command: []string{
								openBaoBinaryName,
								"server",
								fmt.Sprintf("-config=%s", getOpenBaoConfigPath(cluster)),
							},
							Env: []corev1.EnvVar{
								{
									Name: "HOSTNAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.name",
										},
									},
								},
								{
									Name: "POD_IP",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "status.podIP",
										},
									},
								},
								{
									Name:  "BAO_API_ADDR",
									Value: fmt.Sprintf("https://$(POD_IP):%d", openBaoContainerPort),
								},
								{
									// Set umask to 0077 to ensure Raft FSM database files are created
									// with 0600 permissions (owner read/write only) instead of 0660.
									// This matches OpenBao's security expectations for sensitive data files.
									Name:  "UMASK",
									Value: "0077",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									Name:          "api",
									ContainerPort: int32(openBaoContainerPort),
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "cluster",
									ContainerPort: int32(openBaoClusterPort),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      tlsVolumeName,
									MountPath: openBaoTLSMountPath,
									ReadOnly:  true,
								},
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
									Name:      unsealVolumeName,
									MountPath: openBaoUnsealMountPath,
									ReadOnly:  true,
								},
								{
									Name:      dataVolumeName,
									MountPath: openBaoDataPath,
								},
								{
									Name:      tmpVolumeName,
									MountPath: "/tmp",
								},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: probeHTTPGet,
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: probeHTTPGet,
								},
							},
						},
					},
					Volumes: volumes,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				pvc,
			},
		},
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

// deletePVCs removes all PersistentVolumeClaims associated with the OpenBaoCluster.
func (m *Manager) deletePVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := m.client.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			labelOpenBaoCluster: cluster.Name,
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
