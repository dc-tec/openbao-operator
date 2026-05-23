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
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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

func grantTenantOpenBaoWriteAccess(t *testing.T, namespace, username string) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-openbao-writer",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters", "openbaorestores"},
				Verbs:     []string{"create", "delete", "get", "list", "patch", "update", "watch"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create tenant writer role: %v", err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-openbao-writer-binding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     username,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create tenant writer rolebinding: %v", err)
	}
}

func grantClusterHelperImageAccess(t *testing.T, namespace, clusterName, username string) {
	t.Helper()

	role := &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-helper-image-access",
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"openbao.org"},
				Resources:     []string{"openbaoclusters"},
				ResourceNames: []string{clusterName},
				Verbs:         []string{"get", "usehelperimages"},
			},
		},
	}
	if err := k8sClient.Create(ctx, role); err != nil {
		t.Fatalf("create helper image role: %v", err)
	}

	binding := &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-helper-image-access-binding",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.Name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     username,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
	}
	if err := k8sClient.Create(ctx, binding); err != nil {
		t.Fatalf("create helper image rolebinding: %v", err)
	}
}

func waitForOpenBaoRestoreAdmissionPolicies(t *testing.T, namespace string) {
	t.Helper()

	ensureDefaultAdmissionPoliciesApplied(t)

	for attempt := 0; attempt < 25; attempt++ {
		restore := &openbaov1alpha1.OpenBaoRestore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("restore-policy-probe-%d", attempt),
				Namespace: namespace,
			},
			Spec: openbaov1alpha1.OpenBaoRestoreSpec{
				Cluster: "policy-probe",
				Source: openbaov1alpha1.RestoreSource{
					Target: openbaov1alpha1.BackupTarget{
						Provider: "s3",
						Endpoint: "http://example.com",
						Bucket:   testBackupBucket,
					},
					Key: "clusters/probe/snapshot.snap",
				},
				JWTAuthRole: "restore-role",
			},
		}

		err := k8sClient.Create(ctx, restore)
		if err == nil {
			_ = k8sClient.Delete(ctx, restore)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		requireAdmissionDenied(t, err)
		return
	}

	t.Fatalf("expected OpenBaoRestore admission policies to become active after retries")
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

