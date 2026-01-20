package infra

import (
	"context"
	"errors"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func newTestClient(t *testing.T) client.Client {
	t.Helper()

	// Create the Kubernetes service that NetworkPolicy detection requires
	kubernetesService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubernetes",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.43.0.1", // Used to derive the kubernetes Service IP CIDR (10.43.0.1/32)
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}

	return fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(kubernetesService).
		Build()
}

// newTestStatefulSetSpec creates a minimal StatefulSetSpec for testing.
func newTestStatefulSetSpec(cluster *openbaov1alpha1.OpenBaoCluster) StatefulSetSpec {
	return StatefulSetSpec{
		Name:               cluster.Name,
		Revision:           "",
		Image:              cluster.Spec.Image,
		InitContainerImage: "",
		Replicas:           cluster.Spec.Replicas,
		ConfigHash:         "",
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}
}

func newMinimalCluster(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao/openbao-config-init:latest",
			},
		},
	}
}

// createTLSSecretForTest creates a minimal TLS server secret for testing.
// This is needed because ensureStatefulSet now checks for prerequisite resources.
func createTLSSecretForTest(t *testing.T, k8sClient client.Client, cluster *openbaov1alpha1.OpenBaoCluster) {
	t.Helper()
	secretName := tlsServerSecretName(cluster)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
			"ca.crt":  []byte("test-ca"),
		},
	}
	if err := k8sClient.Create(context.Background(), secret); err != nil {
		t.Fatalf("failed to create TLS secret for test: %v", err)
	}
}

// Integration tests that verify the full Reconcile and Cleanup flows

func TestReconcileCreatesAllResources(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

	cluster := newMinimalCluster("infra-full", "default")
	cluster.Status.Initialized = true

	// Create TLS secret before Reconcile, as ensureStatefulSet now checks for prerequisites
	createTLSSecretForTest(t, k8sClient, cluster)

	ctx := context.Background()

	spec := newTestStatefulSetSpec(cluster)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify all resources are created
	resources := []struct {
		name     string
		getFunc  func() error
		resource string
	}{
		{
			name: "ConfigMap",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      configMapName(cluster),
				}, &corev1.ConfigMap{})
			},
			resource: "ConfigMap",
		},
		{
			name: "Headless Service",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      headlessServiceName(cluster),
				}, &corev1.Service{})
			},
			resource: "Service",
		},
		{
			name: "StatefulSet",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      statefulSetName(cluster),
				}, &appsv1.StatefulSet{})
			},
			resource: "StatefulSet",
		},
		{
			name: "ServiceAccount",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      serviceAccountName(cluster),
				}, &corev1.ServiceAccount{})
			},
			resource: "ServiceAccount",
		},
	}

	for _, r := range resources {
		if err := r.getFunc(); err != nil {
			t.Errorf("expected %s to exist: %v", r.resource, err)
		}
	}
}

func TestReconcile_ACMEMode_CreatesChallengeService(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

	cluster := newMinimalCluster("infra-acme", "default")
	cluster.Status.Initialized = true
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://example.invalid/acme",
		Domains:      []string{"infra-acme-acme.default.svc"},
	}

	ctx := context.Background()
	spec := newTestStatefulSetSpec(cluster)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	svc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      acmeServiceName(cluster),
	}, svc); err != nil {
		t.Fatalf("expected ACME challenge Service to exist: %v", err)
	}
	if !svc.Spec.PublishNotReadyAddresses {
		t.Fatalf("expected ACME challenge Service to set publishNotReadyAddresses=true")
	}

	gotPorts := map[int32]bool{}
	for _, p := range svc.Spec.Ports {
		gotPorts[p.Port] = true
	}
	if !gotPorts[80] || !gotPorts[443] {
		t.Fatalf("expected ACME challenge Service to expose ports 80 and 443, got %v", svc.Spec.Ports)
	}
}

