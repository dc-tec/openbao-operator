//go:build integration
// +build integration

package integration

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func requireInvalidRequest(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected invalid request error, got nil")
	}

	if apierrors.IsInvalid(err) {
		return
	}

	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		status := apiStatus.Status()
		if status.Code == http.StatusUnprocessableEntity || status.Reason == metav1.StatusReasonInvalid {
			return
		}
	}

	if strings.Contains(err.Error(), "is invalid") {
		return
	}

	t.Fatalf("expected invalid request error, got %T: %v", err, err)
}

func TestCRD_OpenBaoRestore_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	restore := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoRestore",
			"metadata": map[string]any{
				"name":      "restore-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, restore)
	requireInvalidRequest(t, err)
}

func TestCRD_OpenBaoCluster_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoCluster",
			"metadata": map[string]any{
				"name":      "cluster-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
}

func TestCRD_OpenBaoTenant_RejectsMissingSpec(t *testing.T) {
	namespace := newTestNamespace(t)

	tenant := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoTenant",
			"metadata": map[string]any{
				"name":      "tenant-missing-spec",
				"namespace": namespace,
			},
		},
	}

	err := k8sClient.Create(ctx, tenant)
	requireInvalidRequest(t, err)
}

func TestVAP_OpenBaoRestore_RejectsSpecMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		name := fmt.Sprintf("restore-immutable-%d", attempt)
		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "cluster-1",
				Source: openbaov1alpha1.RestoreSource{
					Key: "backup.enc",
					Target: openbaov1alpha1.BackupTarget{
						Endpoint: "https://objectstore.example.com",
						Bucket:   "backups",
					},
				},
				JWTAuthRole: "restore-role",
			},
		}

		if err := k8sClient.Create(ctx, restore); err != nil {
			t.Fatalf("create OpenBaoRestore: %v", err)
		}

		var latest openbaov1alpha1.OpenBaoRestore
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: restore.Name}, &latest); err != nil {
			t.Fatalf("get OpenBaoRestore: %v", err)
		}
		original := latest.DeepCopy()
		latest.Spec.Source.Key = "backup-v2.enc"

		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			_ = k8sClient.Delete(ctx, &latest)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec is immutable") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoRestore spec mutation after retries")
}

func TestCRD_OpenBaoCluster_RequiresProfile(t *testing.T) {
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		cluster := newMinimalClusterObj(namespace, fmt.Sprintf("cluster-missing-profile-%d", attempt))
		cluster.Spec.Profile = ""

		err := k8sClient.Create(ctx, cluster)
		if err == nil {
			_ = k8sClient.Delete(ctx, cluster)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireInvalidRequest(t, err)
		if !strings.Contains(err.Error(), "spec.profile") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected CRD validation to reject OpenBaoCluster create without spec.profile after retries")
}

func TestVAP_OpenBaoCluster_AllowsDefaultInitContainer(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-default-init")
	cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
	cluster.Spec.InitContainer = nil

	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("expected OpenBaoCluster create without spec.initContainer to succeed, got: %v", err)
	}
}

