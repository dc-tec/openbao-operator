package openbaoclusterclaim

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

const (
	conditionTypeControllerActive         = "ControllerActive"
	conditionTypeAccepted                 = "Accepted"
	conditionTypeServiceContract          = "ServiceContractReady"
	conditionTypeMaterialization          = "MaterializationResolved"
	conditionTypeOwnershipReady           = "OwnershipReady"
	conditionTypeConnectionPublished      = "ConnectionPublished"
	conditionTypeServiceAvailable         = "ServiceAvailable"
	conditionTypeMaintenanceActive        = "MaintenanceActive"
	reasonNotImplemented                  = "NotImplemented"
	reasonIdle                            = "Idle"
	reasonDeleting                        = "Deleting"
	kindConfigMap                         = "ConfigMap"
	kindSecret                            = "Secret"
	kindOpenBaoCluster                    = "OpenBaoCluster"
	kindOpenBaoClusterClaim               = "OpenBaoClusterClaim"
	kindOpenBaoClusterClaimBackupRequest  = "OpenBaoClusterClaimBackupRequest"
	kindOpenBaoClusterClaimRestoreRequest = "OpenBaoClusterClaimRestoreRequest"
	kindOpenBaoClusterClaimUpgradeRequest = "OpenBaoClusterClaimUpgradeRequest"
	kindOpenBaoRestore                    = "OpenBaoRestore"
	kindOpenBaoServiceOffering            = "OpenBaoServiceOffering"
	kindOpenBaoServiceProfile             = "OpenBaoServiceProfile"
	kindOpenBaoTenant                     = "OpenBaoTenant"
	connectionContractLabelKey            = "openbao.org/connection-contract"
	connectionContractLabelValue          = "primary"
)

type Reconciler interface {
	Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error)
}

type Runtime struct {
	Client                   client.Client
	Reader                   client.Reader
	Scheme                   *runtime.Scheme
	EnableServiceClaims      bool
	SameClusterNetwork       SameClusterNetworkConfig
	SameClusterTransitUnseal SameClusterTransitUnsealConfig
}

type runtimeReconciler struct {
	client                   client.Client
	reader                   client.Reader
	scheme                   *runtime.Scheme
	enableServiceClaims      bool
	sameClusterNetwork       SameClusterNetworkConfig
	sameClusterTransitUnseal SameClusterTransitUnsealConfig
}

type SameClusterNetworkConfig struct {
	APIServerCIDR        string
	APIServerEndpointIPs []string
	DNSEndpointIPs       []string
}

type SameClusterTransitUnsealConfig struct {
	Address               string
	KeyName               string
	MountPath             string
	Namespace             string
	TLSCACert             string
	TLSServerName         string
	CredentialsSecretName string
}

func NewReconciler(deps Runtime) Reconciler {
	reader := deps.Reader
	if reader == nil {
		reader = deps.Client
	}
	return runtimeReconciler{
		client:                   deps.Client,
		reader:                   reader,
		scheme:                   deps.Scheme,
		enableServiceClaims:      deps.EnableServiceClaims,
		sameClusterNetwork:       deps.SameClusterNetwork,
		sameClusterTransitUnseal: deps.SameClusterTransitUnseal,
	}
}

