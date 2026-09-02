package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/app/openbaocluster/statusops"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// StatusPodObserver exposes pod-local health observation used by status
// reconciliation for read-replica serving checks.
type StatusPodObserver interface {
	Health(ctx context.Context) (*portopenbao.HealthStatus, error)
}

// StatusPodObserverFactory constructs pod-local observers for OpenBao pods.
type StatusPodObserverFactory func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (StatusPodObserver, error)

// StatusMembershipRuntime exposes authenticated raft membership observation.
type StatusMembershipRuntime interface {
	ReadRaftConfiguration(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftConfigurationResponse, error)
	ReadRaftAutopilotState(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (*portopenbao.RaftAutopilotStateResponse, error)
}

// StatusDependencies provides dependencies needed for status state observation.
type StatusDependencies struct {
	Reader             client.Reader
	PodObserverFactory StatusPodObserverFactory
	MembershipRuntime  StatusMembershipRuntime
}

// StatusState is the app-layer observation model used by status reconciliation.
type StatusState = statusops.StatusState

// StatusPolicyInput contains the inputs for normal status reconciliation policy.
type StatusPolicyInput = statusops.PolicyInput

// ApplyStatusPolicy computes status-owned fields and a requeue decision from observed cluster state.
func ApplyStatusPolicy(logger logr.Logger, input StatusPolicyInput) recon.Result {
	return statusops.ApplyPolicy(logger, input)
}

// ApplyUserAccessBootstrapCondition updates the user-access condition for the
// current cluster generation.
func ApplyUserAccessBootstrapCondition(cluster *openbaov1alpha1.OpenBaoCluster, now metav1.Time) {
	statusops.ApplyUserAccessBootstrapCondition(cluster, now)
}

// GatherStatusState reads current cluster state needed for status reconciliation.
func GatherStatusState(
	ctx context.Context,
	logger logr.Logger,
	deps StatusDependencies,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*StatusState, error) {
	if deps.Reader == nil {
		return nil, fmt.Errorf("status reader dependency is required")
	}

	var podObserverFactory statusops.PodObserverFactory
	if deps.PodObserverFactory != nil {
		podObserverFactory = func(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (statusops.PodObserver, error) {
			return deps.PodObserverFactory(ctx, cluster, podName)
		}
	}

	return statusops.GatherState(
		ctx,
		logger,
		deps.Reader,
		podObserverFactory,
		deps.MembershipRuntime,
		cluster,
		statusops.LabelConfig{
			AppInstanceKey:       labelAppInstance,
			AppManagedByKey:      labelAppManagedBy,
			AppManagedByValue:    labelValueAppManagedByOpenBaoOperator,
			OpenBaoClusterKey:    labelOpenBaoCluster,
			OpenBaoComponentKey:  labelOpenBaoComponent,
			BackupComponentValue: componentBackup,
			AppNameKey:           labelAppName,
			AppNameValue:         labelValueAppNameOpenBao,
			OpenBaoRevisionKey:   labelOpenBaoRevision,
		},
	)
}

// ShouldWarnSelfInitDisabled returns whether reconciliation should emit the root-token warning.
func ShouldWarnSelfInitDisabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return statusops.ShouldWarnSelfInitDisabled(cluster)
}

// IsStaticUnseal reports whether the cluster uses static unseal configuration.
func IsStaticUnseal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return statusops.IsStaticUnseal(cluster)
}