func TestReconcile_ACMEMode_PreflightRejectsGatewayTermination(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

	cluster := newMinimalCluster("acme-gw", "default")
	cluster.Status.Initialized = true
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://example.invalid/acme",
		Domains:      []string{"acme-gw-acme.default.svc"},
	}
	cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
		Enabled:        true,
		TLSPassthrough: false,
		GatewayRef: openbaov1alpha1.GatewayReference{
			Name: "gateway",
		},
		Hostname: "bao.example.com",
	}

	ctx := context.Background()
	spec := newTestStatefulSetSpec(cluster)
	err := manager.Reconcile(ctx, logr.Discard(), cluster, spec)
	if err == nil {
		t.Fatalf("expected Reconcile() to fail preflight for ACME + Gateway termination")
	}
	if !errors.Is(err, ErrACMEGatewayNotConfiguredForPassthrough) {
		t.Fatalf("expected ErrACMEGatewayNotConfiguredForPassthrough, got %T: %v", err, err)
	}
}

func TestReconcile_ACMEMode_PreflightRejectsUnresolvableDomainForPrivateCA(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

	cluster := newMinimalCluster("acme-dns", "default")
	cluster.Status.Initialized = true
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://example.invalid/acme",
		Domains:      []string{"no-such-host.openbao.invalid"},
	}
	cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
		ACMECARoot: "/etc/bao/seal-creds/ca.crt",
	}

	ctx := context.Background()
	spec := newTestStatefulSetSpec(cluster)
	err := manager.Reconcile(ctx, logr.Discard(), cluster, spec)
	if err == nil {
		t.Fatalf("expected Reconcile() to fail preflight for unresolvable ACME domain")
	}
	if !errors.Is(err, ErrACMEDomainNotResolvable) {
		t.Fatalf("expected ErrACMEDomainNotResolvable, got %T: %v", err, err)
	}
}

func TestCleanupRespectsDeletionPolicyForPVCs(t *testing.T) {
	tests := []struct {
		name           string
		policy         openbaov1alpha1.DeletionPolicy
		expectPVCExist bool
	}{
		{
			name:           "retain keeps PVCs",
			policy:         openbaov1alpha1.DeletionPolicyRetain,
			expectPVCExist: true,
		},
		{
			name:           "deletepvcs deletes PVCs",
			policy:         openbaov1alpha1.DeletionPolicyDeletePVCs,
			expectPVCExist: false,
		},
		{
			name:           "deleteall deletes PVCs",
			policy:         openbaov1alpha1.DeletionPolicyDeleteAll,
			expectPVCExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			k8sClient := newTestClient(t)
			manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

			cluster := newMinimalCluster("infra-delete", "default")
			createTLSSecretForTest(t, k8sClient, cluster)

			ctx := context.Background()

			spec := newTestStatefulSetSpec(cluster)
			if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-infra-delete-0",
					Namespace: cluster.Namespace,
					Labels: map[string]string{
						constants.LabelOpenBaoCluster: cluster.Name,
					},
				},
			}
			if err := k8sClient.Create(ctx, pvc); err != nil {
				t.Fatalf("failed to seed PVC: %v", err)
			}

			if err := manager.Cleanup(ctx, logr.Discard(), cluster, tt.policy); err != nil {
				t.Fatalf("Cleanup() error = %v", err)
			}

			err := k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      pvc.Name,
			}, &corev1.PersistentVolumeClaim{})

			if tt.expectPVCExist {
				if err != nil {
					t.Fatalf("expected PVC to exist, got error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected PVC to be deleted")
				}
				if !apierrors.IsNotFound(err) {
					t.Fatalf("expected not found error for PVC, got: %v", err)
				}
			}
		})
	}
}

