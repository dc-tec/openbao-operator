package provisioner

import (
	"context"
	"slices"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	serviceprovisioner "github.com/dc-tec/openbao-operator/internal/service/provisioner"
)

type secretRBACSnapshot struct {
	writerRoleExists        bool
	writerRoleBindingExists bool
	writerSecrets           []string
	readerRoleExists        bool
	readerRoleBindingExists bool
	readerSecrets           []string
}

func emitClusterNormalEvent(recorder events.EventRecorder, cluster *openbaov1alpha1.OpenBaoCluster, reason, note string) {
	if recorder == nil || cluster == nil {
		return
	}
	recorder.Eventf(cluster, nil, corev1.EventTypeNormal, reason, reason, "%s", note)
}

func loadSecretRBACSnapshot(ctx context.Context, c client.Reader, namespace string) (secretRBACSnapshot, error) {
	if c == nil || namespace == "" {
		return secretRBACSnapshot{}, nil
	}

	writerRole, writerRoleExists, err := loadRoleResourceNames(ctx, c, namespace, serviceprovisioner.TenantSecretsWriterRoleName)
	if err != nil {
		return secretRBACSnapshot{}, err
	}
	writerRoleBindingExists, err := roleBindingExists(ctx, c, namespace, serviceprovisioner.TenantSecretsWriterRoleBindingName)
	if err != nil {
		return secretRBACSnapshot{}, err
	}
	readerRole, readerRoleExists, err := loadRoleResourceNames(ctx, c, namespace, serviceprovisioner.TenantSecretsReaderRoleName)
	if err != nil {
		return secretRBACSnapshot{}, err
	}
	readerRoleBindingExists, err := roleBindingExists(ctx, c, namespace, serviceprovisioner.TenantSecretsReaderRoleBindingName)
	if err != nil {
		return secretRBACSnapshot{}, err
	}

	return secretRBACSnapshot{
		writerRoleExists:        writerRoleExists,
		writerRoleBindingExists: writerRoleBindingExists,
		writerSecrets:           writerRole,
		readerRoleExists:        readerRoleExists,
		readerRoleBindingExists: readerRoleBindingExists,
		readerSecrets:           readerRole,
	}, nil
}

func loadRoleResourceNames(ctx context.Context, c client.Reader, namespace, name string) ([]string, bool, error) {
	role := &rbacv1.Role{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	names := make([]string, 0)
	for i := range role.Rules {
		names = append(names, role.Rules[i].ResourceNames...)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	return names, true, nil
}

func roleBindingExists(ctx context.Context, c client.Reader, namespace, name string) (bool, error) {
	roleBinding := &rbacv1.RoleBinding{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, roleBinding); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s secretRBACSnapshot) equal(other secretRBACSnapshot) bool {
	return s.writerRoleExists == other.writerRoleExists &&
		s.writerRoleBindingExists == other.writerRoleBindingExists &&
		s.readerRoleExists == other.readerRoleExists &&
		s.readerRoleBindingExists == other.readerRoleBindingExists &&
		slices.Equal(s.writerSecrets, other.writerSecrets) &&
		slices.Equal(s.readerSecrets, other.readerSecrets)
}
