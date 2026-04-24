package openbaoclusterclaim

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

func (r runtimeReconciler) resolveApprovedServiceContract(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	activeUpgradeRequest *openbaov1alpha1.OpenBaoClusterClaimUpgradeRequest,
	catalog *claimcontract.CatalogBundle,
	acceptance result,
	catalogResolution result,
) (*claimcontract.ApprovedServiceContract, result) {
	if !r.claimsEnabled() {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Approved service contract resolution is disabled until service claims or multi-cluster support is enabled.",
		}
	}
	if !acceptance.Valid {
		if acceptance.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			return nil, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonFeatureDisabled,
				Message: "Approved service contract resolution is disabled until service claims or multi-cluster support is enabled.",
			}
		}
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Approved service contract resolution is waiting for claim acceptance.",
		}
	}
	if !catalogResolution.Valid || catalog == nil {
		if catalogResolution.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			return nil, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonFeatureDisabled,
				Message: "Approved service contract resolution is disabled until service claims or multi-cluster support is enabled.",
			}
		}
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Approved service contract resolution is waiting for immutable catalog inputs.",
		}
	}

	contract, validation := claimcontract.BindApprovedServiceContract(claim, catalog)
	if !validation.Valid {
		return nil, result{
			Valid:   false,
			Reason:  validation.Reason,
			Message: validation.Message,
		}
	}
	if specLock := validateMaterializedServiceSelectionChange(claim, activeUpgradeRequest); !specLock.Valid {
		return nil, specLock
	}

	continuity := claimcontract.ValidateContinuity(claim.Status.Applied, contract)
	if !continuity.Valid {
		return nil, result{
			Valid:   false,
			Reason:  continuity.Reason,
			Message: continuity.Message,
		}
	}

	return contract, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Approved service contract has been resolved from immutable catalog inputs.",
	}
}

func (r runtimeReconciler) resolveCatalogBundle(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	acceptance result,
) (*claimcontract.CatalogBundle, result) {
	if !r.claimsEnabled() {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Immutable catalog resolution is disabled until service claims or multi-cluster support is enabled.",
		}
	}
	if !acceptance.Valid {
		if acceptance.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			return nil, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonFeatureDisabled,
				Message: "Immutable catalog resolution is disabled until service claims or multi-cluster support is enabled.",
			}
		}
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Immutable catalog resolution is waiting for claim acceptance.",
		}
	}

	requestedProfileName, selectionResolution := r.resolveRequestedServiceProfileName(ctx, claim)
	if !selectionResolution.Valid {
		return nil, selectionResolution
	}

	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if loadResult := r.loadClusterCatalogObject(
		ctx,
		requestedProfileName,
		serviceProfile,
		"Referenced OpenBaoServiceProfile does not exist yet.",
		"Immutable catalog resolution could not load the referenced OpenBaoServiceProfile yet.",
	); !loadResult.Valid {
		return nil, loadResult
	}

	catalog := &claimcontract.CatalogBundle{ServiceProfile: serviceProfile}

	if loadResult := r.loadImplementationProfiles(ctx, serviceProfile, catalog); !loadResult.Valid {
		return nil, loadResult
	}

	catalog.ExposureClass = &openbaov1alpha1.OpenBaoExposureClass{}
	if loadResult := r.loadClusterCatalogObject(
		ctx,
		serviceProfile.Spec.Exposure.ClassRef.Name,
		catalog.ExposureClass,
		"Referenced OpenBaoExposureClass does not exist yet.",
		"Immutable catalog resolution could not load the referenced OpenBaoExposureClass yet.",
	); !loadResult.Valid {
		return nil, loadResult
	}
	if catalog.ExposureClass.Spec.EntrypointRef != nil {
		catalog.Entrypoint = &openbaov1alpha1.OpenBaoEntrypoint{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			catalog.ExposureClass.Spec.EntrypointRef.Name,
			catalog.Entrypoint,
			"Referenced OpenBaoEntrypoint does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoEntrypoint yet.",
		); !loadResult.Valid {
			return nil, loadResult
		}
	}
	if catalog.ExposureClass.Spec.IngressPolicyRef != nil {
		catalog.IngressPolicy = &openbaov1alpha1.OpenBaoIngressPolicy{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			catalog.ExposureClass.Spec.IngressPolicyRef.Name,
			catalog.IngressPolicy,
			"Referenced OpenBaoIngressPolicy does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoIngressPolicy yet.",
		); !loadResult.Valid {
			return nil, loadResult
		}
	}

	catalog.BackupProfile = &openbaov1alpha1.OpenBaoBackupProfile{}
	if loadResult := r.loadClusterCatalogObject(
		ctx,
		serviceProfile.Spec.Backup.ProfileRef.Name,
		catalog.BackupProfile,
		"Referenced OpenBaoBackupProfile does not exist yet.",
		"Immutable catalog resolution could not load the referenced OpenBaoBackupProfile yet.",
	); !loadResult.Valid {
		return nil, loadResult
	}

	if catalog.BackupProfile.Spec.TargetRef != nil {
		catalog.BackupTarget = &openbaov1alpha1.OpenBaoBackupTarget{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			catalog.BackupProfile.Spec.TargetRef.Name,
			catalog.BackupTarget,
			"Referenced OpenBaoBackupTarget does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoBackupTarget yet.",
		); !loadResult.Valid {
			return nil, loadResult
		}

		catalog.BackupBackend = &openbaov1alpha1.OpenBaoBackupBackend{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			catalog.BackupTarget.Spec.BackendRef.Name,
			catalog.BackupBackend,
			"Referenced OpenBaoBackupBackend does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoBackupBackend yet.",
		); !loadResult.Valid {
			return nil, loadResult
		}

		if catalog.BackupTarget.Spec.AuthProfileRef != nil {
			catalog.BackupAuth = &openbaov1alpha1.OpenBaoBackupAuthProfile{}
			if loadResult := r.loadClusterCatalogObject(
				ctx,
				catalog.BackupTarget.Spec.AuthProfileRef.Name,
				catalog.BackupAuth,
				"Referenced OpenBaoBackupAuthProfile does not exist yet.",
				"Immutable catalog resolution could not load the referenced OpenBaoBackupAuthProfile yet.",
			); !loadResult.Valid {
				return nil, loadResult
			}
		}

		if catalog.BackupTarget.Spec.TransportProfileRef != nil {
			catalog.TransferProfile = &openbaov1alpha1.OpenBaoTransferProfile{}
			if loadResult := r.loadClusterCatalogObject(
				ctx,
				catalog.BackupTarget.Spec.TransportProfileRef.Name,
				catalog.TransferProfile,
				"Referenced OpenBaoTransferProfile does not exist yet.",
				"Immutable catalog resolution could not load the referenced OpenBaoTransferProfile yet.",
			); !loadResult.Valid {
				return nil, loadResult
			}
		}
	}

	return catalog, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "Immutable top-level catalog revisions have been loaded for the claim.",
	}
}

