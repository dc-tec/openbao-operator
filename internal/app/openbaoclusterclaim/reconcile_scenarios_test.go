package openbaoclusterclaim

import (
	"context"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func TestReconcileOpenBaoClusterClaim(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name             string
		serviceClaims    bool
		skipCatalog      bool
		claim            *openbaov1alpha1.OpenBaoClusterClaim
		objects          []client.Object
		wantPhase        openbaov1alpha1.OpenBaoClusterClaimPhase
		wantController   metav1.ConditionStatus
		wantAccepted     metav1.ConditionStatus
		wantAcceptedBy   string
		wantContract     metav1.ConditionStatus
		wantContractBy   string
		wantPlacement    metav1.ConditionStatus
		wantPlacementBy  string
		wantOwnership    metav1.ConditionStatus
		wantOwnershipBy  string
		wantConnection   metav1.ConditionStatus
		wantConnectionBy string
		wantMode         openbaov1alpha1.OpenBaoClusterClaimMaterializationMode
		transitUnseal    SameClusterTransitUnsealConfig
		assertLocal      func(*testing.T, *openbaov1alpha1.OpenBaoClusterClaim, *openbaov1alpha1.OpenBaoCluster)
		wantLocalRef     *openbaov1alpha1.NamespacedReference
		wantApplied      *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus
		wantLocalObj     bool
		wantFinalizer    bool
		wantSecret       bool
		wantEndpoint     string
	}

	sameClusterBootstrapCase := func(
		name string,
		serviceProfileRef openbaov1alpha1.LocalReference,
		objects []client.Object,
		assertLocal func(*testing.T, *openbaov1alpha1.OpenBaoClusterClaim, *openbaov1alpha1.OpenBaoCluster),
		wantApplied *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus,
	) testCase {
		return testCase{
			name:          name,
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = serviceProfileRef
				return claim
			}(),
			objects:          objects,
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			assertLocal:      assertLocal,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      wantApplied,
			wantLocalObj:     true,
			wantFinalizer:    true,
		}
	}

	for _, tt := range []testCase{
		{
			name:             "disabled",
			serviceClaims:    false,
			claim:            validClaim(),
			objects:          []client.Object{validTenant()},
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhasePending,
			wantController:   metav1.ConditionFalse,
			wantAccepted:     metav1.ConditionFalse,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonFeatureDisabled),
			wantContract:     metav1.ConditionFalse,
			wantContractBy:   string(openbaov1alpha1.ReasonFeatureDisabled),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonFeatureDisabled),
			wantOwnership:    metav1.ConditionFalse,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonFeatureDisabled),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonFeatureDisabled),
			wantFinalizer:    false,
		},
		{
			name:             "missing tenant",
			serviceClaims:    true,
			claim:            validClaim(),
			objects:          nil,
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhasePending,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionFalse,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonPending),
			wantContract:     metav1.ConditionFalse,
			wantContractBy:   string(openbaov1alpha1.ReasonPending),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonPending),
			wantOwnership:    metav1.ConditionFalse,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonPending),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster claim resolves local materialization",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster claim records service offering provenance",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				return claim
			}(),
			objects: append(
				[]client.Object{validTenant(), validServiceOfferingForReconcile("standard-ha", "standard-ha-v1")},
				sameClusterCatalogObjects()...,
			),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatusWithStandardOffering(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster gateway claim materializes local gateway workload",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-gateway-v1"}
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterGatewayCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterGatewayAppliedStatus(),
			assertLocal:      assertProjectedLocalClusterWithGateway,
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		sameClusterBootstrapCase(
			"same-cluster claim resolves configmap-backed bootstrap auth config",
			openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"},
			append([]client.Object{validTenant(), validSameClusterAuthMethodConfigMap()}, sameClusterConfigRefCatalogObjects()...),
			assertProjectedLocalClusterWithAuthConfig,
			validSameClusterConfigRefAppliedStatus(),
		),
		sameClusterBootstrapCase(
			"same-cluster claim resolves secret-backed bootstrap auth config",
			openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"},
			append([]client.Object{validTenant(), validSameClusterAuthMethodSecret()}, sameClusterSecretConfigRefCatalogObjects()...),
			assertProjectedLocalClusterWithAuthConfig,
			validSameClusterSecretConfigRefAppliedStatus(),
		),
		sameClusterBootstrapCase(
			"same-cluster claim resolves configmap-backed bootstrap policy bundle",
			openbaov1alpha1.LocalReference{Name: "standard-ha-policy-v1"},
			append([]client.Object{validTenant(), validSameClusterPolicyContentConfigMap()}, sameClusterPolicyCatalogObjects()...),
			assertProjectedLocalClusterWithPolicyBundle,
			validSameClusterPolicyAppliedStatus(),
		),
		sameClusterBootstrapCase(
			"same-cluster claim resolves secret-backed bootstrap policy bundle",
			openbaov1alpha1.LocalReference{Name: "standard-ha-policy-v1"},
			append([]client.Object{validTenant(), validSameClusterPolicyContentSecret()}, sameClusterSecretPolicyCatalogObjects()...),
			assertProjectedLocalClusterWithPolicyBundle,
			validSameClusterSecretPolicyAppliedStatus(),
		),
		sameClusterBootstrapCase(
			"same-cluster claim resolves configmap-backed bootstrap audit sink",
			openbaov1alpha1.LocalReference{Name: "standard-ha-audit-v1"},
			append([]client.Object{validTenant(), validSameClusterAuditSinkConfigMap()}, sameClusterAuditCatalogObjects()...),
			assertProjectedLocalClusterWithAuditDevice,
			validSameClusterAuditAppliedStatus(),
		),
		sameClusterBootstrapCase(
			"same-cluster claim resolves secret-backed bootstrap audit sink",
			openbaov1alpha1.LocalReference{Name: "standard-ha-audit-v1"},
			append([]client.Object{validTenant(), validSameClusterAuditSinkSecret()}, sameClusterSecretAuditCatalogObjects()...),
			assertProjectedLocalClusterWithAuditDevice,
			validSameClusterSecretAuditAppliedStatus(),
		),
		{
			name:          "same-cluster hardened claim materializes local transit-unseal workload",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-hardened-v1"}
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterHardenedCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			transitUnseal:    validSameClusterTransitUnsealConfig(),
			assertLocal:      assertProjectedHardenedLocalCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterHardenedAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster backup-enabled claim materializes local backup workload",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-backup-v1"}
				claim.Spec.ServiceParameters = &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
					Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
						Location:  "payments-prod",
						Partition: "finance",
					},
				}
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterBackupEnabledCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			assertLocal:      assertProjectedLocalClusterWithBackup,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterBackupEnabledAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster hardened backup-enabled claim materializes local transit-unseal backup workload",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-hardened-backup-v1"}
				claim.Spec.ServiceParameters = &openbaov1alpha1.OpenBaoClusterClaimServiceParametersSpec{
					Backup: &openbaov1alpha1.OpenBaoClusterClaimBackupServiceParametersSpec{
						Location:  "payments-prod",
						Partition: "finance",
					},
				}
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterHardenedBackupCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			transitUnseal:    validSameClusterTransitUnsealConfig(),
			assertLocal:      assertProjectedHardenedLocalClusterWithBackup,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterHardenedBackupAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster direct-managed conflict fails ownership",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				return claim
			}(),
			objects: append([]client.Object{
				validTenant(),
				&openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
				},
			}, sameClusterCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionFalse,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonInvalid),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatus(),
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster existing claim-managed cluster owned by same claim is accepted",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				return claim
			}(),
			objects: append([]client.Object{
				validTenant(),
				&openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "payments-bao",
						Namespace: "payments",
						Labels: map[string]string{
							constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
							constants.LabelOpenBaoClaimNamespace: "payments",
							constants.LabelOpenBaoClaimName:      "payments-bao",
						},
					},
					Spec:   validExistingSameClusterConcreteSpec(),
					Status: openbaov1alpha1.OpenBaoClusterStatus{Phase: openbaov1alpha1.ClusterPhaseRunning},
				},
			}, sameClusterCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
		},
		{
			name:          "same-cluster ready workload publishes connection",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				return claim
			}(),
			objects: append([]client.Object{
				validTenant(),
				&openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "payments-bao",
						Namespace: "payments",
						Labels: map[string]string{
							constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
							constants.LabelOpenBaoClaimNamespace: "payments",
							constants.LabelOpenBaoClaimName:      "payments-bao",
						},
					},
					Spec:   validExistingSameClusterConcreteSpec(),
					Status: openbaov1alpha1.OpenBaoClusterStatus{Phase: openbaov1alpha1.ClusterPhaseRunning},
				},
				validSameClusterPublicService(),
				validSameClusterCASecret(),
			}, sameClusterCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionTrue,
			wantConnectionBy: string(openbaov1alpha1.ReasonReady),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
			wantSecret:       true,
			wantEndpoint:     validSameClusterEndpoint(),
		},
		{
			name:          "same-cluster gateway workload publishes external connection",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-gateway-v1"}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				return claim
			}(),
			objects: append([]client.Object{
				validTenant(),
				&openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "payments-bao",
						Namespace: "payments",
						Labels: map[string]string{
							constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
							constants.LabelOpenBaoClaimNamespace: "payments",
							constants.LabelOpenBaoClaimName:      "payments-bao",
						},
					},
					Spec: func() openbaov1alpha1.OpenBaoClusterSpec {
						spec := validExistingSameClusterConcreteSpec()
						spec.Gateway = &openbaov1alpha1.GatewayConfig{
							Enabled: true,
							GatewayRef: openbaov1alpha1.GatewayReference{
								Name:      "internal-gateway",
								Namespace: "networking",
							},
							Hostname: "payments-bao.example.internal",
							Path:     "/",
							BackendTLS: &openbaov1alpha1.BackendTLSConfig{
								Enabled: ptr.To(true),
							},
						}
						return spec
					}(),
					Status: openbaov1alpha1.OpenBaoClusterStatus{
						Phase: openbaov1alpha1.ClusterPhaseRunning,
						Conditions: []metav1.Condition{{
							Type:   string(openbaov1alpha1.ConditionGatewayIntegrationReady),
							Status: metav1.ConditionTrue,
						}},
					},
				},
				validSameClusterPublicService(),
				validSameClusterCASecret(),
			}, sameClusterGatewayCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseReady,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionTrue,
			wantPlacementBy:  string(openbaov1alpha1.ReasonAccepted),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionTrue,
			wantConnectionBy: string(openbaov1alpha1.ReasonReady),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterGatewayAppliedStatus(),
			wantLocalObj:     true,
			wantFinalizer:    true,
			wantSecret:       true,
			wantEndpoint:     validSameClusterGatewayEndpoint(),
		},
		{
			name:          "bound service profile continuity drift fails closed",
			serviceClaims: true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Status.Applied = openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
					ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
						Name: "standard-ha-v1",
						UID:  "stale-service-profile-uid",
					},
				}
				return claim
			}(),
			objects:          []client.Object{validTenant()},
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionFalse,
			wantContractBy:   string(openbaov1alpha1.ReasonInvalid),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonPending),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantApplied: &openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
				ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{
					Name: "standard-ha-v1",
					UID:  "stale-service-profile-uid",
				},
			},
			wantFinalizer: true,
		},
		{
			name:          "materialized service profile change is blocked until rollout support exists",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-gateway-v1"}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				claim.Status.Applied = derefAppliedStatus(validSameClusterAppliedStatus())
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterGatewayCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionFalse,
			wantContractBy:   string(openbaov1alpha1.ReasonInvalid),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonPending),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied:      validSameClusterAppliedStatus(),
			wantFinalizer:    true,
		},
		{
			name:          "rendered dependency continuity drift fails closed",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-gateway-v1"}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				applied := derefAppliedStatus(validSameClusterGatewayAppliedStatus())
				applied.RenderedDependencies.EntrypointRef.UID = "stale-entrypoint-uid"
				claim.Status.Applied = applied
				return claim
			}(),
			objects:          append([]client.Object{validTenant()}, sameClusterGatewayCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonInvalid),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied: func() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
				applied := validSameClusterGatewayAppliedStatus()
				applied.RenderedDependencies.EntrypointRef.UID = "stale-entrypoint-uid"
				return applied
			}(),
			wantFinalizer: true,
		},
		{
			name:          "bootstrap projected dependency continuity drift fails closed",
			serviceClaims: true,
			skipCatalog:   true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				applied := derefAppliedStatus(validSameClusterSecretConfigRefAppliedStatus())
				applied.RenderedDependencies.BootstrapProjectionIdentity.IdentityHash = "stale-bootstrap-projection-identity"
				claim.Status.Applied = applied
				return claim
			}(),
			objects:          append([]client.Object{validTenant(), validSameClusterAuthMethodSecret()}, sameClusterSecretConfigRefCatalogObjects()...),
			wantPhase:        openbaov1alpha1.OpenBaoClusterClaimPhaseFailed,
			wantController:   metav1.ConditionTrue,
			wantAccepted:     metav1.ConditionTrue,
			wantAcceptedBy:   string(openbaov1alpha1.ReasonAccepted),
			wantContract:     metav1.ConditionTrue,
			wantContractBy:   string(openbaov1alpha1.ReasonAccepted),
			wantPlacement:    metav1.ConditionFalse,
			wantPlacementBy:  string(openbaov1alpha1.ReasonInvalid),
			wantOwnership:    metav1.ConditionTrue,
			wantOwnershipBy:  string(openbaov1alpha1.ReasonAccepted),
			wantConnection:   metav1.ConditionFalse,
			wantConnectionBy: string(openbaov1alpha1.ReasonPending),
			wantMode:         openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
			wantLocalRef:     &openbaov1alpha1.NamespacedReference{Namespace: "payments", Name: "payments-bao"},
			wantApplied: func() *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
				applied := validSameClusterSecretConfigRefAppliedStatus()
				applied.RenderedDependencies.BootstrapProjectionIdentity.IdentityHash = "stale-bootstrap-projection-identity"
				return applied
			}(),
			wantFinalizer: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			statusObjects := append([]client.Object{tt.claim}, tt.objects...)
			scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
			objects := make([]client.Object, 0, len(tt.objects)+1)
			objects = append(objects, tt.claim.DeepCopy())
			if !tt.skipCatalog {
				objects = append(objects, cloneObjects(validCatalogObjects())...)
			}
			objects = append(objects, cloneObjects(tt.objects)...)
			c := builder.WithObjects(objects...).Build()

			reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
				runtimeCfg.EnableServiceClaims = tt.serviceClaims
				runtimeCfg.SameClusterTransitUnseal = tt.transitUnseal
			})
			_, updated := reconcileClaimOnce(t, c, reconciler, tt.claim)

			if updated.Status.Phase != tt.wantPhase {
				t.Fatalf("Phase = %q, want %q", updated.Status.Phase, tt.wantPhase)
			}
			if updated.Status.ObservedGeneration != updated.Generation {
				t.Fatalf("ObservedGeneration = %d, want %d", updated.Status.ObservedGeneration, updated.Generation)
			}
			assertCondition(t, updated.Status.Conditions, conditionTypeControllerActive, tt.wantController, "")
			assertCondition(t, updated.Status.Conditions, conditionTypeAccepted, tt.wantAccepted, tt.wantAcceptedBy)
			assertCondition(t, updated.Status.Conditions, conditionTypeServiceContract, tt.wantContract, tt.wantContractBy)
			assertCondition(t, updated.Status.Conditions, conditionTypeMaterialization, tt.wantPlacement, tt.wantPlacementBy)
			assertCondition(t, updated.Status.Conditions, conditionTypeOwnershipReady, tt.wantOwnership, tt.wantOwnershipBy)
			assertCondition(t, updated.Status.Conditions, conditionTypeConnectionPublished, tt.wantConnection, tt.wantConnectionBy)
			if hasFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer) != tt.wantFinalizer {
				t.Fatalf("claim finalizers = %v, want finalizer present=%t", updated.Finalizers, tt.wantFinalizer)
			}
			if !reflect.DeepEqual(updated.Status.Applied, derefAppliedStatus(tt.wantApplied)) {
				t.Fatalf("Applied = %#v, want %#v", updated.Status.Applied, derefAppliedStatus(tt.wantApplied))
			}

			if updated.Status.Materialization.Mode != tt.wantMode {
				t.Fatalf("Materialization.Mode = %q, want %q", updated.Status.Materialization.Mode, tt.wantMode)
			}
			if tt.wantLocalRef == nil {
				if updated.Status.Materialization.LocalRef != nil {
					t.Fatalf("LocalRef = %#v, want nil", updated.Status.Materialization.LocalRef)
				}
			} else {
				if updated.Status.Materialization.LocalRef == nil || !reflect.DeepEqual(*updated.Status.Materialization.LocalRef, *tt.wantLocalRef) {
					t.Fatalf("LocalRef = %#v, want %#v", updated.Status.Materialization.LocalRef, tt.wantLocalRef)
				}
			}
			if tt.wantLocalObj {
				localCluster := &openbaov1alpha1.OpenBaoCluster{}
				if err := c.Get(context.Background(), client.ObjectKey{Namespace: tt.wantLocalRef.Namespace, Name: tt.wantLocalRef.Name}, localCluster); err != nil {
					t.Fatalf("Get local cluster() error = %v", err)
				}
				if tt.assertLocal != nil {
					tt.assertLocal(t, updated, localCluster)
				} else {
					assertProjectedLocalCluster(t, updated, localCluster)
				}
			}
			secret := &corev1.Secret{}
			err := c.Get(context.Background(), client.ObjectKey{Namespace: updated.Namespace, Name: connectionpublishing.SecretName(updated.Name)}, secret)
			if tt.wantSecret {
				assertPublishedConnection(t, updated, secret, err, tt.wantEndpoint)
				return
			}
			if !apierrors.IsNotFound(err) {
				t.Fatalf("expected no connection secret, got err=%v", err)
			}
		})
	}
}

