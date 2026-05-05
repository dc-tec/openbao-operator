package openbaoclusterclaim

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

type deleteClaimTestCase struct {
	name             string
	serviceClaims    bool
	claim            *openbaov1alpha1.OpenBaoClusterClaim
	objects          []client.Object
	wantFinalizer    bool
	wantLocalAbsent  bool
	wantLocalDelete  bool
	wantLocalKeep    bool
	wantProjectedAbs bool
}

func TestReconcileDeletingOpenBaoClusterClaim(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC))

	for _, tt := range []deleteClaimTestCase{
		{
			name:          "missing same-cluster workload removes finalizer",
			serviceClaims: true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Finalizers = []string{openbaov1alpha1.OpenBaoClusterClaimFinalizer}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				claim.DeletionTimestamp = &now
				return claim
			}(),
			wantFinalizer:   false,
			wantLocalAbsent: true,
		},
		{
			name:          "missing same-cluster workload cleans up projected bootstrap artifacts",
			serviceClaims: true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Finalizers = []string{openbaov1alpha1.OpenBaoClusterClaimFinalizer}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				claim.DeletionTimestamp = &now
				return claim
			}(),
			objects: []client.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-bootstrap-policy-a1b2c3d4",
						Namespace: "payments",
						Labels:    bootstrapProjectionLabels(validClaim()),
					},
					Data: map[string]string{"content": `path "kv/data/*" { capabilities = ["read"] }`},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "claim-bootstrap-audit-a1b2c3d4",
						Namespace: "payments",
						Labels:    bootstrapProjectionLabels(validClaim()),
					},
					Data: map[string][]byte{"sink.json": []byte(`{"path":"stdout"}`)},
				},
			},
			wantFinalizer:    false,
			wantLocalAbsent:  true,
			wantProjectedAbs: true,
		},
		{
			name:          "same-cluster owned workload is deleted before finalizer removal",
			serviceClaims: true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Finalizers = []string{openbaov1alpha1.OpenBaoClusterClaimFinalizer}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				claim.DeletionTimestamp = &now
				claim.Status.Phase = openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting
				return claim
			}(),
			objects: []client.Object{func() client.Object {
				cluster := &openbaov1alpha1.OpenBaoCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "payments-bao",
						Namespace:  "payments",
						Finalizers: []string{openbaov1alpha1.OpenBaoClusterFinalizer},
						Labels: map[string]string{
							constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
							constants.LabelOpenBaoClaimNamespace: "payments",
							constants.LabelOpenBaoClaimName:      "payments-bao",
						},
					},
				}
				return cluster
			}()},
			wantFinalizer:   true,
			wantLocalDelete: true,
		},
		{
			name:          "same-cluster direct-managed workload does not block finalizer removal",
			serviceClaims: true,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaim()
				claim.Finalizers = []string{openbaov1alpha1.OpenBaoClusterClaimFinalizer}
				claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
					Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
					LocalRef: &openbaov1alpha1.NamespacedReference{
						Namespace: "payments",
						Name:      "payments-bao",
					},
				}
				claim.DeletionTimestamp = &now
				return claim
			}(),
			objects: []client.Object{&openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "payments-bao",
					Namespace: "payments",
					Labels: map[string]string{
						constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipDirectManaged,
					},
				},
			}},
			wantFinalizer: false,
			wantLocalKeep: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runDeleteClaimReconcileTest(t, tt)
		})
	}
}

