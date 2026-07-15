package identity

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/revision"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceapply"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portworkload "github.com/dc-tec/openbao-operator/internal/port/workload"
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
	return resourceapply.ApplyOwned(ctx, m.client, m.scheme, cluster, obj)
}

func openBaoPodResourceNames(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	voterReplicas := cluster.Spec.Replicas
	if voterReplicas < 3 {
		voterReplicas = 3
	}

	prefixReplicaCounts := map[string]int32{
		portworkload.StableVoterStatefulSetName(cluster): voterReplicas,
	}

	if portworkload.EffectiveStrategy(cluster) == openbaov1alpha1.UpdateStrategyBlueGreen {
		// Keep the original name authorized for initial BlueGreen bootstrap and
		// RollingUpdate-to-BlueGreen transitions with an unrevisioned Blue.
		prefixReplicaCounts[cluster.Name] = voterReplicas
		resolvedImage := cluster.Spec.Image
		if resolvedImage == "" {
			resolvedImage = constants.GetOpenBaoImage(cluster.Spec.Version)
		}

		blueRevision := ""
		if cluster.Status.BlueGreen == nil {
			blueRevision = revision.OpenBaoClusterRevision(cluster.Spec.Version, resolvedImage, cluster.Spec.Replicas)
		} else {
			blueRevision = cluster.Status.BlueGreen.BlueRevision
		}
		if blueRevision != "" {
			prefixReplicaCounts[fmt.Sprintf("%s-%s", cluster.Name, blueRevision)] = voterReplicas
		}

		if cluster.Status.BlueGreen != nil && cluster.Status.BlueGreen.GreenRevision != "" {
			prefixReplicaCounts[fmt.Sprintf("%s-%s", cluster.Name, cluster.Status.BlueGreen.GreenRevision)] = voterReplicas
		}
	}

	if readReplicas := readReplicaPodResourceCount(cluster); readReplicas > 0 {
		prefixReplicaCounts[resourceidentity.ReadReplicaStatefulSetName(cluster)] = readReplicas
	}

	total := 0
	for _, replicas := range prefixReplicaCounts {
		total += int(replicas)
	}
	names := make([]string, 0, total)
	for prefix, replicas := range prefixReplicaCounts {
		for i := int32(0); i < replicas; i++ {
			names = append(names, prefix+"-"+strconv.FormatInt(int64(i), 10))
		}
	}
	return names
}

func readReplicaPodResourceCount(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	var replicas int32
	if cluster.Spec.ReadReplicas != nil {
		replicas = cluster.Spec.ReadReplicas.Replicas
	}
	if cluster.Status.ReadReplicas != nil {
		if cluster.Status.ReadReplicas.DesiredReplicas > replicas {
			replicas = cluster.Status.ReadReplicas.DesiredReplicas
		}
		if cluster.Status.ReadReplicas.ReadyReplicas > replicas {
			replicas = cluster.Status.ReadReplicas.ReadyReplicas
		}
		if cluster.Status.ReadReplicas.RegisteredReplicas > replicas {
			replicas = cluster.Status.ReadReplicas.RegisteredReplicas
		}
	}
	return replicas
}
