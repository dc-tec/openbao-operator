package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	adminopsapp "github.com/dc-tec/openbao-operator/internal/app/openbaocluster/adminops"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// ApplicationsConfig contains the already-wired collaborators used by the
// three OpenBaoCluster controller-runtime controllers.
type ApplicationsConfig struct {
	Client                   client.Client
	WorkloadReconcilers      []SubReconciler
	WorkloadPolicy           WorkloadResultPolicy
	AdminOpsApplication      *AdminOpsApplication
	StatusDependencies       StatusDependencies
	DeletionDependencies     DeletionDependencies
	StatusIntegration        StatusIntegrationDependencies
	InitializationConfigured bool
}

// Applications is the prebuilt OpenBaoCluster application boundary consumed
// by the controller package.
type Applications struct {
	config ApplicationsConfig
}

// NewApplications constructs the prebuilt OpenBaoCluster application boundary.
func NewApplications(config ApplicationsConfig) *Applications {
	return &Applications{config: config}
}

// ReconcileWorkload executes workload orchestration with its configured steps.
func (a *Applications) ReconcileWorkload(
	ctx context.Context,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError ErrorRecorder,
) (recon.Result, error) {
	if a == nil || a.config.Client == nil {
		return recon.Result{}, fmt.Errorf("workload application client is required")
	}

	return RunWorkloadReconcilers(
		ctx,
		a.config.Client,
		logger,
		original,
		cluster,
		a.config.WorkloadReconcilers,
		recordError,
		a.config.WorkloadPolicy,
	)
}

// ReconcileAdminOps executes the administrative operations application.
func (a *Applications) ReconcileAdminOps(
	ctx context.Context,
	logger logr.Logger,
	original *openbaov1alpha1.OpenBaoCluster,
	cluster *openbaov1alpha1.OpenBaoCluster,
	recordError ErrorRecorder,
) (recon.Result, error) {
	if a == nil || a.config.AdminOpsApplication == nil {
		return recon.Result{}, fmt.Errorf("admin operations application is required")
	}

	return a.config.AdminOpsApplication.Reconcile(
		ctx,
		logger,
		original,
		cluster,
		adminopsapp.ErrorRecorder(recordError),
	)
}

// GatherStatusState observes the runtime state used by status evaluation.
func (a *Applications) GatherStatusState(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*StatusState, error) {
	if a == nil {
		return nil, fmt.Errorf("status application is required")
	}
	return GatherStatusState(ctx, logger, a.config.StatusDependencies, cluster)
}

// HandleDeletion executes the application-owned deletion workflow.
func (a *Applications) HandleDeletion(
	ctx context.Context,
	logger logr.Logger,
	cluster *openbaov1alpha1.OpenBaoCluster,
) error {
	if a == nil {
		return fmt.Errorf("deletion application is required")
	}
	return HandleDeletion(ctx, logger, a.config.DeletionDependencies, cluster)
}

// InitializationConfigured reports whether initialization orchestration is available.
func (a *Applications) InitializationConfigured() bool {
	return a != nil && a.config.InitializationConfigured
}

// EvaluateACMEIntegration evaluates the configured ACME integration contract.
func (a *Applications) EvaluateACMEIntegration(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) ACMEIntegrationResult {
	return EvaluateACMEIntegration(ctx, a.config.StatusIntegration, cluster)
}

// EvaluateGatewayIntegration evaluates the configured Gateway integration contract.
func (a *Applications) EvaluateGatewayIntegration(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) GatewayIntegrationResult {
	return EvaluateGatewayIntegration(ctx, a.config.StatusIntegration, cluster)
}

// EvaluateIngressIntegration evaluates the configured Ingress integration contract.
func (a *Applications) EvaluateIngressIntegration(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) IngressIntegrationResult {
	return EvaluateIngressIntegration(ctx, a.config.StatusIntegration, cluster)
}

// EvaluateAPIServerNetwork evaluates the configured API-server network contract.
func (a *Applications) EvaluateAPIServerNetwork(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) APIServerNetworkResult {
	return EvaluateAPIServerNetwork(ctx, a.config.StatusIntegration, cluster)
}