func (r runtimeReconciler) loadImplementationProfiles(
	ctx context.Context,
	serviceProfile *openbaov1alpha1.OpenBaoServiceProfile,
	catalog *claimcontract.CatalogBundle,
) result {
	if serviceProfile.Spec.Bootstrap.ProfileRef != nil {
		catalog.BootstrapProfile = &openbaov1alpha1.OpenBaoBootstrapProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Bootstrap.ProfileRef.Name,
			catalog.BootstrapProfile,
			"Referenced OpenBaoBootstrapProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoBootstrapProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Storage.ProfileRef != nil {
		catalog.StorageProfile = &openbaov1alpha1.OpenBaoStorageProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Storage.ProfileRef.Name,
			catalog.StorageProfile,
			"Referenced OpenBaoStorageProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoStorageProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Unseal != nil && serviceProfile.Spec.Unseal.ProfileRef != nil {
		catalog.UnsealProfile = &openbaov1alpha1.OpenBaoUnsealProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Unseal.ProfileRef.Name,
			catalog.UnsealProfile,
			"Referenced OpenBaoUnsealProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoUnsealProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Runtime != nil && serviceProfile.Spec.Runtime.ProfileRef != nil {
		catalog.RuntimeProfile = &openbaov1alpha1.OpenBaoRuntimeProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Runtime.ProfileRef.Name,
			catalog.RuntimeProfile,
			"Referenced OpenBaoRuntimeProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoRuntimeProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Observability != nil && serviceProfile.Spec.Observability.ProfileRef != nil {
		catalog.ObservabilityProfile = &openbaov1alpha1.OpenBaoObservabilityProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Observability.ProfileRef.Name,
			catalog.ObservabilityProfile,
			"Referenced OpenBaoObservabilityProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoObservabilityProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Network != nil && serviceProfile.Spec.Network.ProfileRef != nil {
		catalog.NetworkProfile = &openbaov1alpha1.OpenBaoNetworkProfile{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Network.ProfileRef.Name,
			catalog.NetworkProfile,
			"Referenced OpenBaoNetworkProfile does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoNetworkProfile yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}
	if serviceProfile.Spec.Lifecycle.PolicyRef != nil {
		catalog.UpgradePolicy = &openbaov1alpha1.OpenBaoUpgradePolicy{}
		if loadResult := r.loadClusterCatalogObject(
			ctx,
			serviceProfile.Spec.Lifecycle.PolicyRef.Name,
			catalog.UpgradePolicy,
			"Referenced OpenBaoUpgradePolicy does not exist yet.",
			"Immutable catalog resolution could not load the referenced OpenBaoUpgradePolicy yet.",
		); !loadResult.Valid {
			return loadResult
		}
	}

	return result{Valid: true}
}

func (r runtimeReconciler) loadClusterCatalogObject(
	ctx context.Context,
	name string,
	obj client.Object,
	notFoundMessage string,
	pendingMessage string,
) result {
	if err := r.client.Get(ctx, client.ObjectKey{Name: name}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: notFoundMessage,
			}
		}
		return result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: pendingMessage,
		}
	}
	return result{Valid: true}
}

