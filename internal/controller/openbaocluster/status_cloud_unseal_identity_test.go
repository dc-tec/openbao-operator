package openbaocluster

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestSetCloudUnsealIdentityReadyCondition(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	tests := []struct {
		name          string
		cluster       *openbaov1alpha1.OpenBaoCluster
		objects       []client.Object
		wantPresent   bool
		wantStatus    metav1.ConditionStatus
		wantReason    string
		wantMessageIn string
	}{
		{
			name: "non cloud unseal removes condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Status.Conditions = []metav1.Condition{{
					Type:   string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
					Status: metav1.ConditionTrue,
				}}
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: "transit"}
				return cluster
			}(),
			wantPresent: false,
		},
		{
			name: "ambient aws identity is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "awskms",
					AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
						Region:   "eu-central-1",
						KMSKeyID: "alias/openbao",
					},
				}
				cluster.Spec.ServiceAccount = &openbaov1alpha1.ServiceAccountConfig{
					Annotations: map[string]string{
						"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao",
					},
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonWorkloadIdentityConfigured,
			wantMessageIn: "standard AWS credential chain",
		},
		{
			name: "missing secret becomes false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "awskms",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
					AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
						Region:   "eu-central-1",
						KMSKeyID: "alias/openbao",
					},
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonCredentialsSecretMissing,
			wantMessageIn: "aws-creds",
		},
		{
			name: "secret-backed config reports ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "awskms",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
					AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
						Region:   "eu-central-1",
						KMSKeyID: "alias/openbao",
					},
				}
				return cluster
			}(),
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "aws-creds", Namespace: "default"}},
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    "Ready",
			wantMessageIn: "credentials Secret default/aws-creds",
		},
		{
			name: "inline gcp credentials are explicit not ambient",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
					Type: "gcpckms",
					GCPCloudKMS: &openbaov1alpha1.GCPCloudKMSSealConfig{
						Project:     "proj",
						Region:      "europe-west1",
						KeyRing:     "ring",
						CryptoKey:   "key",
						Credentials: "/etc/gcp/creds.json",
					},
				}
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    "Ready",
			wantMessageIn: "operator only projects it automatically",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}
			reconciler.setCloudUnsealIdentityReadyCondition(context.Background(), tt.cluster)
			assertClusterCondition(
				t,
				tt.cluster,
				openbaov1alpha1.ConditionCloudUnsealIdentityReady,
				tt.wantPresent,
				tt.wantStatus,
				tt.wantReason,
				tt.wantMessageIn,
			)
		})
	}
}
