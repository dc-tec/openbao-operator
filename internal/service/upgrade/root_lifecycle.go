package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/logging"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/core"
)

// RootUpgradeSessionStart captures the root status-plane facts needed to start
// an upgrade attempt.
type RootUpgradeSessionStart struct {
	FromVersion string
	ToVersion   string
	Replicas    int32
}

// RootUpgradeSessionCompletion captures the root status-plane facts observed
// when a root upgrade attempt completes successfully.
type RootUpgradeSessionCompletion struct {
	FromVersion string
	ToVersion   string
	Duration    time.Duration
}

// NewRootUpgradeSessionStart reads the cluster state required to start a root
// upgrade attempt.
func NewRootUpgradeSessionStart(cluster *openbaov1alpha1.OpenBaoCluster) RootUpgradeSessionStart {
	if cluster == nil {
		return RootUpgradeSessionStart{}
	}

	return RootUpgradeSessionStart{
		FromVersion: cluster.Status.CurrentVersion,
		ToVersion:   cluster.Spec.Version,
		Replicas:    cluster.Spec.Replicas,
	}
}

// Apply mutates the cluster status to reflect a started root upgrade attempt.
func (start RootUpgradeSessionStart) Apply(status *openbaov1alpha1.OpenBaoClusterStatus) {
	if status == nil {
		return
	}
	core.SetUpgradeStarted(status, start.FromVersion, start.ToVersion, start.Replicas)
}

// CompleteRootUpgradeSession captures completion details and mutates the status
// to the completed root-upgrade state.
func CompleteRootUpgradeSession(
	status *openbaov1alpha1.OpenBaoClusterStatus,
	targetVersion string,
	now time.Time,
) RootUpgradeSessionCompletion {
	completion := RootUpgradeSessionCompletion{ToVersion: targetVersion}
	if status == nil {
		return completion
	}

	if status.Upgrade != nil {
		completion.FromVersion = status.Upgrade.FromVersion
		if status.Upgrade.StartedAt != nil {
			completion.Duration = now.Sub(status.Upgrade.StartedAt.Time)
		}
	}

	core.SetUpgradeComplete(status, targetVersion)
	return completion
}

// RecordRootUpgradeSessionStart marks the metrics session as a newly started
// root upgrade attempt.
func RecordRootUpgradeSessionStart(metrics *Metrics, strategy string) {
	if metrics == nil {
		return
	}
	metrics.IncrementTotal(strategy)
}

// RecordRootUpgradeSessionSuccess records successful completion metrics for a
// root upgrade attempt.
func RecordRootUpgradeSessionSuccess(metrics *Metrics, strategy string, completion RootUpgradeSessionCompletion) {
	if metrics == nil {
		return
	}
	if completion.Duration > 0 {
		metrics.RecordDuration(completion.Duration.Seconds(), completion.FromVersion, completion.ToVersion)
	}
	SetTerminalProgressMetrics(metrics, UpgradeStatusSuccess)
	metrics.IncrementSuccess(strategy)
}

// UpgradeStartedAuditFields builds the common audit-event fields for the start
// of an upgrade attempt.
func UpgradeStartedAuditFields(
	cluster *openbaov1alpha1.OpenBaoCluster,
	strategy string,
	fromVersion string,
	toVersion string,
) map[string]string {
	fields := upgradeAuditFields(cluster, strategy)
	fields["from_version"] = fromVersion
	fields["to_version"] = toVersion
	return fields
}

// UpgradeCompletedAuditFields builds the common audit-event fields for the
// successful completion of an upgrade attempt.
func UpgradeCompletedAuditFields(cluster *openbaov1alpha1.OpenBaoCluster, strategy string, version string) map[string]string {
	fields := upgradeAuditFields(cluster, strategy)
	fields["version"] = version
	return fields
}

func upgradeAuditFields(cluster *openbaov1alpha1.OpenBaoCluster, strategy string) map[string]string {
	fields := map[string]string{
		"strategy": strategy,
	}
	if cluster == nil {
		return fields
	}

	fields["cluster_namespace"] = cluster.Namespace
	fields["cluster_name"] = cluster.Name
	return fields
}

// RootUpgradeStartOptions controls strategy-owned persistence and event
// behavior for starting a root upgrade lifecycle.
type RootUpgradeStartOptions struct {
	Persist   func(context.Context, *openbaov1alpha1.OpenBaoCluster, RootUpgradeSessionStart) error
	EmitEvent func(fromVersion, toVersion string)
}

// RootUpgradeCompletionOptions controls strategy-owned persistence and event
// behavior for completing a root upgrade lifecycle.
type RootUpgradeCompletionOptions struct {
	Persist   func(context.Context, *openbaov1alpha1.OpenBaoCluster, RootUpgradeSessionCompletion) error
	EmitEvent func(fromVersion, toVersion string)
}

// StartRootUpgradeLifecycle applies the common start-of-upgrade lifecycle for
// root status-based upgrades. Strategy code supplies the persistence and event
// hooks that remain strategy-specific.
func StartRootUpgradeLifecycle(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *Metrics,
	strategy string,
	opts RootUpgradeStartOptions,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if opts.Persist == nil {
		return fmt.Errorf("persist callback is required")
	}

	start := NewRootUpgradeSessionStart(cluster)

	logger.Info("Initializing upgrade",
		"from", start.FromVersion,
		"to", start.ToVersion,
		"replicas", start.Replicas)

	start.Apply(&cluster.Status)

	if err := opts.Persist(ctx, cluster, start); err != nil {
		return err
	}

	RecordRootUpgradeSessionStart(metrics, strategy)
	logging.LogAuditEvent(logger, logging.EventUpgradeStarted, UpgradeStartedAuditFields(cluster, strategy, start.FromVersion, start.ToVersion))
	if opts.EmitEvent != nil {
		opts.EmitEvent(start.FromVersion, start.ToVersion)
	}

	logger.Info("Upgrade initialized",
		"partition", start.Replicas)

	return nil
}

// CompleteRootUpgradeLifecycle applies the common successful completion path
// for root status-based upgrades. Strategy code supplies the persistence and
// event hooks that remain strategy-specific.
func CompleteRootUpgradeLifecycle(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
	metrics *Metrics,
	strategy string,
	opts RootUpgradeCompletionOptions,
) error {
	if cluster == nil {
		return fmt.Errorf("cluster is required")
	}
	if opts.Persist == nil {
		return fmt.Errorf("persist callback is required")
	}

	completion := CompleteRootUpgradeSession(&cluster.Status, cluster.Spec.Version, time.Now())

	if err := opts.Persist(ctx, cluster, completion); err != nil {
		return err
	}

	RecordRootUpgradeSessionSuccess(metrics, strategy, completion)
	logging.LogAuditEvent(logger, logging.EventUpgradeCompleted, UpgradeCompletedAuditFields(cluster, strategy, completion.ToVersion))
	if opts.EmitEvent != nil {
		opts.EmitEvent(completion.FromVersion, completion.ToVersion)
	}

	logger.Info("Upgrade completed successfully",
		"version", completion.ToVersion,
		"duration", completion.Duration.Seconds())

	return nil
}