func (r runtimeReconciler) Reconcile(ctx context.Context, key types.NamespacedName, logger logr.Logger) (recon.Result, error) {
	if r.client == nil {
		return recon.Result{}, fmt.Errorf("client is required")
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := r.client.Get(ctx, key, claim); err != nil {
		if apierrors.IsNotFound(err) {
			return recon.Result{}, nil
		}
		return recon.Result{}, fmt.Errorf("failed to get OpenBaoClusterClaim: %w", err)
	}

	logger = logger.WithValues("openBaoClusterClaim", key.String())
	original := claim.DeepCopy()
	claimsEnabled := r.claimsEnabled()

	if !claim.DeletionTimestamp.IsZero() {
		claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting
		claim.Status.ObservedGeneration = claim.Generation
		meta.SetStatusCondition(&claim.Status.Conditions, controllerCondition(claimsEnabled, claim.Generation))
		if !reflect.DeepEqual(original.Status, claim.Status) {
			if err := r.client.Status().Patch(ctx, claim, client.MergeFrom(original)); err != nil {
				return recon.Result{}, fmt.Errorf("patch deleting OpenBaoClusterClaim status: %w", err)
			}
		}
		return r.reconcileDeletion(ctx, claim)
	}

	if claimsEnabled {
		beforeFinalizer := claim.DeepCopy()
		if ensureFinalizer(claim, openbaov1alpha1.OpenBaoClusterClaimFinalizer) {
			if err := r.client.Patch(ctx, claim, client.MergeFrom(beforeFinalizer)); err != nil {
				return recon.Result{}, fmt.Errorf("add OpenBaoClusterClaim finalizer: %w", err)
			}
			original = claim.DeepCopy()
		}
	}

	selectionPinned, err := r.reconcileServiceOfferingSelection(ctx, claim)
	if err != nil {
		return recon.Result{}, err
	}
	if selectionPinned {
		return recon.Result{}, nil
	}

	activeUpgradeRequest, err := r.resolveActiveUpgradeRequest(ctx, claim)
	if err != nil {
		return recon.Result{}, fmt.Errorf("resolve active OpenBaoClusterClaimUpgradeRequest: %w", err)
	}
	activeBackupRequest, err := r.resolveActiveBackupRequest(ctx, claim)
	if err != nil {
		return recon.Result{}, fmt.Errorf("resolve active OpenBaoClusterClaimBackupRequest: %w", err)
	}
	activeRestoreRequest, err := r.resolveActiveRestoreRequest(ctx, claim)
	if err != nil {
		return recon.Result{}, fmt.Errorf("resolve active OpenBaoClusterClaimRestoreRequest: %w", err)
	}

	tenant, acceptance := r.resolveTenant(ctx, claim)
	catalog, catalogResolution := r.resolveCatalogBundle(ctx, claim, acceptance)
	approvedContract, contractResolution := r.resolveApprovedServiceContract(claim, activeUpgradeRequest, catalog, acceptance, catalogResolution)
	localTarget, localResolved := r.resolveSameClusterMaterialization(claim, tenant, acceptance)
	activeRestoreExecution, err := r.resolveActiveRestoreExecution(ctx, localTarget, localResolved)
	if err != nil {
		return recon.Result{}, fmt.Errorf("resolve active OpenBaoRestore: %w", err)
	}
	bootstrapInputs, bootstrapResolution := r.resolveSameClusterBootstrapInputs(ctx, claim, localTarget, catalog)
	renderedContract, renderedResolution := r.resolveRenderedExecutionContract(
		claim,
		localTarget,
		localResolved,
		approvedContract,
		catalog,
		bootstrapInputs,
		bootstrapResolution,
		contractResolution,
	)
	desiredLocalCluster, localClusterResolution := r.resolveDesiredLocalCluster(
		claim,
		localResolved,
		renderedContract,
		renderedResolution,
	)
	materializationResult := resolvedMaterializationResult(localResolved, localClusterResolution)
	ownershipResult := r.resolveOwnership(ctx, claim, localTarget, localResolved, materializationResult)

	localCluster, localClusterEnsured, err := r.reconcileLocalClusterState(
		ctx,
		claim,
		localTarget,
		localResolved,
		ownershipResult,
		desiredLocalCluster,
		localClusterResolution,
		bootstrapInputs,
	)
	if err != nil {
		if classified, ok := classifyLocalClusterReconcileError(err); ok {
			localClusterResolution = classified
		} else {
			return recon.Result{}, err
		}
	}
	materializationResult = resolvedMaterializationResult(localResolved, localClusterResolution)
	connectionChanged, publication, err := r.reconcileConnectionContract(ctx, claim, localResolved, localCluster)
	if err != nil {
		return recon.Result{}, err
	}
	requeue := recon.Result{}
	if shouldRequeuePendingClaimState(
		claim,
		acceptance,
		catalogResolution,
		contractResolution,
		localResolved,
		bootstrapResolution,
		renderedResolution,
		localClusterResolution,
		publication,
		localCluster,
	) {
		requeue.RequeueAfter = constants.RequeueShort
	}

	claim.Status.ObservedGeneration = claim.Generation
	claim.Status.Materialization = desiredMaterializationStatus(
		localTarget,
		localResolved,
		renderedResolution,
		claim.Status.Materialization.LocalRef,
		original.Status.Applied,
	)
	claim.Status.Applied = desiredAppliedStatus(
		original.Status.Applied,
		claim,
		approvedContract,
		renderedContract,
		contractResolution,
		renderedResolution,
	)
	claim.Status.Rollout = desiredRolloutStatus(contractResolution, renderedResolution, localClusterResolution, localResolved, claim)
	claim.Status.Upgrade = desiredUpgradeStatus(activeUpgradeRequest)
	claim.Status.Restore = desiredRestoreStatus(activeRestoreRequest, activeRestoreExecution)
	claim.Status.Backup = desiredBackupStatusWithRequest(localCluster, activeBackupRequest)
	claim.Status.Phase = claimPhase(
		contractResolution,
		renderedResolution,
		localClusterResolution,
		localResolved,
		ownershipResult,
		localCluster,
		publication,
		activeUpgradeRequest,
		activeRestoreRequest,
		activeRestoreExecution,
	)
	meta.SetStatusCondition(&claim.Status.Conditions, controllerCondition(claimsEnabled, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, acceptanceCondition(acceptance, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, serviceContractCondition(contractResolution, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, materializationCondition(materializationResult, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, ownershipCondition(ownershipResult, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, connectionCondition(publication, claim.Generation))
	meta.SetStatusCondition(&claim.Status.Conditions, serviceAvailabilityCondition(
		claim.Status.Phase,
		publication,
		localResolved,
		localCluster,
		activeUpgradeRequest,
		activeRestoreRequest,
		activeRestoreExecution,
		claim.Generation,
	))
	meta.SetStatusCondition(&claim.Status.Conditions, maintenanceActiveCondition(activeUpgradeRequest, activeRestoreRequest, activeRestoreExecution, claim.Generation))
	claim.Status.Summary = desiredStatusSummary(
		claim,
		acceptance,
		contractResolution,
		materializationResult,
		ownershipResult,
		localCluster,
		publication,
		activeUpgradeRequest,
		activeBackupRequest,
		activeRestoreRequest,
		activeRestoreExecution,
	)

	if reflect.DeepEqual(original.Status, claim.Status) && !localClusterEnsured && !connectionChanged {
		logger.V(1).Info("OpenBaoClusterClaim status already up to date")
		return requeue, nil
	}

	if err := r.client.Status().Patch(ctx, claim, client.MergeFrom(original)); err != nil {
		return recon.Result{}, fmt.Errorf("patch OpenBaoClusterClaim status: %w", err)
	}

	logger.Info(
		"Reconciled OpenBaoClusterClaim state",
		"serviceClaimsEnabled",
		r.enableServiceClaims,
		"phase",
		claim.Status.Phase,
		"localClusterEnsured",
		localClusterEnsured,
	)
	return requeue, nil
}

func (r runtimeReconciler) resolveTenant(ctx context.Context, claim *openbaov1alpha1.OpenBaoClusterClaim) (*openbaov1alpha1.OpenBaoTenant, result) {
	if !r.claimsEnabled() {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Claim validation is disabled until service claims are enabled.",
		}
	}

	if claim.Spec.TenantRef.Name == "" {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim.spec.tenantRef.name must be set.",
		}
	}
	if localReferenceName(claim.Spec.ServiceOfferingRef) == "" && claim.Spec.ServiceProfileRef.Name == "" {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim requires either spec.serviceOfferingRef.name or spec.serviceProfileRef.name.",
		}
	}

	tenant := &openbaov1alpha1.OpenBaoTenant{}
	key := client.ObjectKey{Namespace: claim.Namespace, Name: claim.Spec.TenantRef.Name}
	if err := r.client.Get(ctx, key, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Referenced OpenBaoTenant does not exist yet.",
			}
		}
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoClusterClaim acceptance could not load the referenced OpenBaoTenant yet.",
		}
	}

	return tenant, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "OpenBaoClusterClaim tenant governance has been accepted.",
	}
}
