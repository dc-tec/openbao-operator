package infra

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// ensureServiceAccount manages the ServiceAccount for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        saName,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: nil,
		},
	}

	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Annotations != nil {
		sa.Annotations = cluster.Spec.ServiceAccount.Annotations
	}

	if err := m.applyResource(ctx, sa, cluster); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount %s/%s: %w", cluster.Namespace, saName, err)
	}

	return nil
}

// ensureRBAC creates a Role and RoleBinding for the OpenBaoCluster service account.
//
// This is required for:
// - OpenBao's Kubernetes auto-join discovery (pod list/watch by label selector)
// - OpenBao's Kubernetes service registration (pod label updates; some clients use PATCH)
//
// Security hardening:
// - list/watch is scoped to the namespace (required for label-selector discovery)
// - mutation (patch/update) is scoped to the OpenBao StatefulSet Pod resourceNames only
//
// Uses Server-Side Apply.
func (m *Manager) ensureRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := serviceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"

	podResourceNames := openBaoPodResourceNames(cluster)

	// Ensure Role exists using SSA
	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"pods"},
				ResourceNames: podResourceNames,
				Verbs:         []string{"patch", "update"},
			},
		},
	}

	if err := m.applyResource(ctx, role, cluster); err != nil {
		return fmt.Errorf("failed to ensure Role %s/%s: %w", cluster.Namespace, roleName, err)
	}

	// Ensure RoleBinding exists using SSA
	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: cluster.Namespace,
			},
		},
	}

	if err := m.applyResource(ctx, roleBinding, cluster); err != nil {
		return fmt.Errorf("failed to ensure RoleBinding %s/%s: %w", cluster.Namespace, roleBindingName, err)
	}

	return nil
}

// serviceAccountName returns the name for the ServiceAccount resource.
func serviceAccountName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	if cluster.Spec.ServiceAccount != nil && cluster.Spec.ServiceAccount.Name != "" {
		return cluster.Spec.ServiceAccount.Name
	}
	return cluster.Name + constants.SuffixServiceAccount
}

func openBaoPodResourceNames(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	// Pod names are predictable (<statefulSetName>-<ordinal>), and RBAC resourceNames
	// cannot express wildcards. Include at least the default replica count (3) to:
	// - avoid any "replicas=0" defaulting edge cases
	// - reduce transient failures during scale-down before pods are deleted
	replicas := cluster.Spec.Replicas
	if replicas < 3 {
		replicas = 3
	}

	// Default/rolling deployments use the base StatefulSet name (= cluster.Name).
	// Blue/green deployments use revisioned StatefulSets named "<cluster.Name>-<revision>".
	//
	// The OpenBao pod ServiceAccount must be able to patch its own Pod (service registration labels),
	// regardless of whether the pod belongs to Blue or Green.
	prefixes := map[string]struct{}{
		cluster.Name: {},
	}

	if cluster.Spec.Upgrade != nil && cluster.Spec.Upgrade.Strategy == openbaov1alpha1.UpdateStrategyBlueGreen {
		resolvedImage := cluster.Spec.Image
		if resolvedImage == "" {
			resolvedImage = constants.GetOpenBaoImage(cluster.Spec.Version)
		}

		blueRevision := revision.OpenBaoClusterRevision(cluster.Spec.Version, resolvedImage, cluster.Spec.Replicas)
		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.BlueRevision != "" {
			blueRevision = cluster.Status.BlueGreen.BlueRevision
		}
		prefixes[fmt.Sprintf("%s-%s", cluster.Name, blueRevision)] = struct{}{}

		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.GreenRevision != "" {
			prefixes[fmt.Sprintf("%s-%s", cluster.Name, cluster.Status.BlueGreen.GreenRevision)] = struct{}{}
		}
	}

	names := make([]string, 0, len(prefixes)*int(replicas))
	for prefix := range prefixes {
		for i := int32(0); i < replicas; i++ {
			names = append(names, prefix+"-"+strconv.FormatInt(int64(i), 10))
		}
	}
	return names
}