func TestVAP_OpenBaoCluster_RejectsOIDCBootstrapWithoutSelfInitEnabled(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-oidc-without-self-init")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: false,
		OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
			Enabled: true,
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "spec.selfInit.oidc.enabled requires spec.selfInit.enabled to be true") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RequiresTrustedIngressPeersForManagedIngress(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-ingress-without-peer")
	cluster.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    "bao.example.com",
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	wantMessage := "spec.ingress.enabled requires at least one spec.network.trustedIngressPeers"
	if !strings.Contains(err.Error(), wantMessage) {
		t.Fatalf("unexpected error message: %v", err)
	}

	allowed := newMinimalClusterObj(namespace, "cluster-ingress-with-peer")
	allowed.Spec.Ingress = &openbaov1alpha1.IngressConfig{
		Enabled: true,
		Host:    "bao.example.com",
	}
	allowed.Spec.Network = &openbaov1alpha1.NetworkConfig{
		TrustedIngressPeers: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "ingress-system",
					},
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, allowed); err != nil {
		t.Fatalf("expected ingress with trusted ingress peers to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoCluster_DeniesCustomBackupImageWithoutHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	username := "backup-image-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	tenantClient := newImpersonatedClient(t, username)

	cluster := newMinimalClusterObj(namespace, "cluster-custom-backup-image-denied")
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 0 * * *",
		Image:       "ghcr.io/attacker/backup-exfil:latest",
		JWTAuthRole: "backup-role",
		Target: openbaov1alpha1.BackupTarget{
			Provider: "s3",
			Endpoint: "https://objectstore.example.com",
			Bucket:   testBackupBucket,
		},
	}

	err := tenantClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "custom backup helper images") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_DeniesBackupImageChangeWithoutHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-custom-backup-image-update-denied")
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	username := "backup-image-update-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	tenantClient := newImpersonatedClient(t, username)

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := tenantClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster as tenant editor: %v", err)
	}
	original := latest.DeepCopy()
	latest.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 0 * * *",
		Image:       "ghcr.io/attacker/backup-exfil:latest",
		JWTAuthRole: "backup-role",
		Target: openbaov1alpha1.BackupTarget{
			Provider: "s3",
			Endpoint: "https://objectstore.example.com",
			Bucket:   testBackupBucket,
		},
	}

	err := tenantClient.Patch(ctx, &latest, client.MergeFrom(original))
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "custom backup helper images") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_AllowsCustomBackupImageWithHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	username := "backup-image-delegate"
	clusterName := "cluster-custom-backup-image-allowed"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	grantClusterHelperImageAccess(t, namespace, clusterName, username)
	tenantClient := newImpersonatedClient(t, username)

	cluster := newMinimalClusterObj(namespace, clusterName)
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 0 * * *",
		Image:       "ghcr.io/platform/backup-helper:1.2.3",
		JWTAuthRole: "backup-role",
		Target: openbaov1alpha1.BackupTarget{
			Provider: "s3",
			Endpoint: "https://objectstore.example.com",
			Bucket:   testBackupBucket,
		},
	}

	if err := tenantClient.Create(ctx, cluster); err != nil {
		t.Fatalf("expected helper-image-authorized OpenBaoCluster create to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoCluster_AllowsUnchangedCustomBackupImageWithoutHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-custom-backup-image-unchanged")
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 0 * * *",
		Image:       "ghcr.io/platform/backup-helper:1.2.3",
		JWTAuthRole: "backup-role",
		Target: openbaov1alpha1.BackupTarget{
			Provider: "s3",
			Endpoint: "https://objectstore.example.com",
			Bucket:   testBackupBucket,
		},
	}
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create platform-authored OpenBaoCluster with custom helper image: %v", err)
	}

	username := "backup-image-standard-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	tenantClient := newImpersonatedClient(t, username)

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := tenantClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster as tenant editor: %v", err)
	}
	original := latest.DeepCopy()
	latest.Spec.Backup.Schedule = "0 1 * * *"

	if err := tenantClient.Patch(ctx, &latest, client.MergeFrom(original)); err != nil {
		t.Fatalf("expected unchanged custom backup helper image update to succeed, got: %v", err)
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
	wantMessage := "spec.unseal.credentialsSecretRef for ocikms requires spec.unseal.ocikms.authTypeAPIKey=true"
	if !strings.Contains(err.Error(), wantMessage) {
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
	if !strings.Contains(err.Error(), "Transit unseal address must use HTTPS") {
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
	wantMessage := "spec.unseal.transit.tlsClientCert and spec.unseal.transit.tlsClientKey must be set together"
	if !strings.Contains(err.Error(), wantMessage) {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_AllowsHardenedOfficialImageVerificationDefaults(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newValidHardenedAdmissionCluster(namespace, "cluster-hardened-image-verification-defaults")
	cluster.Spec.ImageVerification = &openbaov1alpha1.ImageVerificationConfig{
		Enabled:       true,
		FailurePolicy: "Block",
	}
	cluster.Spec.OperatorImageVerification = &openbaov1alpha1.ImageVerificationConfig{
		Enabled:       true,
		FailurePolicy: "Block",
	}

	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf(
			"expected Hardened OpenBaoCluster with enabled official image verification defaults to succeed, got: %v",
			err,
		)
	}
}

func TestVAP_OpenBaoCluster_RejectsHardenedWeakeningSecurityContext(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	tests := []struct {
		name      string
		configure func(*corev1.PodSecurityContext)
		wantError string
	}{
		{
			name: "run as root",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.RunAsNonRoot = ptr.To(false)
			},
			wantError: "Hardened profile does not allow spec.securityContext.runAsNonRoot=false",
		},
		{
			name: "unconfined seccomp",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.SeccompProfile = &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeUnconfined,
				}
			},
			wantError: "Hardened profile does not allow spec.securityContext.seccompProfile.type=Unconfined",
		},
		{
			name: "root uid",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.RunAsUser = ptr.To(int64(0))
			},
			wantError: "Hardened profile does not allow root UID/GID overrides in spec.securityContext",
		},
		{
			name: "root gid",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.RunAsGroup = ptr.To(int64(0))
			},
			wantError: "Hardened profile does not allow root UID/GID overrides in spec.securityContext",
		},
		{
			name: "root fs group",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.FSGroup = ptr.To(int64(0))
			},
			wantError: "Hardened profile does not allow root UID/GID overrides in spec.securityContext",
		},
		{
			name: "root supplemental group",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.SupplementalGroups = []int64{0}
			},
			wantError: "Hardened profile does not allow root supplemental groups in spec.securityContext",
		},
		{
			name: "pod sysctl",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.Sysctls = []corev1.Sysctl{{
					Name:  "kernel.shm_rmid_forced",
					Value: "1",
				}}
			},
			wantError: "Hardened profile does not allow pod sysctl overrides in spec.securityContext",
		},
		{
			name: "windows options",
			configure: func(securityContext *corev1.PodSecurityContext) {
				securityContext.WindowsOptions = &corev1.WindowsSecurityContextOptions{}
			},
			wantError: "Hardened profile does not allow Windows pod security options in spec.securityContext",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newValidHardenedAdmissionCluster(namespace, fmt.Sprintf("cluster-hardened-security-%d", i))
			cluster.Spec.SecurityContext = &corev1.PodSecurityContext{}
			tt.configure(cluster.Spec.SecurityContext)

			err := k8sClient.Create(ctx, cluster)
			requireAdmissionDenied(t, err)
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestVAP_OpenBaoCluster_AllowsHardenedSafeSecurityContextOverrides(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newValidHardenedAdmissionCluster(namespace, "cluster-hardened-safe-security")
	cluster.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsNonRoot:        ptr.To(true),
		RunAsUser:           ptr.To(int64(1001)),
		RunAsGroup:          ptr.To(int64(1001)),
		FSGroup:             ptr.To(int64(1001)),
		FSGroupChangePolicy: ptr.To(corev1.FSGroupChangeOnRootMismatch),
		SupplementalGroups:  []int64{1002},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("expected Hardened OpenBaoCluster with safe securityContext overrides to succeed, got: %v", err)
	}
}