// TestCleanupReliesOnGarbageCollection verifies that the Cleanup method delegates
// resource deletion to Kubernetes GC. Since all resources have OwnerReferences,
// they will be automatically deleted when the OpenBaoCluster is deleted.
// Cleanup() only handles PVC deletion based on policy.
func TestCleanupReliesOnGarbageCollection(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")

	// Create the cluster in the fake client so it has a UID for OwnerReference
	cluster := newMinimalCluster("infra-gc-cleanup", "default")
	cluster.Status.Initialized = true
	cluster.APIVersion = apiVersion
	cluster.Kind = kind

	ctx := context.Background()

	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}

	// Re-fetch to get the UID
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, cluster); err != nil {
		t.Fatalf("failed to get cluster: %v", err)
	}

	createTLSSecretForTest(t, k8sClient, cluster)

	// Create all resources
	spec := newTestStatefulSetSpec(cluster)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Cleanup should succeed (it now only handles PVCs)
	if err := manager.Cleanup(ctx, logr.Discard(), cluster, openbaov1alpha1.DeletionPolicyRetain); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	// Verify resources STILL EXIST after Cleanup (GC handles deletion, not Cleanup())
	// These resources have OwnerReferences setting and will be deleted by K8s GC
	// when the OpenBaoCluster is deleted.
	resources := []struct {
		name     string
		getFunc  func() error
		resource string
	}{
		{
			name: "ConfigMap",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      configMapName(cluster),
				}, &corev1.ConfigMap{})
			},
			resource: "ConfigMap",
		},
		{
			name: "Headless Service",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      headlessServiceName(cluster),
				}, &corev1.Service{})
			},
			resource: "Service",
		},
		{
			name: "StatefulSet",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      statefulSetName(cluster),
				}, &appsv1.StatefulSet{})
			},
			resource: "StatefulSet",
		},
		{
			name: "ServiceAccount",
			getFunc: func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: cluster.Namespace,
					Name:      serviceAccountName(cluster),
				}, &corev1.ServiceAccount{})
			},
			resource: "ServiceAccount",
		},
	}

	for _, r := range resources {
		err := r.getFunc()
		if err != nil {
			t.Errorf("expected %s to still exist after Cleanup() (GC handles deletion): %v", r.resource, err)
		}
	}

	// Verify StatefulSet has owner reference (confirming GC will clean it up)
	sts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      statefulSetName(cluster),
	}, sts); err != nil {
		t.Fatalf("failed to get StatefulSet: %v", err)
	}

	foundOwnerRef := false
	for _, ref := range sts.OwnerReferences {
		if ref.Kind == kind && ref.Name == cluster.Name {
			foundOwnerRef = true
			break
		}
	}
	if !foundOwnerRef {
		t.Error("expected StatefulSet to have OwnerReference to OpenBaoCluster for GC")
	}
}

// Multi-Tenancy Tests
// These tests verify that resource naming and scoping satisfy multi-tenancy requirements.

// TestMultiTenancyResourceNamingUniqueness verifies that resources created for different
// OpenBaoClusters are uniquely named using the cluster name prefix, preventing cross-tenant
// sharing of Secrets, ConfigMaps, StatefulSets, and Services (FR-MT-05).
func TestMultiTenancyResourceNamingUniqueness(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")
	ctx := context.Background()

	// Create two clusters with different names in the same namespace
	cluster1 := newMinimalCluster("tenant-alpha", "shared-ns")
	cluster1.Status.Initialized = true
	cluster2 := newMinimalCluster("tenant-beta", "shared-ns")
	cluster2.Status.Initialized = true
	createTLSSecretForTest(t, k8sClient, cluster1)
	createTLSSecretForTest(t, k8sClient, cluster2)

	// Reconcile both clusters
	spec1 := newTestStatefulSetSpec(cluster1)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster1, spec1); err != nil {
		t.Fatalf("Reconcile() cluster1 error = %v", err)
	}
	spec2 := newTestStatefulSetSpec(cluster2)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster2, spec2); err != nil {
		t.Fatalf("Reconcile() cluster2 error = %v", err)
	}

	// Verify each cluster has uniquely named resources
	resourceTests := []struct {
		getResource func(string) error
		suffix      string
	}{
		{
			suffix: "-config (ConfigMap)",
			getResource: func(name string) error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: "shared-ns",
					Name:      name + constants.SuffixConfigMap,
				}, &corev1.ConfigMap{})
			},
		},
		{
			suffix: "-unseal-key (Secret)",
			getResource: func(name string) error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: "shared-ns",
					Name:      name + constants.SuffixUnsealKey,
				}, &corev1.Secret{})
			},
		},
		{
			suffix: " (StatefulSet)",
			getResource: func(name string) error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: "shared-ns",
					Name:      name,
				}, &appsv1.StatefulSet{})
			},
		},
		{
			suffix: " (Headless Service)",
			getResource: func(name string) error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Namespace: "shared-ns",
					Name:      name,
				}, &corev1.Service{})
			},
		},
	}

	for _, rt := range resourceTests {
		// Verify cluster1 resource exists
		if err := rt.getResource(cluster1.Name); err != nil {
			t.Errorf("expected resource %s%s for cluster1 to exist: %v", cluster1.Name, rt.suffix, err)
		}
		// Verify cluster2 resource exists
		if err := rt.getResource(cluster2.Name); err != nil {
			t.Errorf("expected resource %s%s for cluster2 to exist: %v", cluster2.Name, rt.suffix, err)
		}
	}
}

