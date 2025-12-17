package infra

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/openbao/operator/api/v1alpha1"
	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/internal/provisioner"
)

// getSentinelImage returns the Sentinel image to use.
// If cluster.Spec.Sentinel.Image is set, it is used.
// Otherwise, derives from operator version (OPERATOR_VERSION env var).
// Falls back to "openbao/operator-sentinel:latest" if version is not available.
func getSentinelImage(cluster *openbaov1alpha1.OpenBaoCluster, logger logr.Logger) string {
	if cluster.Spec.Sentinel != nil && cluster.Spec.Sentinel.Image != "" {
		return cluster.Spec.Sentinel.Image
	}

	// Derive from operator version
	operatorVersion := os.Getenv(constants.EnvOperatorVersion)
	if operatorVersion != "" {
		return fmt.Sprintf("openbao/operator-sentinel:%s", operatorVersion)
	}

	// Fallback (should not happen in production)
	logger.Info("OPERATOR_VERSION not set; using latest tag for Sentinel image. This should not happen in production.",
		"fallback_image", "openbao/operator-sentinel:latest")
	return "openbao/operator-sentinel:latest"
}

// ensureSentinelServiceAccount creates the ServiceAccount for the Sentinel.
func (m *Manager) ensureSentinelServiceAccount(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelServiceAccountName,
			Namespace: cluster.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
				constants.LabelAppInstance:    cluster.Name,
				constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
				constants.LabelAppComponent:   "sentinel",
				constants.LabelOpenBaoCluster: cluster.Name,
			},
		},
	}

	if err := m.applyResource(ctx, sa, cluster); err != nil {
		return fmt.Errorf("failed to ensure Sentinel ServiceAccount %s/%s: %w", cluster.Namespace, constants.SentinelServiceAccountName, err)
	}

	logger.Info("Ensured Sentinel ServiceAccount", "serviceaccount", constants.SentinelServiceAccountName)
	return nil
}

// ensureSentinelRBAC creates the Role and RoleBinding for the Sentinel.
func (m *Manager) ensureSentinelRBAC(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	role := provisioner.GenerateSentinelRole(cluster.Namespace)
	role.TypeMeta = metav1.TypeMeta{
		Kind:       "Role",
		APIVersion: "rbac.authorization.k8s.io/v1",
	}

	if err := m.applyResource(ctx, role, cluster); err != nil {
		return fmt.Errorf("failed to ensure Sentinel Role %s/%s: %w", cluster.Namespace, constants.SentinelRoleName, err)
	}

	roleBinding := provisioner.GenerateSentinelRoleBinding(cluster.Namespace)
	roleBinding.TypeMeta = metav1.TypeMeta{
		Kind:       "RoleBinding",
		APIVersion: "rbac.authorization.k8s.io/v1",
	}

	if err := m.applyResource(ctx, roleBinding, cluster); err != nil {
		return fmt.Errorf("failed to ensure Sentinel RoleBinding %s/%s: %w", cluster.Namespace, constants.SentinelRoleBindingName, err)
	}

	logger.Info("Ensured Sentinel RBAC", "role", constants.SentinelRoleName, "rolebinding", constants.SentinelRoleBindingName)
	return nil
}

// ensureSentinelDeployment creates the Deployment for the Sentinel.
func (m *Manager) ensureSentinelDeployment(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) error {
	deploymentName := cluster.Name + constants.SentinelDeploymentNameSuffix

	// Get Sentinel image
	sentinelImage := getSentinelImage(cluster, logger)
	if verifiedImageDigest != "" {
		sentinelImage = verifiedImageDigest
	}

	// Get resources with defaults
	resources := cluster.Spec.Sentinel.Resources
	if resources == nil {
		resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("64Mi"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
				corev1.ResourceCPU:    resource.MustParse("200m"),
			},
		}
	}

	// Get debounce window
	debounceWindowSeconds := int32(2) // Default
	if cluster.Spec.Sentinel.DebounceWindowSeconds != nil {
		debounceWindowSeconds = *cluster.Spec.Sentinel.DebounceWindowSeconds
	}

	labels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelAppComponent:   "sentinel",
		constants.LabelOpenBaoCluster: cluster.Name,
	}

	deployment := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: cluster.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)), // Sentinel is stateless, single replica
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: constants.SentinelServiceAccountName,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(true),
						RunAsUser:    ptr.To(constants.UserNonRoot),
						RunAsGroup:   ptr.To(constants.UserNonRoot),
						FSGroup:      ptr.To(constants.UserNonRoot),
						// Enforce a secure seccomp profile to limit available system calls
						// (required for restricted Pod Security Standard)
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "sentinel",
							Image: sentinelImage,
							SecurityContext: &corev1.SecurityContext{
								// Prevent privilege escalation (required for restricted Pod Security Standard)
								AllowPrivilegeEscalation: ptr.To(false),
								// Drop ALL capabilities (required for restricted Pod Security Standard)
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// Seccomp profile is set at pod level, but explicit here for clarity
								SeccompProfile: &corev1.SeccompProfile{
									Type: corev1.SeccompProfileTypeRuntimeDefault,
								},
								// Run as non-root (inherited from PodSecurityContext, but explicit here)
								RunAsNonRoot: ptr.To(true),
							},
							Command: []string{
								"/sentinel",
							},
							Env: []corev1.EnvVar{
								{
									Name: constants.EnvPodNamespace,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  constants.EnvClusterName,
									Value: cluster.Name,
								},
								{
									Name:  constants.EnvSentinelDebounceWindowSeconds,
									Value: fmt.Sprintf("%d", debounceWindowSeconds),
								},
							},
							Resources: *resources,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(8082),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/healthz",
										Port: intstr.FromInt32(8082),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
						},
					},
				},
			},
		},
	}

	if err := m.applyResource(ctx, deployment, cluster); err != nil {
		return fmt.Errorf("failed to ensure Sentinel Deployment %s/%s: %w", cluster.Namespace, deploymentName, err)
	}

	logger.Info("Ensured Sentinel Deployment", "deployment", deploymentName, "image", sentinelImage)
	return nil
}

// ensureSentinelCleanup deletes Sentinel resources when Sentinel is disabled.
func (m *Manager) ensureSentinelCleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	deploymentName := cluster.Name + constants.SentinelDeploymentNameSuffix

	// Delete Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: cluster.Namespace,
		},
	}
	if err := m.client.Delete(ctx, deployment); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Sentinel Deployment: %w", err)
	}

	// Delete ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelServiceAccountName,
			Namespace: cluster.Namespace,
		},
	}
	if err := m.client.Delete(ctx, sa); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Sentinel ServiceAccount: %w", err)
	}

	// Delete Role
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelRoleName,
			Namespace: cluster.Namespace,
		},
	}
	if err := m.client.Delete(ctx, role); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Sentinel Role: %w", err)
	}

	// Delete RoleBinding
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.SentinelRoleBindingName,
			Namespace: cluster.Namespace,
		},
	}
	if err := m.client.Delete(ctx, roleBinding); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Sentinel RoleBinding: %w", err)
	}

	logger.Info("Cleaned up Sentinel resources")
	return nil
}
