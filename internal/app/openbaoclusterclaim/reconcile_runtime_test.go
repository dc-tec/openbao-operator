package openbaoclusterclaim

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/connectionpublishing"
)

func TestReconcileSameClusterClaimProjectsConfiguredAPIServerEndpointIPs(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+2)
	objects = append(objects, claim.DeepCopy(), validTenant())
	objects = append(objects, catalogObjects...)
	scheme, builder := newClaimTestClientBuilder(t, claim)
	c := builder.WithStatusSubresource(claim).WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
		runtimeCfg.SameClusterNetwork = SameClusterNetworkConfig{
			APIServerEndpointIPs: []string{"172.29.0.2"},
		}
	})

	reconcileClaimOnce(t, c, reconciler, claim)

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "payments", Name: "payments-bao"}, cluster); err != nil {
		t.Fatalf("Get local cluster error = %v", err)
	}
	if cluster.Spec.Network == nil {
		t.Fatal("cluster spec network = nil, want projected api server endpoint IPs")
	}
	if len(cluster.Spec.Network.APIServerEndpointIPs) != 1 || cluster.Spec.Network.APIServerEndpointIPs[0] != "172.29.0.2" {
		t.Fatalf("apiServerEndpointIPs = %v, want 172.29.0.2", cluster.Spec.Network.APIServerEndpointIPs)
	}
}

func TestReconcileSameClusterClaimSurfacesLocalAdmissionFailureOnClaimStatus(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+2)
	objects = append(objects, claim.DeepCopy(), validTenant())
	objects = append(objects, catalogObjects...)
	baseClient := builder.WithObjects(objects...).Build()

	createErr := apierrors.NewForbidden(
		schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"},
		claim.Name,
		fmt.Errorf("ValidatingAdmissionPolicy 'openbao-operator-openbao-validate-openbaocluster' denied request: Backup endpoint must use HTTPS or S3 scheme in Hardened profile."),
	)
	c := &interceptCreateClient{
		Client:           baseClient,
		openBaoCreateErr: createErr,
	}

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, baseClient, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseFailed)
	}
	condition := meta.FindStatusCondition(updated.Status.Conditions, conditionTypeMaterialization)
	if condition == nil {
		t.Fatal("materialization condition = nil, want populated failure")
	}
	if condition.Status != metav1.ConditionFalse {
		t.Fatalf("materialization condition status = %q, want %q", condition.Status, metav1.ConditionFalse)
	}
	if condition.Reason != string(openbaov1alpha1.ReasonInvalid) {
		t.Fatalf("materialization condition reason = %q, want %q", condition.Reason, openbaov1alpha1.ReasonInvalid)
	}
	if !strings.Contains(condition.Message, "Backup endpoint must use HTTPS or S3 scheme in Hardened profile.") {
		t.Fatalf("materialization condition message = %q, want hardened backup admission failure", condition.Message)
	}
}

