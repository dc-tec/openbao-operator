package openbaocluster

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminopsstatus"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/statusapply"
)

// PatchStatusOwnedFields patches only status-controller owned status fields.
func PatchStatusOwnedFields(ctx context.Context, c client.Client, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
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
			ObservedGeneration: cluster.Status.ObservedGeneration,
			Phase:              cluster.Status.Phase,
			ActiveLeader:       cluster.Status.ActiveLeader,
			ReadyReplicas:      cluster.Status.ReadyReplicas,
			ReadReplicas:       cluster.Status.ReadReplicas,
			CurrentVersion:     cluster.Status.CurrentVersion,
			LastBackupTime:     cluster.Status.LastBackupTime,
			Conditions:         cluster.Status.Conditions,
		},
	}

	explicitNullPaths := make([]string, 0, 2)
	if cluster.Status.ReadReplicas == nil {
		explicitNullPaths = append(explicitNullPaths, "status.readReplicas")
	}
	if cluster.Status.LastBackupTime == nil {
		explicitNullPaths = append(explicitNullPaths, "status.lastBackupTime")
	}

	applyConfig, err := statusapply.ToApplyConfigurationWithExplicitNulls(applyCluster, c, explicitNullPaths...)
	if err != nil {
		return fmt.Errorf("failed to convert cluster to ApplyConfiguration: %w", err)
	}

	return c.Status().Apply(ctx, applyConfig, client.FieldOwner(constants.FieldOwnerStatus))
}

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

	if err := c.Status().Apply(ctx, applyConfig, client.FieldOwner(constants.FieldOwnerWorkloadStatus)); err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Skipping workload status patch because OpenBaoCluster no longer exists", "reason", reason)
			return nil
		}
		return fmt.Errorf("failed to patch workload status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}
	logger.V(1).Info("Patched OpenBaoCluster workload status (SSA)", "reason", reason, "fieldOwner", constants.FieldOwnerWorkloadStatus)
	return nil
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
	if original.Status.AcceptedUpgradeStrategy == cluster.Status.AcceptedUpgradeStrategy &&
		reflect.DeepEqual(original.Status.BlueGreen, cluster.Status.BlueGreen) &&
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
	err := adminopsstatus.MutateWithReader(ctx, reader, c, cluster, func(obj *openbaov1alpha1.OpenBaoCluster) error {
		obj.Status.AcceptedUpgradeStrategy = cluster.Status.AcceptedUpgradeStrategy
		obj.Status.BlueGreen = cluster.Status.BlueGreen
		obj.Status.UpgradeRequests = cluster.Status.UpgradeRequests
		obj.Status.BreakGlass = cluster.Status.BreakGlass
		obj.Status.AdminOps = adminOps
		return nil
	}, adminopsstatus.MutateOptions{RetryOnConflict: true})
	if err != nil {
		return fmt.Errorf("failed to patch adminops status (%s) for OpenBaoCluster %s/%s: %w", reason, cluster.Namespace, cluster.Name, err)
	}

	logger.V(1).Info("Patched OpenBaoCluster adminops status (SSA)", "reason", reason, "fieldOwner", constants.FieldOwnerAdminOpsStatus)
	return nil
}
