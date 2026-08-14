//go:build integration
// +build integration

package integration

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/hardenedcontract"
	hardenedfixtures "github.com/dc-tec/openbao-operator/test/fixtures/hardenedcontract"
)

func TestVAP_OpenBaoCluster_HardenedContractCatalog(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	const restrictedUsername = "hardened-contract-catalog-editor"
	grantTenantOpenBaoWriteAccess(t, namespace, restrictedUsername)
	restrictedClient := newImpersonatedClient(t, restrictedUsername)

	for index, fixture := range hardenedfixtures.Fixtures() {
		t.Run(fixture.Name, func(t *testing.T) {
			cluster := hardenedfixtures.NewValidCluster(
				namespace,
				fmt.Sprintf("hardened-contract-%02d", index),
			)
			if fixture.Configure != nil {
				fixture.Configure(cluster)
			}

			requestClient := k8sClient
			if fixture.AuthorizationOnly {
				requestClient = restrictedClient
			}
			err := requestClient.Create(ctx, cluster, client.DryRunAll)

			if fixture.AdmissionRule == "" {
				if err != nil {
					t.Fatalf("expected admission to accept fixture, got: %v", err)
				}
				return
			}

			requireAdmissionDenied(t, err)
			rule, found := hardenedcontract.RuleFor(fixture.AdmissionRule)
			if !found {
				t.Fatalf("fixture references unknown admission rule %q", fixture.AdmissionRule)
			}
			if !strings.Contains(err.Error(), rule.AdmissionMessage) {
				t.Fatalf(
					"admission error for rule %q does not contain catalog message %q: %v",
					rule.ID,
					rule.AdmissionMessage,
					err,
				)
			}
		})
	}
}

func TestVAP_OpenBaoCluster_HardenedAllowsUnsealSecretReferences(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	tests := []struct {
		name      string
		configure func(*openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name: "aws",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "aws-credentials"}
			},
		},
		{
			name: "azure",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type:                 "azurekeyvault",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "azure-credentials"},
					AzureKeyVault: &openbaov1alpha1.AzureKeyVaultSealConfig{
						VaultName: "fixture-vault",
						KeyName:   "fixture-key",
					},
				}
			},
		},
		{
			name: "pkcs11",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type:                 "pkcs11",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "pkcs11-credentials"},
					PKCS11: &openbaov1alpha1.PKCS11SealConfig{
						Lib:      "/usr/lib/libpkcs11.so",
						Slot:     "0",
						KeyLabel: "openbao",
					},
				}
			},
		},
		{
			name: "kms-plugin",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type:                 "kms",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "kms-plugin-credentials"},
					KMS:                  &openbaov1alpha1.KMSPluginSealConfig{PluginName: "corp-kms"},
				}
			},
		},
		{
			name: "transit",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type:                 "transit",
					CredentialsSecretRef: &corev1.LocalObjectReference{Name: "transit-credentials"},
					Transit: &openbaov1alpha1.TransitSealConfig{
						Address:   "https://transit.example.com",
						KeyName:   "autounseal",
						MountPath: "transit/",
					},
				}
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := hardenedfixtures.NewValidCluster(namespace, fmt.Sprintf("hardened-secret-ref-%02d", index))
			test.configure(cluster)
			if err := k8sClient.Create(ctx, cluster, client.DryRunAll); err != nil {
				t.Fatalf("expected Hardened Secret reference to be accepted, got: %v", err)
			}
		})
	}
}

func TestVAP_OpenBaoCluster_DevelopmentAllowsInlineUnsealConfiguration(t *testing.T) {
	namespace := newTestNamespace(t)
	waitForOpenBaoClusterAdmissionPolicies(t, namespace)

	tests := []struct {
		name      string
		configure func(*openbaov1alpha1.OpenBaoCluster)
	}{
		{
			name: "aws",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal.AWSKMS.SecretKey = "inline-secret-key"
				cluster.Spec.Unseal.AWSKMS.SessionToken = "inline-session-token"
			},
		},
		{
			name: "azure",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "azurekeyvault",
					AzureKeyVault: &openbaov1alpha1.AzureKeyVaultSealConfig{
						VaultName: "fixture-vault", KeyName: "fixture-key", ClientSecret: "inline-client-secret",
					},
				}
			},
		},
		{
			name: "pkcs11",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "pkcs11",
					PKCS11: &openbaov1alpha1.PKCS11SealConfig{
						Lib: "/usr/lib/libpkcs11.so", Slot: "0", PIN: "inline-pin", KeyLabel: "openbao",
					},
				}
			},
		},
		{
			name: "kms-plugin",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "kms",
					KMS: &openbaov1alpha1.KMSPluginSealConfig{
						PluginName: "corp-kms", Config: map[string]string{"token": "inline-token"},
					},
				}
			},
		},
		{
			name: "transit",
			configure: func(cluster *openbaov1alpha1.OpenBaoCluster) {
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "transit",
					Transit: &openbaov1alpha1.TransitSealConfig{
						Address: "https://transit.example.com", Token: "inline-token", KeyName: "autounseal", MountPath: "transit/",
					},
				}
			},
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cluster := hardenedfixtures.NewValidCluster(namespace, fmt.Sprintf("development-inline-%02d", index))
			cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
			test.configure(cluster)
			if err := k8sClient.Create(ctx, cluster, client.DryRunAll); err != nil {
				t.Fatalf("expected Development inline configuration to be accepted, got: %v", err)
			}
		})
	}
}