func TestReconcileSameClusterClaimRecoversWhenLocalCASecretReadIsInitiallyForbidden(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}

	localCluster := &openbaov1alpha1.OpenBaoCluster{
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
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	baseClient := builder.WithObjects(objects...).Build()

	caKey := client.ObjectKey{Namespace: "payments", Name: connectionpublishing.LocalCASecretName("payments-bao")}
	c := &interceptGetClient{
		Client:              baseClient,
		forbiddenSecretKey:  caKey,
		forbiddenSecretGets: 1,
	}

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	result, updated := reconcileClaimOnce(t, baseClient, reconciler, claim)
	if result.RequeueAfter != constants.RequeueShort {
		t.Fatalf("first Reconcile() requeueAfter = %s, want %s", result.RequeueAfter, constants.RequeueShort)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning {
		t.Fatalf("first phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseProvisioning)
	}
	connection := meta.FindStatusCondition(updated.Status.Conditions, conditionTypeConnectionPublished)
	if connection == nil || connection.Status != metav1.ConditionFalse || connection.Reason != string(openbaov1alpha1.ReasonPending) {
		t.Fatalf("connection condition after first reconcile = %#v, want pending", connection)
	}
	if updated.Status.Connection.Endpoint != "" {
		t.Fatalf("connection endpoint after first reconcile = %q, want empty", updated.Status.Connection.Endpoint)
	}
	secret := &corev1.Secret{}
	if err := baseClient.Get(context.Background(), client.ObjectKey{Namespace: claim.Namespace, Name: connectionpublishing.SecretName(claim.Name)}, secret); !apierrors.IsNotFound(err) {
		t.Fatalf("claim connection Secret get after first reconcile error = %v, want not found", err)
	}

	result, updated = reconcileClaimOnce(t, baseClient, reconciler, claim)
	if result.RequeueAfter != 0 {
		t.Fatalf("second Reconcile() requeueAfter = %s, want 0", result.RequeueAfter)
	}
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseReady {
		t.Fatalf("second phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseReady)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeServiceAvailable, metav1.ConditionTrue, string(openbaov1alpha1.ReasonReady))
	assertCondition(t, updated.Status.Conditions, conditionTypeMaintenanceActive, metav1.ConditionFalse, reasonIdle)
	if updated.Status.Summary != nil {
		t.Fatalf("claim summary = %#v, want nil once service is ready", updated.Status.Summary)
	}
	if updated.Status.Connection.Endpoint != validSameClusterEndpoint() {
		t.Fatalf("connection endpoint after second reconcile = %q, want %q", updated.Status.Connection.Endpoint, validSameClusterEndpoint())
	}
	if err := baseClient.Get(context.Background(), client.ObjectKey{Namespace: claim.Namespace, Name: connectionpublishing.SecretName(claim.Name)}, secret); err != nil {
		t.Fatalf("claim connection Secret get after second reconcile error = %v", err)
	}
}

func TestReconcileSameClusterClaimFailsWhenConnectionSecretNameIsOccupiedByForeignSecret(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}

	localCluster := &openbaov1alpha1.OpenBaoCluster{
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
	}
	conflictingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: claim.Namespace,
			Name:      connectionpublishing.SecretName(claim.Name),
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "manual"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"endpoint": []byte("https://manual.example.internal"),
			"ca.crt":   []byte("manual-ca"),
		},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+6)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
		conflictingSecret,
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseFailed)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeConnectionPublished, metav1.ConditionFalse, string(openbaov1alpha1.ReasonInvalid))
	if !strings.Contains(findCondition(updated.Status.Conditions, conditionTypeConnectionPublished).Message, "already exists and is not owned") {
		t.Fatalf("connection condition message = %q, want custody failure", findCondition(updated.Status.Conditions, conditionTypeConnectionPublished).Message)
	}
	if updated.Status.Summary == nil {
		t.Fatal("claim summary = nil, want connection custody failure summary")
	}
	if updated.Status.Summary.Severity != openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError {
		t.Fatalf("claim summary severity = %q, want %q", updated.Status.Summary.Severity, openbaov1alpha1.OpenBaoClusterClaimStatusSeverityError)
	}
	if updated.Status.Summary.Reason != string(openbaov1alpha1.ReasonInvalid) {
		t.Fatalf("claim summary reason = %q, want %q", updated.Status.Summary.Reason, openbaov1alpha1.ReasonInvalid)
	}
	if updated.Status.Summary.SourceRef == nil || updated.Status.Summary.SourceRef.Kind != "Secret" || updated.Status.Summary.SourceRef.Name != connectionpublishing.SecretName(claim.Name) {
		t.Fatalf("claim summary sourceRef = %#v, want claim connection Secret", updated.Status.Summary.SourceRef)
	}
	if updated.Status.Connection.Endpoint != "" || updated.Status.Connection.SecretRef != nil || updated.Status.Connection.CABundleRef != nil {
		t.Fatalf("published connection status = %#v, want empty after custody conflict", updated.Status.Connection)
	}

	current := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conflictingSecret), current); err != nil {
		t.Fatalf("Get conflicting secret error = %v", err)
	}
	if string(current.Data["endpoint"]) != "https://manual.example.internal" {
		t.Fatalf("conflicting secret endpoint = %q, want unchanged manual endpoint", string(current.Data["endpoint"]))
	}
}