func newValidHardenedAdmissionCluster(namespace, name string) *openbaov1alpha1.OpenBaoCluster {
	cluster := newMinimalClusterObj(namespace, name)
	cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
	cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeExternal
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "health-check",
				Operation: openbaov1alpha1.SelfInitOperationRead,
				Path:      "sys/health",
			},
		},
	}
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "awskms",
		AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
			Region:   "eu-central-1",
			KMSKeyID: "alias/openbao-unseal",
		},
	}

	return cluster
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
					"version":  testOpenBaoVersion244,
					"image":    testOpenBaoImage244,
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
		wantMessage := "spec.initContainer is optional; when set, spec.initContainer.enabled must be true."
		if !strings.Contains(err.Error(), wantMessage) {
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
	cluster.Spec.Version = testOpenBaoVersion250
	cluster.Spec.Image = testOpenBaoImage250
	if err := k8sClient.Create(ctx, cluster); err != nil {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	updateClusterStatus(t, cluster, func(status *openbaov1alpha1.OpenBaoClusterStatus) {
		status.Initialized = true
		status.CurrentVersion = testOpenBaoVersion250
	})

	var latest openbaov1alpha1.OpenBaoCluster
	key := types.NamespacedName{Namespace: namespace, Name: cluster.Name}
	if err := k8sClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster: %v", err)
	}

	original := latest.DeepCopy()
	latest.Spec.Version = testOpenBaoVersion244
	latest.Spec.Image = testOpenBaoImage244

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
		status.CurrentVersion = testOpenBaoVersion244
		status.Upgrade = &openbaov1alpha1.UpgradeProgress{
			FromVersion:      testOpenBaoVersion244,
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
	latest.Spec.Version = testOpenBaoVersion250
	latest.Spec.Image = testOpenBaoImage250

	err := k8sClient.Patch(ctx, &latest, client.MergeFrom(original))
	requireAdmissionDenied(t, err)
	wantMessage := "spec.version cannot be reduced below status.upgrade.targetVersion after rolling progress has started."
	if !strings.Contains(err.Error(), wantMessage) {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsNumericBackupEndpoint(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	cluster := newMinimalClusterObj(namespace, "cluster-backup-numeric-endpoint")
	cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
		Schedule:    "0 0 * * *",
		Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
		JWTAuthRole: "backup-role",
		Target: openbaov1alpha1.BackupTarget{
			Endpoint: "http://2130706433:9000",
			Bucket:   testBackupBucket,
			CredentialsSecretRef: &corev1.LocalObjectReference{
				Name: "backup-creds",
			},
		},
	}

	err := k8sClient.Create(ctx, cluster)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "numeric IP encoding") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoCluster_RejectsBackupEndpointSSRFBypasses(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	tests := []struct {
		name        string
		endpoint    string
		wantMessage string
	}{
		{
			name:        "uppercase-scheme-link-local",
			endpoint:    "HTTP://169.254.169.254/latest/meta-data",
			wantMessage: "Backup endpoint cannot point to link-local addresses",
		},
		{
			name:        "userinfo-link-local",
			endpoint:    "http://storage.example.com@169.254.169.254/latest/meta-data",
			wantMessage: "Backup endpoint cannot point to link-local addresses",
		},
		{
			name:        "shorthand-loopback",
			endpoint:    "http://127.1:9000",
			wantMessage: "numeric IP encoding",
		},
		{
			name:        "hex-loopback",
			endpoint:    "http://0x7f000001:9000",
			wantMessage: "numeric IP encoding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := newMinimalClusterObj(namespace, "cluster-backup-ssrf-"+tt.name)
			cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
				Schedule:    "0 0 * * *",
				Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
				JWTAuthRole: "backup-role",
				Target: openbaov1alpha1.BackupTarget{
					Endpoint: tt.endpoint,
					Bucket:   testBackupBucket,
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "backup-creds",
					},
				},
			}

			err := k8sClient.Create(ctx, cluster)
			requireAdmissionDenied(t, err)
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}

func TestVAP_OpenBaoRestore_DeniesCustomImageWithoutHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoRestoreAdmissionPolicies(t, namespace)

	username := "restore-image-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	tenantClient := newImpersonatedClient(t, username)

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-custom-image-denied",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "target-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Endpoint: "https://objectstore.example.com",
					Bucket:   testBackupBucket,
				},
				Key: "clusters/prod/snapshot.snap",
			},
			JWTAuthRole: "restore-role",
			Image:       "ghcr.io/attacker/restore-exfil:latest",
			Force:       true,
		},
	}

	err := tenantClient.Create(ctx, restore)
	requireAdmissionDenied(t, err)
	if !strings.Contains(err.Error(), "custom restore helper images") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestVAP_OpenBaoRestore_AllowsCustomImageWithHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoRestoreAdmissionPolicies(t, namespace)

	username := "restore-image-delegate"
	clusterName := "target-cluster"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	grantClusterHelperImageAccess(t, namespace, clusterName, username)
	tenantClient := newImpersonatedClient(t, username)

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-custom-image-allowed",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: clusterName,
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Endpoint: "https://objectstore.example.com",
					Bucket:   testBackupBucket,
				},
				Key: "clusters/prod/snapshot.snap",
			},
			JWTAuthRole: "restore-role",
			Image:       "ghcr.io/platform/backup-helper:1.2.3",
			Force:       true,
		},
	}

	if err := tenantClient.Create(ctx, restore); err != nil {
		t.Fatalf("expected helper-image-authorized OpenBaoRestore create to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoRestore_AllowsUnchangedCustomImageUpdateWithoutHelperImageVerb(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoRestoreAdmissionPolicies(t, namespace)

	clusterName := "target-cluster"
	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-custom-image-unchanged",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: clusterName,
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Endpoint: "https://objectstore.example.com",
					Bucket:   testBackupBucket,
				},
				Key: "clusters/prod/snapshot.snap",
			},
			JWTAuthRole: "restore-role",
			Image:       "ghcr.io/platform/backup-helper:1.2.3",
			Force:       true,
		},
	}
	if err := k8sClient.Create(ctx, restore); err != nil {
		t.Fatalf("create platform-authored OpenBaoRestore with custom helper image: %v", err)
	}

	username := "restore-image-standard-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, username)
	tenantClient := newImpersonatedClient(t, username)

	var latest openbaov1alpha1.OpenBaoRestore
	key := types.NamespacedName{Namespace: namespace, Name: restore.Name}
	if err := tenantClient.Get(ctx, key, &latest); err != nil {
		t.Fatalf("get OpenBaoRestore as tenant editor: %v", err)
	}
	original := latest.DeepCopy()
	latest.Annotations = map[string]string{"openbao.org/test": "metadata-update"}

	if err := tenantClient.Patch(ctx, &latest, client.MergeFrom(original)); err != nil {
		t.Fatalf("expected unchanged custom restore helper image update to succeed, got: %v", err)
	}
}