// TestMultiTenancyNamespaceIsolation verifies that resources are created in the correct
// namespace and that clusters in different namespaces are isolated (FR-MT-01, FR-MT-02).
func TestMultiTenancyNamespaceIsolation(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")
	ctx := context.Background()

	// Create two clusters with the same name but in different namespaces
	cluster1 := newMinimalCluster("same-cluster-name", "namespace-a")
	cluster1.Status.Initialized = true
	cluster2 := newMinimalCluster("same-cluster-name", "namespace-b")
	cluster2.Status.Initialized = true
	createTLSSecretForTest(t, k8sClient, cluster1)
	createTLSSecretForTest(t, k8sClient, cluster2)

	// Reconcile both clusters
	spec1 := newTestStatefulSetSpec(cluster1)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster1, spec1); err != nil {
		t.Fatalf("Reconcile() cluster1 error = %v", err)
	}
	spec2 := newTestStatefulSetSpec(cluster2)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster2, spec2); err != nil {
		t.Fatalf("Reconcile() cluster2 error = %v", err)
	}

	// Verify resources exist in their respective namespaces
	sts1 := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "namespace-a",
		Name:      "same-cluster-name",
	}, sts1); err != nil {
		t.Errorf("expected StatefulSet to exist in namespace-a: %v", err)
	}

	sts2 := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "namespace-b",
		Name:      "same-cluster-name",
	}, sts2); err != nil {
		t.Errorf("expected StatefulSet to exist in namespace-b: %v", err)
	}

	// Verify the StatefulSets are truly different objects (different namespaces)
	if sts1.Namespace == sts2.Namespace {
		t.Errorf("expected StatefulSets to be in different namespaces, but both are in %s", sts1.Namespace)
	}

	// Verify ConfigMaps are namespace-isolated
	cm1 := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "namespace-a",
		Name:      "same-cluster-name" + constants.SuffixConfigMap,
	}, cm1); err != nil {
		t.Errorf("expected ConfigMap to exist in namespace-a: %v", err)
	}

	cm2 := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: "namespace-b",
		Name:      "same-cluster-name" + constants.SuffixConfigMap,
	}, cm2); err != nil {
		t.Errorf("expected ConfigMap to exist in namespace-b: %v", err)
	}
}