func TestReconcileSameClusterClaimKeepsPublishedConnectionObservedAtStableAcrossNoopReconcile(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}

	claim := validClaim()
	claim.Status.Materialization = openbaov1alpha1.OpenBaoClusterClaimMaterializationStatus{
		Mode: openbaov1alpha1.OpenBaoClusterClaimMaterializationModeSameCluster,
		LocalRef: &openbaov1alpha1.NamespacedReference{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	}

	localCluster := &openbaov1alpha1.OpenBaoCluster{
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
	}

	builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+5)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		localCluster,
		validSameClusterPublicService(),
		validSameClusterCASecret(),
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithObjects(objects...).Build()

	reconciler := NewReconciler(Runtime{
		Client:              c,
		Scheme:              scheme,
		EnableServiceClaims: true,
	})

	key := client.ObjectKeyFromObject(claim)
	if _, err := reconciler.Reconcile(context.Background(), key, testr.New(t)); err != nil {
		t.Fatalf("first Reconcile() error = %v", err)
	}

	updated := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := c.Get(context.Background(), key, updated); err != nil {
		t.Fatalf("Get claim after first reconcile error = %v", err)
	}
	if updated.Status.Connection.ObservedAt == nil {
		t.Fatal("first connection observedAt = nil, want publish timestamp")
	}
	firstObservedAt := updated.Status.Connection.ObservedAt.DeepCopy()

	if _, err := reconciler.Reconcile(context.Background(), key, testr.New(t)); err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if err := c.Get(context.Background(), key, updated); err != nil {
		t.Fatalf("Get claim after second reconcile error = %v", err)
	}
	if updated.Status.Connection.ObservedAt == nil {
		t.Fatal("second connection observedAt = nil, want preserved publish timestamp")
	}
	if !updated.Status.Connection.ObservedAt.Equal(firstObservedAt) {
		t.Fatalf("second connection observedAt = %v, want preserved %v on no-op republish", updated.Status.Connection.ObservedAt, firstObservedAt)
	}
}

func TestReconcileSameClusterClaimFailsWhenProjectedBootstrapSecretNameIsOccupiedByForeignSecret(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
	projectedRef := validSameClusterSecretConfigRefAppliedStatus().RenderedDependencies.BootstrapProjectionRefs[0]
	conflictingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      projectedRef.Name,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "manual"},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"config.json": []byte(`{"issuer":"https://manual.example.internal"}`)},
	}

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterSecretConfigRefCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+4)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		validSameClusterAuthMethodSecret(),
		conflictingSecret,
	)
	objects = append(objects, catalogObjects...)
	c := builder.WithStatusSubresource(claim).WithObjects(objects...).Build()

	reconciler := newClaimTestReconciler(t, scheme, c, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
	})

	_, updated := reconcileClaimOnce(t, c, reconciler, claim)
	if updated.Status.Phase != openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, openbaov1alpha1.OpenBaoClusterClaimPhaseFailed)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeMaterialization, metav1.ConditionFalse, string(openbaov1alpha1.ReasonInvalid))
	materialization := findCondition(updated.Status.Conditions, conditionTypeMaterialization)
	if materialization == nil || !strings.Contains(materialization.Message, "bootstrap projection is blocked") {
		t.Fatalf("materialization condition = %#v, want bootstrap custody failure", materialization)
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{}
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "payments", Name: "payments-bao"}, cluster)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("Get local cluster error = %v, want not found after bootstrap projection conflict", err)
	}

	current := &corev1.Secret{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conflictingSecret), current); err != nil {
		t.Fatalf("Get conflicting projected secret error = %v", err)
	}
	if string(current.Data["config.json"]) != `{"issuer":"https://manual.example.internal"}` {
		t.Fatalf("conflicting projected secret data = %q, want unchanged manual payload", string(current.Data["config.json"]))
	}
}

func TestReconcileSameClusterClaimCreatesProjectedBootstrapSecretWithoutTenantSecretGet(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-configref-v1"}
	projectedRef := validSameClusterSecretConfigRefAppliedStatus().RenderedDependencies.BootstrapProjectionRefs[0]

	scheme, builder := newClaimTestClientBuilder(t, claim)
	catalogObjects := cloneObjects(sameClusterSecretConfigRefCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+3)
	objects = append(objects,
		claim.DeepCopy(),
		validTenant(),
		validSameClusterAuthMethodSecret(),
	)
	objects = append(objects, catalogObjects...)
	baseClient := builder.WithStatusSubresource(claim).WithObjects(objects...).Build()
	reader := &interceptGetClient{
		Client:              baseClient,
		forbiddenSecretKey:  types.NamespacedName{Namespace: "payments", Name: projectedRef.Name},
		forbiddenSecretGets: 1,
	}

	reconciler := newClaimTestReconciler(t, scheme, baseClient, func(runtimeCfg *Runtime) {
		runtimeCfg.EnableServiceClaims = true
		runtimeCfg.Reader = reader
	})

	_, updated := reconcileClaimOnce(t, baseClient, reconciler, claim)
	if updated.Status.Phase == openbaov1alpha1.OpenBaoClusterClaimPhaseFailed {
		t.Fatalf("Phase = %q, want non-failed claim after projected Secret create", updated.Status.Phase)
	}
	assertCondition(t, updated.Status.Conditions, conditionTypeMaterialization, metav1.ConditionTrue, string(openbaov1alpha1.ReasonAccepted))

	projected := &corev1.Secret{}
	if err := baseClient.Get(context.Background(), types.NamespacedName{Namespace: "payments", Name: projectedRef.Name}, projected); err != nil {
		t.Fatalf("Get projected secret error = %v", err)
	}
	if !bootstrapProjectionObjectOwnedByClaim(projected, claim) {
		t.Fatalf("projected secret labels = %#v, want claim-owned bootstrap projection labels", projected.Labels)
	}
}