func TestVAP_OpenBaoRestore_RejectsUnsafeEndpoints(t *testing.T) {
	ensureDefaultAdmissionPoliciesApplied(t)
	namespace := newTestNamespace(t)

	tests := []struct {
		name        string
		endpoint    string
		wantMessage string
	}{
		{
			name:        "link-local",
			endpoint:    "http://169.254.169.254/latest/meta-data",
			wantMessage: "Restore endpoint cannot point to link-local addresses",
		},
		{
			name:        "uppercase-scheme-link-local",
			endpoint:    "HTTPS://169.254.169.254/latest/meta-data",
			wantMessage: "Restore endpoint cannot point to link-local addresses",
		},
		{
			name:        "userinfo-link-local",
			endpoint:    "https://storage.example.com@169.254.169.254/latest/meta-data",
			wantMessage: "Restore endpoint cannot point to link-local addresses",
		},
		{
			name:        "shorthand-loopback",
			endpoint:    "https://127.1:9000",
			wantMessage: "numeric IP encoding",
		},
		{
			name:        "hex-loopback",
			endpoint:    "https://0x7f000001:9000",
			wantMessage: "numeric IP encoding",
		},
		{
			name:        "plain-http-external",
			endpoint:    "http://example.com",
			wantMessage: "Restore endpoint must use HTTPS or S3 scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for attempt := 0; attempt < 25; attempt++ {
				restore := &openbaov1alpha1.OpenBaoRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("restore-%s-%d", tt.name, attempt),
						Namespace: namespace,
					},
					Spec: openbaov1alpha1.OpenBaoRestoreSpec{
						Cluster: "does-not-matter-for-admission",
						Source: openbaov1alpha1.RestoreSource{
							Target: openbaov1alpha1.BackupTarget{
								Provider: "s3",
								Endpoint: tt.endpoint,
								Bucket:   testBackupBucket,
								CredentialsSecretRef: &corev1.LocalObjectReference{
									Name: "restore-creds",
								},
							},
							Key: "clusters/prod/snapshot.snap",
						},
						JWTAuthRole: "restore",
						Image:       "ghcr.io/dc-tec/openbao-backup:1.0.0",
						Force:       true,
					},
				}

				err := k8sClient.Create(ctx, restore)
				if err == nil {
					_ = k8sClient.Delete(ctx, restore)
					time.Sleep(100 * time.Millisecond)
					continue
				}

				requireAdmissionDenied(t, err)
				if !strings.Contains(err.Error(), tt.wantMessage) {
					t.Fatalf("unexpected error message: %v", err)
				}
				return
			}

			t.Fatalf("expected VAP to deny OpenBaoRestore endpoint %q after retries", tt.endpoint)
		})
	}
}

