package backup

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
)

// UpgradeStrategyRuntime implements pre-upgrade backup operations for upgrade strategies.
type UpgradeStrategyRuntime struct {
	client client.Client
	scheme *runtime.Scheme
}

// NewUpgradeStrategyRuntime creates a runtime adapter for strategy packages.
func NewUpgradeStrategyRuntime(c client.Client, scheme *runtime.Scheme) *UpgradeStrategyRuntime {
	return &UpgradeStrategyRuntime{
		client: c,
		scheme: scheme,
	}
}

// EnsureServiceAccount creates or updates the ServiceAccount used by backup jobs.
func (r *UpgradeStrategyRuntime) EnsureServiceAccount(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupServiceAccount(ctx, r.client, r.scheme, cluster)
}

// EnsureRBAC creates or updates Role/RoleBinding required by backup jobs.
func (r *UpgradeStrategyRuntime) EnsureRBAC(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	return EnsureBackupRBAC(ctx, r.client, r.scheme, cluster)
}

// BuildPreUpgradeJob constructs a pre-upgrade snapshot backup job.
func (r *UpgradeStrategyRuntime) BuildPreUpgradeJob(cluster *openbaov1alpha1.OpenBaoCluster, opts portbackup.JobBuildOptions) (*batchv1.Job, error) {
	return BuildJob(cluster, JobOptions{
		JobName:                opts.JobName,
		JobType:                JobTypePreUpgrade,
		FilenamePrefix:         opts.FilenamePrefix,
		VerifiedExecutorDigest: opts.VerifiedExecutorDigest,
		ClientConfig:           opts.ClientConfig,
		Platform:               opts.Platform,
		TargetStatefulSetName:  opts.TargetStatefulSetName,
	})
}
