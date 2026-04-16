package openbaocluster

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
)

const (
	workloadFieldOwner = "openbao-workload-controller"
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

	applyConfig, err := statusapply.ToApplyConfiguration(applyCluster, c)
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
	return PatchAdminOpsOwnedFieldsWithReader(ctx, c, c, logger, original, cluster, reason)
}

// PatchAdminOpsOwnedFieldsWithReader patches only admin-ops controller owned
// status fields, using reader for live read-before-write freshness when
// available.
func PatchAdminOpsOwnedFieldsWithReader(
	ctx context.Context,
	reader client.Reader,
	c client.Client,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	reason string,
) error {
	if original == nil || cluster == nil {
		return nil
	}

	// Backup-only diffs are persisted by the backup manager. When we do patch
	// adminops status for other reasons, we still apply the full current adminops
	// plane so shared SSA ownership does not clear backup or peer fields by omission.
	if reflect.DeepEqual(original.Status.BlueGreen, cluster.Status.BlueGreen) &&
		reflect.DeepEqual(original.Status.UpgradeRequests, cluster.Status.UpgradeRequests) &&
		reflect.DeepEqual(original.Status.BreakGlass, cluster.Status.BreakGlass) &&
		reflect.DeepEqual(original.Status.AdminOps, cluster.Status.AdminOps) {
		return nil
	}

	adminOps := cluster.Status.AdminOps
	if adminOps == nil {
		adminOps = &openbaov1alpha1.AdminOpsControllerStatus{}
	}

	cluster.Status.AdminOps = adminOps
	key := types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}
	updated, err := statusapply.MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(ctx, reader, c, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.BlueGreen = cluster.Status.BlueGreen
		obj.Status.UpgradeRequests = cluster.Status.UpgradeRequests
		obj.Status.BreakGlass = cluster.Status.BreakGlass
		obj.Status.AdminOps = adminOps
		return nil
	}, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{})
	if err != nil && apierrors.IsConflict(err) {
		// Ownership-conflict path: retry with force only on SSA conflict.
		updated, err = statusapply.MutateAndApplyOpenBaoClusterAdminOpsStatusWithReader(ctx, reader, c, key, func(obj *openbaov1alpha1.OpenBaoCluster) error {
			obj.Status.BlueGreen = cluster.Status.BlueGreen
			obj.Status.UpgradeRequests = cluster.Status.UpgradeRequests
			obj.Status.BreakGlass = cluster.Status.BreakGlass
			obj.Status.AdminOps = adminOps
			return nil
		}, statusapply.OpenBaoClusterAdminOpsStatusApplyOptions{
			ForceOwnership: true,
		})
	}
	if err != nil {
		return fmt.Errorf("failed to patch adminops status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	cluster.Status.BlueGreen = updated.Status.BlueGreen
	cluster.Status.UpgradeRequests = updated.Status.UpgradeRequests
	cluster.Status.Backup = updated.Status.Backup
	cluster.ResourceVersion = updated.ResourceVersion
	cluster.Status.BreakGlass = updated.Status.BreakGlass
	cluster.Status.AdminOps = updated.Status.AdminOps

	logger.V(1).Info("Patched OpenBaoCluster adminops status (SSA)", "reason", reason, "fieldOwner", constants.FieldOwnerAdminOpsStatus)
	return nil
}