func TestReconcileOpenBaoClusterClaimFailsInvalidSameClusterPreUpgradeSnapshotWithoutBackup(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-snapshot-invalid-v1"}

	serviceProfile := sameClusterServiceProfile()
	serviceProfile.Name = "standard-ha-snapshot-invalid-v1"
	preUpgradeSnapshot := true
	serviceProfile.Spec.Lifecycle.PreUpgradeSnapshot = &preUpgradeSnapshot

	scheme, builder := newClaimTestClientBuilder(t, claim)
	c := builder.WithObjects(
		claim.DeepCopy(),
		validTenant(),
		serviceProfile,
		sameClusterBootstrapProfile(),
		sameClusterExposureClass(),
		sameClusterBackupProfile(),
	).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)

	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseFailed)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceContract, metav1.ConditionTrue, string(openbaov1alpha1.ReasonAccepted))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaterialization, metav1.ConditionFalse, string(openbaov1alpha1.ReasonInvalid))
	assertCondition(t, updated.Status.Conditions, conditionTypeOwnershipReady, metav1.ConditionTrue, string(openbaov1alpha1.ReasonAccepted))
	assertCondition(t, updated.Status.Conditions, conditionTypeConnectionPublished, metav1.ConditionFalse, string(openbaov1alpha1.ReasonPending))

	materialization := findCondition(updated.Status.Conditions, conditionTypeMaterialization)
	if materialization == nil || materialization.Message != "Same-cluster pre-upgrade snapshots require a rendered backup contract that can be projected into OpenBaoCluster.spec.backup." {
		t.Fatalf("materialization condition = %#v, want concrete pre-upgrade snapshot validation message", materialization)
	}
	connection := findCondition(updated.Status.Conditions, conditionTypeConnectionPublished)
	if connection == nil || connection.Message != "Connection publication is waiting for the local concrete workload." {
		t.Fatalf("connection condition = %#v, want local pending publication message", connection)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "payments-bao"}, cluster); !apierrors.IsNotFound(err) {
		t.Fatalf("Get projected OpenBaoCluster() error = %v, want not found", err)
	}
}

