package openbaocluster

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetACMEIntegrationReadyCondition_FastContract(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	newACMECluster := func() *openbaov1alpha1.OpenBaoCluster {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.TLS.Mode = openbaov1alpha1.TLSModeACME
		cluster.Spec.TLS.ACME = &openbaov1alpha1.ACMEConfig{
			DirectoryURL: "https://acme.example/directory",
		}
		cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
			Type: "transit",
			Transit: &openbaov1alpha1.TransitSealConfig{
				Address:   "https://infra-bao.example",
				KeyName:   "autounseal",
				MountPath: "transit/",
			},
		}
		return cluster
	}

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		wantPresent   bool
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "non acme removes condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Status.Conditions = []metav1.Condition{{
					Type:   string(openbaov1alpha1.ConditionACMEIntegrationReady),
					Status: metav1.ConditionTrue,
					Reason: constants.ReasonACMEIntegrationReady,
				}}
				return cluster
			}(),
			wantPresent: false,
		},
		{
			name: "public acme integration ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				return newACMECluster()
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonACMEIntegrationReady,
			wantMessageIn: "prerequisites are satisfied",
		},
		{
			name: "gateway passthrough required",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Gateway = &openbaov1alpha1.GatewayConfig{
					Enabled:        true,
					Hostname:       "bao.example.test",
					TLSPassthrough: false,
					GatewayRef: openbaov1alpha1.GatewayReference{
						Name: "shared-gateway",
					},
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonACMEGatewayNotConfiguredForPassthrough,
			wantMessageIn: "tlsPassthrough=true",
		},
		{
			name: "private acme domain must resolve in ha",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Replicas = 2
				cluster.Status.Initialized = true
				cluster.Spec.TLS.ACME.Domains = []string{"does-not-resolve.invalid"}
				cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
					ACMECARoot: "/etc/bao/seal-creds/ca.crt",
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonACMEDomainNotResolvable,
			wantMessageIn: "does-not-resolve.invalid",
		},
		{
			name: "private acme trust bundle missing",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newACMECluster()
				cluster.Spec.Configuration = &openbaov1alpha1.OpenBaoConfiguration{
					ACMECARoot: "/etc/bao/seal-creds/ca.crt",
				}
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{
					Name: "seal-creds",
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonPrerequisitesMissing,
			wantMessageIn: "trust bundle is unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler := &OpenBaoClusterReconciler{
				Client:       fakeClient,
				Applications: newStatusTestApplications(fakeClient, scheme),
			}

			reconciler.setACMEIntegrationReadyCondition(context.Background(), tt.cluster)

			cond := meta.FindStatusCondition(tt.cluster.Status.Conditions, string(openbaov1alpha1.ConditionACMEIntegrationReady))
			if !tt.wantPresent {
				if cond != nil {
					t.Fatalf("expected ACMEIntegrationReady condition to be removed, got %#v", cond)
				}
				return
			}
			if cond == nil {
				t.Fatal("expected ACMEIntegrationReady condition")
			}
			if cond.Status != tt.wantStatus || cond.Reason != tt.wantReason {
				t.Fatalf("ACMEIntegrationReady = %#v, want status=%s reason=%s", cond, tt.wantStatus, tt.wantReason)
			}
			if tt.wantMessageIn != "" && !strings.Contains(cond.Message, tt.wantMessageIn) {
				t.Fatalf("message = %q, want substring %q", cond.Message, tt.wantMessageIn)
			}
		})
	}
}