func TestCRD_OpenBaoCluster_RejectsHAACMEWithoutSharedCache(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "cluster-acme-ha-missing-cache")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		DirectoryURL: "https://acme.example/directory",
		Domain:       "bao.example.com",
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
	if !strings.Contains(err.Error(), "HA ACME clusters require spec.tls.acme.sharedCache") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCRD_OpenBaoCluster_RejectsSharedCacheOutsideACME(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "cluster-non-acme-shared-cache")
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
	cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
		SharedCache: &openbaov1alpha1.ACMESharedCacheConfig{
			Mode: openbaov1alpha1.ACMESharedCacheModeManagedPVC,
			Size: "1Gi",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
	if !strings.Contains(err.Error(), "spec.tls.acme.sharedCache is only supported when spec.tls.mode is ACME") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCRD_OpenBaoCluster_RejectsOCIKMSCredentialsSecretWithoutAPIKeyMode(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "cluster-ocikms-secret-without-api-key")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type:                 "ocikms",
		CredentialsSecretRef: &corev1.LocalObjectReference{Name: "oci-creds"},
		OCIKMS: &openbaov1alpha1.OCIKMSSealConfig{
			KeyID:              "ocid1.key.oc1..example",
			CryptoEndpoint:     "https://kms.us-ashburn-1.oraclecloud.com",
			ManagementEndpoint: "https://kms.us-ashburn-1.oraclecloud.com",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
	if !strings.Contains(err.Error(), "spec.unseal.credentialsSecretRef for ocikms requires spec.unseal.ocikms.authTypeAPIKey=true") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCRD_OpenBaoCluster_RejectsPKCS11WithoutSlotOrTokenLabel(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "cluster-pkcs11-missing-slot-tokenlabel")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "pkcs11",
		PKCS11: &openbaov1alpha1.PKCS11SealConfig{
			Lib:      "/usr/lib/libpkcs11.so",
			KeyLabel: "openbao-key",
			PIN:      "1234",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
	if !strings.Contains(err.Error(), "spec.unseal.pkcs11.slot or spec.unseal.pkcs11.tokenLabel is required") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCRD_OpenBaoCluster_RejectsPKCS11WithSlotAndTokenLabel(t *testing.T) {
	namespace := newTestNamespace(t)

	cluster := newMinimalClusterObj(namespace, "cluster-pkcs11-slot-and-tokenlabel")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "pkcs11",
		PKCS11: &openbaov1alpha1.PKCS11SealConfig{
			Lib:        "/usr/lib/libpkcs11.so",
			Slot:       "0",
			TokenLabel: "openbao-token",
			KeyLabel:   "openbao-key",
			PIN:        "1234",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireInvalidRequest(t, err)
	if !strings.Contains(err.Error(), "spec.unseal.pkcs11.slot and spec.unseal.pkcs11.tokenLabel are mutually exclusive") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsHardenedTransitInlineToken(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-hardened-transit-inline-token")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true}
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "transit",
		Transit: &openbaov1alpha1.TransitSealConfig{
			Address:   "https://infra-bao.example",
			Token:     "s.inline",
			KeyName:   "autounseal",
			MountPath: "transit/",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "Hardened profile does not allow spec.unseal.transit.token") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsHardenedTransitAddressWithoutHTTPS(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-hardened-transit-http")
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{Enabled: true}
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "transit",
		Transit: &openbaov1alpha1.TransitSealConfig{
			Address:   "http://infra-bao.example",
			KeyName:   "autounseal",
			MountPath: "transit/",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "Hardened profile requires spec.unseal.transit.address to use HTTPS") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsTransitClientCertWithoutKey(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-transit-client-cert-without-key")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "transit",
		Transit: &openbaov1alpha1.TransitSealConfig{
			Address:       "https://infra-bao.example",
			KeyName:       "autounseal",
			MountPath:     "transit/",
			TLSClientCert: "/etc/bao/seal-creds/client.crt",
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "spec.unseal.transit.tlsClientCert and spec.unseal.transit.tlsClientKey must be set together") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsDisabledInitContainerOverride(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		cluster := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "openbao.org/v1alpha1",
				"kind":       "OpenBaoCluster",
				"metadata": map[string]any{
					"name":      fmt.Sprintf("cluster-disabled-init-override-%d", attempt),
					"namespace": namespace,
				},
				"spec": map[string]any{
					"version":  "2.4.4",
					"image":    "openbao/openbao:2.4.4",
					"replicas": int64(3),
					"profile":  "Development",
					"tls": map[string]any{
						"enabled":        true,
						"rotationPeriod": "720h",
					},
					"storage": map[string]any{
						"size": "10Gi",
					},
					"initContainer": map[string]any{
						"enabled": false,
					},
				},
			},
		}

		err := k8sClient.Create(ctx, cluster)
		if err == nil {
			_ = k8sClient.Delete(ctx, cluster)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec.initContainer is optional; when set, spec.initContainer.enabled must be true.") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoCluster create with disabled initContainer override after retries")
}

func TestVAP_OpenBaoCluster_RejectsDowngradeBelowCurrentVersion(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-downgrade-current-version")
	cluster.Spec.Version = "2.5.0"
	cluster.Spec.Image = "openbao/openbao:2.5.0"
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = "2.5.0"
	})

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := k8sClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster: %v", err)
	}

	original := latest.DeepCopy()
	latest.Spec.Version = "2.4.4"
	latest.Spec.Image = "openbao/openbao:2.4.4"

	err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "spec.version cannot be downgraded below status.currentVersion.") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsRollingTargetRegressionAfterRolloutStarts(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-rolling-target-regression")
	cluster.Spec.Version = "2.6.0"
	cluster.Spec.Image = "openbao/openbao:2.6.0"
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = "2.4.4"
		status.Upgrade = &openbaov1alpha1.UpgradeProgress{
			FromVersion:      "2.4.4",
			TargetVersion:    "2.6.0",
			CurrentPartition: 2,
			CompletedPods:    []int32{2},
		}
	})

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := k8sClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster: %v", err)
	}

	original := latest.DeepCopy()
	latest.Spec.Version = "2.5.0"
	latest.Spec.Image = "openbao/openbao:2.5.0"

	err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "spec.version cannot be reduced below status.upgrade.targetVersion after rolling progress has started.") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_AllowsRollingTargetCorrectionBeforeRolloutStarts(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-rolling-target-correction")
	cluster.Spec.Version = "2.6.0"
	cluster.Spec.Image = "openbao/openbao:2.6.0"
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = "2.4.4"
		status.Upgrade = &openbaov1alpha1.UpgradeProgress{
			FromVersion:      "2.4.4",
			TargetVersion:    "2.6.0",
			CurrentPartition: cluster.Spec.Replicas,
		}
	})

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := k8sClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster: %v", err)
	}

	original := latest.DeepCopy()
	latest.Spec.Version = "2.5.0"
	latest.Spec.Image = "openbao/openbao:2.5.0"

	if err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original)); err != nil {
		t.Fatalf("expected retarget before rollout progress to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoTenant_RejectsCrossNamespaceSelfService(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)
	targetNamespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tenant-self-service-%d", attempt),
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: targetNamespace,
			},
		}

		err := k8sClient.Create(ctx, tenant)
		if err == nil {
			_ = k8sClient.Delete(ctx, tenant)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "can only target its own namespace") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny cross-namespace OpenBaoTenant create after retries")
}

