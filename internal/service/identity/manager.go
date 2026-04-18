package identity

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

// Manager reconciles ServiceAccount and RBAC resources for an OpenBaoCluster.
type Manager struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewManager(c client.Client, scheme *runtime.Scheme) *Manager {
	return &Manager{
		client: c,
		scheme: scheme,
	}
}

// Reconcile ensures the cluster ServiceAccount and RBAC resources exist.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.ensureServiceAccount(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureRBAC(ctx, logger, cluster); err != nil {
		return err
	}
	return nil
}

func (m *Manager) ensureServiceAccount(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := resourceidentity.ServiceAccountName(cluster)

	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        saName,
			Namespace:   cluster.Namespace,
			Labels:      serviceAccountLabels(cluster),
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

func serviceAccountLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	labels := resourceidentity.Labels(cluster)
	labels[constants.LabelOpenBaoServiceAccountRole] = constants.ServiceAccountRoleMain
	return labels
}

func (m *Manager) ensureRBAC(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	saName := resourceidentity.ServiceAccountName(cluster)
	roleName := saName + "-role"
	roleBindingName := saName + "-rolebinding"

	podResourceNames := openBaoPodResourceNames(cluster)

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: cluster.Namespace,
			Labels:    resourceidentity.Labels(cluster),
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

	roleBinding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: cluster.Namespace,
			Labels:    resourceidentity.Labels(cluster),
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

func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-operator"),
	}

	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		if apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

func openBaoPodResourceNames(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	replicas := cluster.Spec.Replicas
	if replicas < 3 {
		replicas = 3
	}

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