func TestReconcileSameClusterClaimDoesNotMaskNonAdmissionLocalCreateErrors(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(corev1) error = %v", err)
	}

	claim := validClaim()
	builder := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(claim)
	catalogObjects := cloneObjects(sameClusterCatalogObjects())
	objects := make([]client.Object, 0, len(catalogObjects)+2)
	objects = append(objects, claim.DeepCopy(), validTenant())
	objects = append(objects, catalogObjects...)
	baseClient := builder.WithObjects(objects...).Build()

	createErr := apierrors.NewForbidden(
		schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"},
		claim.Name,
		fmt.Errorf("User \"system:serviceaccount:openbao-operator-system:openbao-operator-controller\" cannot create resource \"openbaoclusters\" in API group \"openbao.org\" in the namespace \"payments\""),
	)
	c := &interceptCreateClient{
		Client:           baseClient,
		openBaoCreateErr: createErr,
	}

	reconciler := NewReconciler(Runtime{
		Client:              c,
		Scheme:              scheme,
		EnableServiceClaims: true,
	})

	if _, err := reconciler.Reconcile(context.Background(), client.ObjectKeyFromObject(claim), testr.New(t)); err == nil {
		t.Fatal("Reconcile() error = nil, want RBAC-style create error")
	}
}

func TestSameClusterSourceLoadResultReturnsPendingForForbidden(t *testing.T) {
	t.Parallel()

	err := apierrors.NewForbidden(
		schema.GroupResource{Group: "", Resource: "secrets"},
		"kubernetes-auth-default",
		fmt.Errorf("User %q cannot get resource %q in namespace %q", "system:serviceaccount:openbao-operator-system:openbao-operator-controller", "secrets", "payments"),
	)

	got := sameClusterSourceLoadResult(err, "bootstrap auth method config", "Secret")
	if got.Valid {
		t.Fatalf("sameClusterSourceLoadResult() = %#v, want pending", got)
	}
	if got.Reason != openbaov1alpha1.ReasonPending {
		t.Fatalf("sameClusterSourceLoadResult() reason = %q, want %q", got.Reason, openbaov1alpha1.ReasonPending)
	}
	if !strings.Contains(got.Message, "tenant secret RBAC to converge") {
		t.Fatalf("sameClusterSourceLoadResult() message = %q, want RBAC convergence warning", got.Message)
	}
}

func TestEnsureLocalClusterFailsCreateRaceWhenTargetIsClaimedByAnotherOwner(t *testing.T) {
	t.Parallel()

	claim := validClaim()
	liveCluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "payments-bao",
			Labels: map[string]string{
				constants.LabelOpenBaoOwnershipMode: constants.LabelValueOpenBaoOwnershipDirectManaged,
			},
		},
	}

	_, builder := newClaimTestClientBuilder(t, claim)
	base := builder.WithObjects(liveCluster).Build()
	racingClient := &interceptCreateClient{
		Client:               base,
		openBaoAlreadyExists: true,
	}
	racingReader := &interceptOpenBaoClusterReader{
		Client:              base,
		notFoundClusterKey:  client.ObjectKey{Namespace: "payments", Name: "payments-bao"},
		notFoundClusterGets: 1,
	}
	reconciler := runtimeReconciler{
		client:              racingClient,
		reader:              racingReader,
		enableServiceClaims: true,
	}

	_, _, err := reconciler.ensureLocalCluster(context.Background(), claim, &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "payments",
			Name:      "payments-bao",
		},
	})
	if err == nil {
		t.Fatal("ensureLocalCluster() error = nil, want ownership conflict")
	}
	classified, ok := classifyLocalClusterReconcileError(err)
	if !ok {
		t.Fatalf("ensureLocalCluster() error = %v, want classified ownership conflict", err)
	}
	if classified.Reason != openbaov1alpha1.ReasonInvalid {
		t.Fatalf("classified reason = %q, want %q", classified.Reason, openbaov1alpha1.ReasonInvalid)
	}
	if !strings.Contains(classified.Message, "directly-managed OpenBaoCluster already exists") {
		t.Fatalf("classified message = %q, want direct-managed ownership conflict", classified.Message)
	}
}