func TestVAP_OpenBaoTenant_RejectsSelfServiceQuotaCustomization(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	namespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tenant-self-service-quota-%d", attempt),
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: namespace,
				Quota: &corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods: resource.MustParse("5"),
					},
				},
			},
		}

		err := k8sClient.Create(ctx, tenant)
		if err == nil {
			_ = k8sClient.Delete(ctx, tenant)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "may not customize spec.quota or spec.limitRange") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny self-service OpenBaoTenant quota customization after retries")
}

func TestVAP_OpenBaoTenant_AllowsAdminQuotaCustomization(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	operatorNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openbao-operator-system",
		},
	}
	if err := k8sClient.Create(ctx, operatorNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create operator namespace: %v", err)
	}

	targetNamespace := newTestNamespace(t)
	tenant := &openbaov1alpha1.OpenBaoTenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-admin-quota",
			Namespace: operatorNamespace.Name,
		},
		Spec: openbaov1alpha1.OpenBaoTenantSpec{
			TargetNamespace: targetNamespace,
			Quota: &corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourcePods: resource.MustParse("5"),
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, tenant); err != nil {
		t.Fatalf("expected operator-namespace OpenBaoTenant with quota override to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoTenant_RejectsTargetNamespaceMutation(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)

	operatorNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openbao-operator-system",
		},
	}
	if err := k8sClient.Create(ctx, operatorNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create operator namespace: %v", err)
	}

	targetNamespace := newTestNamespace(t)
	otherTargetNamespace := newTestNamespace(t)

	for attempt := 0; attempt < 25; attempt++ {
		tenant := &openbaov1alpha1.OpenBaoTenant{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("tenant-immutable-%d", attempt),
				Namespace: operatorNamespace.Name,
			},
			Spec: openbaov1alpha1.OpenBaoTenantSpec{
				TargetNamespace: targetNamespace,
			},
		}

		if err := k8sClient.Create(ctx, tenant); err != nil {
			t.Fatalf("create OpenBaoTenant: %v", err)
		}

		var latest openbaov1alpha1.OpenBaoTenant
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: tenant.Namespace, Name: tenant.Name}, &latest); err != nil {
			t.Fatalf("get OpenBaoTenant: %v", err)
		}
		original := latest.DeepCopy()
		latest.Spec.TargetNamespace = otherTargetNamespace

		err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
		if err == nil {
			_ = k8sClient.Delete(ctx, &latest)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		if !strings.Contains(err.Error(), "spec.targetNamespace is immutable") {
			t.Fatalf("unexpected error message: %v", err)
		}
		return
	}

	t.Fatalf("expected VAP to deny OpenBaoTenant targetNamespace mutation after retries")
}