func TestVAP_OpenBaoRestore_AllowsInClusterHTTPServiceEndpoint(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoRestoreAdmissionPolicies(t, namespace)

	restore := &openbaov1alpha1.OpenBaoRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restore-in-cluster-http-service",
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoRestoreSpec{
			Cluster: "target-cluster",
			Source: openbaov1alpha1.RestoreSource{
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Endpoint: "http://rustfs-svc.rustfs.svc.cluster.local:9000",
					Bucket:   testBackupBucket,
				},
				Key: "clusters/prod/snapshot.snap",
			},
			JWTAuthRole: "restore",
			Force:       true,
		},
	}

	if err := k8sClient.Create(ctx, restore); err != nil {
		t.Fatalf("expected in-cluster HTTP restore endpoint to be allowed, got: %v", err)
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
		status.CurrentVersion = testOpenBaoVersion244
		status.Upgrade = &openbaov1alpha1.UpgradeProgress{
			FromVersion:      testOpenBaoVersion244,
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
	latest.Spec.Version = testOpenBaoVersion250
	latest.Spec.Image = testOpenBaoImage250

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
		tenantKey := types.NamespacedName{Namespace: tenant.Namespace, Name: tenant.Name}
		if err := k8sClient.Get(ctx, tenantKey, &latest); err != nil {
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