// TestMultiTenancyResourceLabeling verifies that all resources are labeled with the
// cluster name to enable proper identification and deletion.
func TestMultiTenancyResourceLabeling(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")
	ctx := context.Background()

	cluster := newMinimalCluster("labeled-cluster", "labeling-test")
	cluster.Status.Initialized = true
	createTLSSecretForTest(t, k8sClient, cluster)

	spec := newTestStatefulSetSpec(cluster)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify StatefulSet has correct labels
	sts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, sts); err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}

	expectedLabels := map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}

	for key, expectedVal := range expectedLabels {
		if sts.Labels[key] != expectedVal {
			t.Errorf("expected StatefulSet label %s=%s, got %s", key, expectedVal, sts.Labels[key])
		}
	}

	// Verify ConfigMap has correct labels
	cm := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixConfigMap,
	}, cm); err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}

	for key, expectedVal := range expectedLabels {
		if cm.Labels[key] != expectedVal {
			t.Errorf("expected ConfigMap label %s=%s, got %s", key, expectedVal, cm.Labels[key])
		}
	}
}

func TestOwnerReferencesSetOnCreatedResources(t *testing.T) {
	k8sClient := newTestClient(t)
	manager := NewManager(k8sClient, testScheme, "openbao-operator-system", "", nil, "")
	ctx := context.Background()

	// Create the cluster in the fake client so it has a UID for OwnerReference
	cluster := newMinimalCluster("owner-ref-test", "ownerref-ns")
	cluster.Status.Initialized = true

	// Set TypeMeta (required for SetControllerReference)
	cluster.APIVersion = apiVersion
	cluster.Kind = kind

	// Create the cluster resource first
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("failed to create cluster: %v", err)
	}

	// Re-fetch to get the UID
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, cluster); err != nil {
		t.Fatalf("failed to get cluster: %v", err)
	}

	createTLSSecretForTest(t, k8sClient, cluster)

	spec := newTestStatefulSetSpec(cluster)
	if err := manager.Reconcile(ctx, logr.Discard(), cluster, spec); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Helper to verify OwnerReference
	verifyOwnerRef := func(t *testing.T, refs []metav1.OwnerReference, resourceType string) {
		t.Helper()
		if len(refs) == 0 {
			t.Errorf("%s: expected at least one OwnerReference, got none", resourceType)
			return
		}
		found := false
		for _, ref := range refs {
			if ref.UID == cluster.UID {
				found = true
				if ref.Kind != kind {
					t.Errorf("%s: expected OwnerReference Kind '%s', got %s", resourceType, kind, ref.Kind)
				}
				if ref.Name != cluster.Name {
					t.Errorf("%s: expected OwnerReference Name %s, got %s", resourceType, cluster.Name, ref.Name)
				}
				if ref.Controller == nil || !*ref.Controller {
					t.Errorf("%s: expected OwnerReference Controller=true", resourceType)
				}
				break
			}
		}
		if !found {
			t.Errorf("%s: OwnerReference with cluster UID not found", resourceType)
		}
	}

	// Verify StatefulSet OwnerReference
	sts := &appsv1.StatefulSet{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}, sts); err != nil {
		t.Fatalf("expected StatefulSet to exist: %v", err)
	}
	verifyOwnerRef(t, sts.OwnerReferences, "StatefulSet")

	// Verify ConfigMap OwnerReference
	cm := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixConfigMap,
	}, cm); err != nil {
		t.Fatalf("expected ConfigMap to exist: %v", err)
	}
	verifyOwnerRef(t, cm.OwnerReferences, "ConfigMap")

	// Verify Headless Service OwnerReference
	svc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      headlessServiceName(cluster),
	}, svc); err != nil {
		t.Fatalf("expected headless Service to exist: %v", err)
	}
	verifyOwnerRef(t, svc.OwnerReferences, "Headless Service")

	// Verify Unseal Secret OwnerReference
	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixUnsealKey,
	}, secret); err != nil {
		t.Fatalf("expected unseal Secret to exist: %v", err)
	}
	verifyOwnerRef(t, secret.OwnerReferences, "Unseal Secret")

	// Verify ServiceAccount OwnerReference
	sa := &corev1.ServiceAccount{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      cluster.Name + constants.SuffixServiceAccount,
	}, sa); err != nil {
		t.Fatalf("expected ServiceAccount to exist: %v", err)
	}
	verifyOwnerRef(t, sa.OwnerReferences, "ServiceAccount")
}
