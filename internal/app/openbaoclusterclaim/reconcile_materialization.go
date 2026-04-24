package openbaoclusterclaim

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

func (r runtimeReconciler) resolveSameClusterMaterialization(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	tenant *openbaov1alpha1.OpenBaoTenant,
	acceptance result,
) (*openbaov1alpha1.NamespacedReference, result) {
	if !r.enableServiceClaims {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Same-cluster claim materialization is disabled until service claims are enabled.",
		}
	}
	if !acceptance.Valid || tenant == nil {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Same-cluster materialization is waiting for tenant acceptance.",
		}
	}
	if tenant.Spec.TargetNamespace == "" {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonInvalid,
			Message: "Referenced OpenBaoTenant must set spec.targetNamespace for same-cluster materialization.",
		}
	}

	return &openbaov1alpha1.NamespacedReference{
			Namespace: tenant.Spec.TargetNamespace,
			Name:      claimcontract.ClaimManagedLocalClusterName(claim.Name),
		}, result{
			Valid:   true,
			Reason:  openbaov1alpha1.ReasonAccepted,
			Message: "OpenBaoClusterClaim resolved to the same-cluster materialization path.",
		}
}

func (r runtimeReconciler) resolveRenderedExecutionContract(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	localResolved result,
	approved *claimcontract.ApprovedServiceContract,
	catalog *claimcontract.CatalogBundle,
	bootstrapInputs claimcontract.SameClusterBootstrapResolvedInputs,
	bootstrapResolution result,
	approvedResult result,
) (*claimcontract.RenderedExecutionContract, result) {
	if !approvedResult.Valid || approved == nil {
		if approvedResult.Reason == openbaov1alpha1.ReasonFeatureDisabled {
			return nil, result{
				Valid:   false,
				Reason:  openbaov1alpha1.ReasonFeatureDisabled,
				Message: "Rendered execution contract resolution is disabled until approved contract resolution is enabled.",
			}
		}
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonPending,
			Message: "Rendered execution contract resolution is waiting for the approved service contract.",
		}
	}
	if localResolved.Valid && localTarget != nil {
		if !bootstrapResolution.Valid {
			return nil, bootstrapResolution
		}
		rendered, renderResult := claimcontract.RenderSameClusterExecutionContract(
			claim,
			localTarget,
			approved,
			catalog,
			claimcontract.SameClusterTransitUnsealDefaults{
				Address:               r.sameClusterTransitUnseal.Address,
				KeyName:               r.sameClusterTransitUnseal.KeyName,
				MountPath:             r.sameClusterTransitUnseal.MountPath,
				Namespace:             r.sameClusterTransitUnseal.Namespace,
				TLSServerName:         r.sameClusterTransitUnseal.TLSServerName,
				CredentialsSecretName: r.sameClusterTransitUnseal.CredentialsSecretName,
			},
			bootstrapInputs,
		)
		if renderResult.Valid {
			claimcontract.ApplySameClusterNetworkDefaults(rendered, claimcontract.SameClusterNetworkDefaults{
				APIServerCIDR:        r.sameClusterNetwork.APIServerCIDR,
				APIServerEndpointIPs: r.sameClusterNetwork.APIServerEndpointIPs,
				DNSEndpointIPs:       r.sameClusterNetwork.DNSEndpointIPs,
			})
			if continuity := claimcontract.ValidateRenderedDependencyContinuity(claim.Status.Applied, rendered); !continuity.Valid {
				return nil, result{
					Valid:   false,
					Reason:  continuity.Reason,
					Message: continuity.Message,
				}
			}
		}
		return rendered, result{
			Valid:   renderResult.Valid,
			Reason:  renderResult.Reason,
			Message: renderResult.Message,
		}
	}
	if localResolved.Reason == openbaov1alpha1.ReasonFeatureDisabled {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Rendered execution contract resolution is disabled until same-cluster service claims are enabled.",
		}
	}
	return nil, result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonPending,
		Message: "Rendered execution contract resolution is waiting for materialization-path selection.",
	}
}

func (r runtimeReconciler) resolveDesiredLocalCluster(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localResolved result,
	rendered *claimcontract.RenderedExecutionContract,
	renderedResult result,
) (*openbaov1alpha1.OpenBaoCluster, result) {
	if localResolved.Valid {
		if !renderedResult.Valid || rendered == nil {
			if renderedResult.Reason == openbaov1alpha1.ReasonFeatureDisabled {
				return nil, result{
					Valid:   false,
					Reason:  openbaov1alpha1.ReasonFeatureDisabled,
					Message: "Same-cluster concrete workload materialization is disabled until rendered execution contracts are available.",
				}
			}
			return nil, result{
				Valid:   false,
				Reason:  renderedResult.Reason,
				Message: renderedResult.Message,
			}
		}
		cluster, validation := claimcontract.DesiredSameClusterCluster(claim, rendered)
		return cluster, result{
			Valid:   validation.Valid,
			Reason:  validation.Reason,
			Message: validation.Message,
		}
	}
	if localResolved.Reason == openbaov1alpha1.ReasonFeatureDisabled {
		return nil, result{
			Valid:   false,
			Reason:  openbaov1alpha1.ReasonFeatureDisabled,
			Message: "Same-cluster concrete workload materialization is disabled until service claims are enabled.",
		}
	}
	return nil, result{
		Valid:   false,
		Reason:  openbaov1alpha1.ReasonPending,
		Message: "Same-cluster concrete workload materialization is waiting for materialization-path selection.",
	}
}