func assertPublishedConnection(
	t *testing.T,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	secret *corev1.Secret,
	err error,
	wantEndpoint string,
) {
	t.Helper()

	if err != nil {
		t.Fatalf("Get secret() error = %v", err)
	}
	if claim.Status.Connection.Endpoint != wantEndpoint {
		t.Fatalf("connection endpoint = %q, want %q", claim.Status.Connection.Endpoint, wantEndpoint)
	}
	if claim.Status.Connection.SecretRef == nil || claim.Status.Connection.SecretRef.Name != connectionpublishing.SecretName(claim.Name) {
		t.Fatalf("connection secret ref = %#v, want %q", claim.Status.Connection.SecretRef, connectionpublishing.SecretName(claim.Name))
	}
	if string(secret.Data["endpoint"]) != wantEndpoint {
		t.Fatalf("secret endpoint = %q, want %q", string(secret.Data["endpoint"]), wantEndpoint)
	}
	if claim.Status.Connection.CABundleRef == nil || claim.Status.Connection.CABundleRef.Name != connectionpublishing.SecretName(claim.Name) || claim.Status.Connection.CABundleRef.Kind != testSecretKind {
		t.Fatalf("connection caBundleRef = %#v, want Secret/%q", claim.Status.Connection.CABundleRef, connectionpublishing.SecretName(claim.Name))
	}
	if claim.Status.Connection.ObservedAt == nil {
		t.Fatal("connection observedAt should be populated when the claim-facing contract is published")
	}
	if string(secret.Data["ca.crt"]) == "" {
		t.Fatal("secret ca.crt should be populated")
	}
}

type interceptCreateClient struct {
	client.Client
	openBaoCreateErr     error
	openBaoAlreadyExists bool
}

func (c *interceptCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*openbaov1alpha1.OpenBaoCluster); ok {
		if c.openBaoCreateErr != nil {
			return c.openBaoCreateErr
		}
		if c.openBaoAlreadyExists {
			return apierrors.NewAlreadyExists(schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"}, obj.GetName())
		}
	}
	return c.Client.Create(ctx, obj, opts...)
}

type interceptGetClient struct {
	client.Client
	forbiddenSecretKey  client.ObjectKey
	forbiddenSecretGets int
}

func (c *interceptGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*corev1.Secret); ok && key == c.forbiddenSecretKey && c.forbiddenSecretGets > 0 {
		c.forbiddenSecretGets--
		return apierrors.NewForbidden(
			schema.GroupResource{Group: "", Resource: "secrets"},
			key.Name,
			fmt.Errorf("User %q cannot get resource %q in namespace %q", "system:serviceaccount:openbao-operator-system:openbao-operator-controller", "secrets", key.Namespace),
		)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

type interceptOpenBaoClusterReader struct {
	client.Client
	notFoundClusterKey  client.ObjectKey
	notFoundClusterGets int
}

func (c *interceptOpenBaoClusterReader) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*openbaov1alpha1.OpenBaoCluster); ok && key == c.notFoundClusterKey && c.notFoundClusterGets > 0 {
		c.notFoundClusterGets--
		return apierrors.NewNotFound(schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"}, key.Name)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func cloneObjects(objs []client.Object) []client.Object {
	clones := make([]client.Object, 0, len(objs))
	for _, obj := range objs {
		clones = append(clones, obj.DeepCopyObject().(client.Object))
	}
	return clones
}

func assertCondition(t *testing.T, conditions []metav1.Condition, conditionType string, wantStatus metav1.ConditionStatus, wantReason string) {
	t.Helper()

	condition := findCondition(conditions, conditionType)
	if condition == nil {
		t.Fatalf("condition %q not found", conditionType)
	}
	if condition.Status != wantStatus {
		t.Fatalf("condition %q status = %q, want %q", conditionType, condition.Status, wantStatus)
	}
	if wantReason != "" && condition.Reason != wantReason {
		t.Fatalf("condition %q reason = %q, want %q", conditionType, condition.Reason, wantReason)
	}
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func derefAppliedStatus(status *openbaov1alpha1.OpenBaoClusterClaimAppliedStatus) openbaov1alpha1.OpenBaoClusterClaimAppliedStatus {
	if status == nil {
		return openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{}
	}

	return *status
}