func (r runtimeReconciler) reconcileServiceOfferingSelection(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (bool, error) {
	if claim == nil || !r.claimsEnabled() {
		return false, nil
	}
	offeringName := localReferenceName(claim.Spec.ServiceOfferingRef)
	if offeringName == "" || claim.Status.Materialization.Mode != "" {
		return false, nil
	}

	revisionName, offeringResolution := r.resolveServiceOfferingRevision(ctx, offeringName)
	if !offeringResolution.Valid {
		return false, nil
	}
	if strings.TrimSpace(claim.Spec.ServiceProfileRef.Name) == revisionName {
		return false, nil
	}

	updated := claim.DeepCopy()
	updated.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: revisionName}
	if err := r.client.Update(ctx, updated); err != nil {
		return false, fmt.Errorf(
			"pin OpenBaoClusterClaim %s/%s to OpenBaoServiceOffering %q revision %q: %w",
			claim.Namespace,
			claim.Name,
			offeringName,
			revisionName,
			err,
		)
	}
	claim.Spec.ServiceProfileRef = updated.Spec.ServiceProfileRef
	return true, nil
}

func (r runtimeReconciler) resolveRequestedServiceProfileName(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
) (string, result) {
	if claim == nil {
		return "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "OpenBaoClusterClaim is required to resolve a requested service-profile revision.",
		}
	}

	requestedProfileName := strings.TrimSpace(claim.Spec.ServiceProfileRef.Name)
	offeringName := localReferenceName(claim.Spec.ServiceOfferingRef)
	if offeringName == "" {
		if requestedProfileName == "" {
			return "", result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonInvalid,
				Message: "OpenBaoClusterClaim requires either spec.serviceOfferingRef.name or spec.serviceProfileRef.name.",
			}
		}
		return requestedProfileName, result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim uses an explicit pinned service-profile revision.",
		}
	}

	revisionName, offeringResolution := r.resolveServiceOfferingRevision(ctx, offeringName)
	if !offeringResolution.Valid {
		return "", offeringResolution
	}
	if requestedProfileName == "" || claim.Status.Materialization.Mode == "" && requestedProfileName != revisionName {
		return "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoClusterClaim service-offering selection is waiting for its pinned service-profile revision to be stored.",
		}
	}

	return requestedProfileName, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "OpenBaoClusterClaim service-offering selection has been pinned to an immutable service-profile revision.",
	}
}

func (r runtimeReconciler) resolveServiceOfferingRevision(
	ctx context.Context,
	offeringName string,
) (string, result) {
	offering := &openbaov1alpha1.OpenBaoServiceOffering{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: offeringName}, offering); err != nil {
		if apierrors.IsNotFound(err) {
			return "", result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Referenced OpenBaoServiceOffering does not exist yet.",
			}
		}
		return "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoClusterClaim could not load the referenced OpenBaoServiceOffering yet.",
		}
	}

	revisionName := strings.TrimSpace(offering.Spec.CurrentRevisionRef.Name)
	if revisionName == "" {
		return "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Referenced OpenBaoServiceOffering does not define spec.currentRevisionRef.name.",
		}
	}

	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := r.client.Get(ctx, client.ObjectKey{Name: revisionName}, serviceProfile); err != nil {
		if apierrors.IsNotFound(err) {
			return "", result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonPending,
				Message: "Referenced OpenBaoServiceOffering points at an OpenBaoServiceProfile that does not exist yet.",
			}
		}
		return "", result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "OpenBaoClusterClaim could not load the OpenBaoServiceProfile referenced by the selected OpenBaoServiceOffering yet.",
		}
	}

	return revisionName, result{
		Valid:   true,
		Reason:  openbaov1alpha1.ReasonAccepted,
		Message: "OpenBaoServiceOffering has been resolved to its current immutable service-profile revision.",
	}
}