func TestReconcileOpenBaoClusterClaimPinsServiceOfferingBeforeMaterialization(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}

	statusObjects := append([]client.Object{claim, validTenant(), validServiceOfferingForReconcile("standard-ha", "standard-ha-v1")}, sameClusterCatalogObjects()...)
	scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
	c := builder.WithObjects(
		append(
			[]client.Object{claim.DeepCopy(), validTenant(), validServiceOfferingForReconcile("standard-ha", "standard-ha-v1")},
			cloneObjects(sameClusterCatalogObjects())...,
		)...,
	).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Spec.ServiceProfileRef.Name != "standard-ha-v1" {
		t.Fatalf("ServiceProfileRef after first reconcile = %q, want %q", updated.Spec.ServiceProfileRef.Name, "standard-ha-v1")
	}
	if !hasFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer) {
		t.Fatalf("expected claim finalizer after first reconcile, got %v", updated.Finalizers)
	}

	_, updated = reconcileClaimOnce(t, c, reconciler, claim)

	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning {
		t.Fatalf("Phase after second reconcile = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning)
	}
	if !reflect.DeepEqual(updated.Status.Applied, derefAppliedStatus(validSameClusterAppliedStatusWithStandardOffering())) {
		t.Fatalf("Applied after second reconcile = %#v, want %#v", updated.Status.Applied, derefAppliedStatus(validSameClusterAppliedStatusWithStandardOffering()))
	}
}

