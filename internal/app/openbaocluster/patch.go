package openbaocluster

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	workloadFieldOwner        = "openbao-workload-controller"
	adminOpsSupportFieldOwner = "openbao-adminops-support-controller"
)

// PatchWorkloadOwnedFields patches only workload-controller owned status fields.
func PatchWorkloadOwnedFields(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	reason string,
) error {
	if original == nil || cluster == nil {
		return nil
	}

	if original.Status.Initialized == cluster.Status.Initialized &&
		original.Status.SelfInitialized == cluster.Status.SelfInitialized &&
		reflect.DeepEqual(original.Status.Workload, cluster.Status.Workload) {
		return nil
	}

	workload := cluster.Status.Workload
	if workload == nil {
		workload = &openbaov1alpha1.WorkloadControllerStatus{}
	}

	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Initialized:     cluster.Status.Initialized,
			SelfInitialized: cluster.Status.SelfInitialized,
			Workload:        workload,
		},
	}

	applyConfig, err := toApplyConfiguration(applyCluster, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	if err := c.Status().Apply(ctx, applyConfig, client.FieldOwner(workloadFieldOwner)); err != nil {
		return fmt.Errorf("failed to patch workload status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster workload status (SSA)", "reason", reason, "fieldOwner", workloadFieldOwner)
	return nil
}

// PatchAdminOpsOwnedFields patches only admin-ops controller owned status fields.
func PatchAdminOpsOwnedFields(
	ctx context.Context,
	c client.Client,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	reason string,
) error {
	if original == nil || cluster == nil {
		return nil
	}

	if reflect.DeepEqual(original.Status.BlueGreen, cluster.Status.BlueGreen) &&
		reflect.DeepEqual(original.Status.Backup, cluster.Status.Backup) &&
		reflect.DeepEqual(original.Status.BreakGlass, cluster.Status.BreakGlass) &&
		reflect.DeepEqual(original.Status.AdminOps, cluster.Status.AdminOps) {
		return nil
	}

	adminOps := cluster.Status.AdminOps
	if adminOps == nil {
		adminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	applyCluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name,
			Namespace: cluster.Namespace,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			BlueGreen:  cluster.Status.BlueGreen,
			Backup:     cluster.Status.Backup,
			BreakGlass: cluster.Status.BreakGlass,
			AdminOps:   adminOps,
		},
	}

	applyConfig, err := toApplyConfiguration(applyCluster, c)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	if err := c.Status().Apply(ctx, applyConfig, client.FieldOwner(adminOpsSupportFieldOwner), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to patch adminops status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster adminops status (SSA)", "reason", reason, "fieldOwner", adminOpsSupportFieldOwner)
	return nil
}