func TestReconcileDeletingOpenBaoClusterClaimDeletesProjectedSecretWithoutTenantSecretGet(t *testing.T) {
	t.Parallel()

	now := metav1.NewTime(time.Date(2026, time.April, 20, 12, 0, 0, 0, time.UTC))
	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
	claim.Finalizers = []string{openbaov1alpha1.OpenBaoClusterClaimFinalizer}
	claim.DeletionTimestamp = &now
	claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}
	applied := validSameClusterSecretConfigRefAppliedStatus()
	claim.Status.Applied = *applied
	projectedRef := applied.RenderedDependencies.BootstrapProjectionRefs[0]
	projectedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectedRef.Name,
			Namespace: "payments",
			Labels:    bootstrapProjectionLabels(claim),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"config.json": []byte(`{"issuer":"https://kubernetes.default.svc"}`)},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim, projectedSecret)
	baseClient := builder.WithObjects(claim.DeepCopy(), projectedSecret.DeepCopy()).Build()
	reader := &interceptGetClient{
		Client:              baseClient,
		forbiddenSecretKey:  types.NamespacedName{Namespace: "payments", Name: projectedRef.Name},
		forbiddenSecretGets: 1,
	}
	reconciler := newClaimTestReconciler(t, scheme, baseClient, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
		runtimeCfg.Reader = reader
	})

	if _, err := reconciler.Reconcile(context.Background(), client.ObjectKeyFromObject(claim), testr.New(t)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if reader.forbiddenSecretGets != 1 {
		t.Fatalf("projected Secret get attempts = %d, want none", 1-reader.forbiddenSecretGets)
	}

	current := &corev1.Secret{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Namespace: "payments", Name: projectedRef.Name}, current); !apierrors.IsNotFound(err) {
		t.Fatalf("Get projected Secret error = %v, want not found after finalization cleanup", err)
	}
	updated := &openbaov1alpha1.OpenBaoClusterClaim{}
	err := baseClient.Get(context.Background(), client.ObjectKeyFromObject(claim), updated)
	if err == nil && hasFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer) {
		t.Fatalf("claim finalizers = %v, want claim finalizer removed", updated.Finalizers)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatalf("Get claim error = %v", err)
	}
}

func runDeleteClaimReconcileTest(t *testing.T, tt deleteClaimTestCase) {
	t.Helper()

	statusObjects := append([]client.Object{tt.claim}, tt.objects...)
	scheme, builder := newClaimTestClientBuilder(t, statusObjects...)
	objects := append([]client.Object{tt.claim.DeepCopy()}, cloneObjects(tt.objects)...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = tt.serviceClaims
	})
	if _, err := reconciler.Reconcile(context.Background(), client.ObjectKeyFromObject(tt.claim), testr.New(t)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(tt.claim), updated); err != nil {
		if !tt.wantFinalizer && apierrors.IsNotFound(err) {
			return
		}
		t.Fatalf("Get() error = %v", err)
	}
	if hasFinalizer(updated.Finalizers, openbaov1alpha1.OpenBaoClusterClaimFinalizer) != tt.wantFinalizer {
		t.Fatalf("claim finalizers = %v, want finalizer present=%t", updated.Finalizers, tt.wantFinalizer)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseDeleting)
	}

	assertDeleteClaimLocalState(t, c, tt)
}

func assertDeleteClaimLocalState(t *testing.T, c client.Client, tt deleteClaimTestCase) {
	t.Helper()

	if !tt.serviceClaims {
		return
	}

	local := &openbaov1alpha1.OpenBaoCluster{}
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "payments-bao"}, local)
	if tt.wantLocalAbsent {
		if !apierrors.IsNotFound(err) {
			t.Fatalf("expected local OpenBaoCluster to be absent, got err=%v", err)
		}
	} else {
		if err != nil {
			t.Fatalf("Get local OpenBaoCluster() error = %v", err)
		}
		if tt.wantLocalDelete && local.DeletionTimestamp.IsZero() {
			t.Fatalf("expected local OpenBaoCluster deletion timestamp to be set")
		}
		if tt.wantLocalKeep && !local.DeletionTimestamp.IsZero() {
			t.Fatalf("expected local OpenBaoCluster to remain undeleted")
		}
		if local.Spec.SelfInit != nil {
			assertProjectedBootstrapRefsExist(t, c, local)
		}
	}
	if tt.wantProjectedAbs {
		configMaps := &corev1.ConfigMapList{}
		if err := c.List(context.Background(), configMaps, client.InNamespace("payments"), client.MatchingLabels(bootstrapProjectionLabels(tt.claim))); err != nil {
			t.Fatalf("List projected ConfigMaps() error = %v", err)
		}
		if len(configMaps.Items) != 0 {
			t.Fatalf("projected ConfigMaps = %#v, want none", configMaps.Items)
		}
		secrets := &corev1.SecretList{}
		if err := c.List(context.Background(), secrets, client.InNamespace("payments"), client.MatchingLabels(bootstrapProjectionLabels(tt.claim))); err != nil {
			t.Fatalf("List projected Secrets() error = %v", err)
		}
		if len(secrets.Items) != 0 {
			t.Fatalf("projected Secrets = %#v, want none", secrets.Items)
		}
	}
}

func validClaim() *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-bao", Namespace: "payments"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
}

func validCatalogObjects() []client.Object {
	return []client.Object{
		validServiceProfile(),
		validBootstrapProfile(),
		validExposureClass(),
		validBackupProfile(),
		validEntrypoint(),
	}
}