func TestReconcileOpenBaoClusterClaimRequeuesWhileBootstrapDependencyIsPending(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}

	statusObjects := append([]client.Object{claim, validTenant()}, sameClusterSecretConfigRefCatalogObjects()...)
	scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
	c := builder.WithObjects(
		append([]client.Object{claim.DeepCopy(), validTenant()}, cloneObjects(sameClusterSecretConfigRefCatalogObjects())...)...,
	).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	result, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhasePending {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhasePending)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceContract, metav1.ConditionTrue, string(openbaov1alpha1.ReasonAccepted))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaterialization, metav1.ConditionFalse, string(openbaov1alpha1.ReasonPending))

	materialization := findCondition(updated.Status.Conditions, conditionTypeMaterialization)
	if materialization == nil || materialization.Message != "Bootstrap auth-method config Secret does not exist yet." {
		t.Fatalf("materialization condition = %#v, want pending missing bootstrap dependency message", materialization)
	}
	if updated.Status.Materialization.LocalRef != nil {
		t.Fatalf("Materialization.LocalRef = %#v, want nil while bootstrap dependency is still pending", updated.Status.Materialization.LocalRef)
	}

	projected := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "payments-bao"}, projected); !apierrors.IsNotFound(err) {
		t.Fatalf("Get projected OpenBaoCluster() error = %v, want not found", err)
	}
}

func TestReconcileOpenBaoClusterClaimKeepsLocalRefNilAcrossPendingBootstrapRetriesWithoutRenderedContract(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
	claim.Status.Applied = openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
		ServiceProfileRef: &openbaov1alpha1.OpenBaoClusterClaimBoundRevisionReference{Name: "standard-ha-configref-v1", UID: "uid-service-profile"},
		ApprovedContract:  &openbaov1alpha1.OpenBaoClusterClaimContractIdentityStatus{IdentityHash: "sha256:approved"},
	}

	statusObjects := append([]client.Object{claim, validTenant()}, sameClusterSecretConfigRefCatalogObjects()...)
	scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
	c := builder.WithObjects(
		append([]client.Object{claim.DeepCopy(), validTenant()}, cloneObjects(sameClusterSecretConfigRefCatalogObjects())...)...,
	).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	result, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, constants.RequeueShort)
	}
	if updated.Status.Materialization.LocalRef != nil {
		t.Fatalf("Materialization.LocalRef = %#v, want nil while bootstrap dependency is still pending without any rendered contract", updated.Status.Materialization.LocalRef)
	}
	if updated.Status.Applied.RenderedContract != nil {
		t.Fatalf("Applied.RenderedContract = %#v, want nil while bootstrap dependency is still pending", updated.Status.Applied.RenderedContract)
	}
}

func TestReconcileOpenBaoClusterClaimMaterializesIngressAndPublishesConnection(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-ingress-v1"}

	statusObjects := append([]client.Object{claim, &openbaov1alpha1.OpenBaoCluster{}, validTenant()}, sameClusterIngressCatalogObjects()...)
	scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
	c := builder.WithObjects(
		append([]client.Object{claim.DeepCopy(), validTenant()}, cloneObjects(sameClusterIngressCatalogObjects())...)...,
	).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Materialization.Mode != openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster {
		t.Fatalf("materialization mode = %q, want SameCluster", updated.Status.Materialization.Mode)
	}
	local := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "payments-bao"}, local); err != nil {
		t.Fatalf("Get local OpenBaoCluster error = %v", err)
	}
	assertProjectedLocalClusterWithIngress(t, updated, local)
	if updated.Status.Connection.Endpoint != "" {
		t.Fatalf("connection endpoint after first reconcile = %q, want empty while ingress is not ready", updated.Status.Connection.Endpoint)
	}

	local.Status.Phase = openbaov1alpha1.ClusterPhaseRunning
	local.Status.Conditions = []metav1.Condition{{
		Type:   string(openbaov1alpha1.ConditionIngressIntegrationReady),
		Status: metav1.ConditionTrue,
		Reason: "IngressIntegrationReady",
	}}
	if err := c.Status().Update(context.Background(), local); err != nil {
		t.Fatalf("Update local OpenBaoCluster status error = %v", err)
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      connectionpublishing.LocalPublicServiceName("payments-bao"),
			CreationTimestamp: metav1.NewTime(time.Date(
				2026, time.April, 20, 17, 0, 0, 0, time.UTC,
			)),
		},
	}
	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      connectionpublishing.LocalCASecretName("payments-bao"),
			CreationTimestamp: metav1.NewTime(time.Date(
				2026, time.April, 20, 18, 0, 0, 0, time.UTC,
			)),
		},
		Data: map[string][]byte{
			"ca.crt": []byte("-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"),
		},
	}
	if err := c.Create(context.Background(), service); err != nil {
		t.Fatalf("Create local public Service error = %v", err)
	}
	if err := c.Create(context.Background(), caSecret); err != nil {
		t.Fatalf("Create local CA Secret error = %v", err)
	}

	_, updated = reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Connection.Endpoint != "https://payments-bao.example.internal" {
		t.Fatalf("connection endpoint after second reconcile = %q, want ingress hostname", updated.Status.Connection.Endpoint)
	}
}
