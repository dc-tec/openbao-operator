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

func TestSetBackupConfigurationReadyCondition(t *testing.T) {
	t.Parallel()

	scheme := newOpenBaoClusterTestScheme(t)

	newBackupCluster := func() *openbaov1alpha1.OpenBaoCluster {
		cluster := newOpenBaoClusterStatusTestObject()
		cluster.Spec.Profile = openbaov1alpha1.ProfileDevelopment
		cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{
			JWTAuthRole: "backup-role",
			Target: openbaov1alpha1.BackupTarget{
				Provider: "s3",
				Bucket:   "backups",
			},
		}
		return cluster
	}

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
			name: "no backup removes condition",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newOpenBaoClusterStatusTestObject()
				cluster.Status.Conditions = []metav1.Condition{{
					Type:   string(openbaov1alpha1.ConditionBackupConfigurationReady),
					Status: metav1.ConditionTrue,
				}}
				return cluster
			}(),
			wantPresent: false,
		},
		{
			name: "ambient identity assumption is visible",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				return newBackupCluster()
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonAmbientIdentityAssumed,
			wantMessageIn: "generated ServiceAccount",
		},
		{
			name: "explicit workload identity is visible",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/openbao-backup"
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    constants.ReasonWorkloadIdentityConfigured,
			wantMessageIn: "roleArn",
		},
		{
			name: "split identity surfaces are called out",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
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
			wantReason:    constants.ReasonAmbientIdentityAssumed,
			wantMessageIn: "do not inherit that identity automatically",
		},
		{
			name: "missing auth is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.SelfInit = nil
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonAuthenticationRequired,
			wantMessageIn: "configure jwtAuthRole or tokenSecretRef",
		},
		{
			name: "missing token secret is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "backup-token"}
				cluster.Spec.SelfInit = nil
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonTokenSecretMissing,
			wantMessageIn: "Backup token Secret",
		},
		{
			name: "hardened profile requires egress rules",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
				return cluster
			}(),
			wantPresent:   true,
			wantStatus:    metav1.ConditionFalse,
			wantReason:    constants.ReasonNetworkEgressRulesRequired,
			wantMessageIn: "spec.network.egressRules",
		},
		{
			name: "configured credentials secret reports ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "s3-creds"}
				return cluster
			}(),
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s3-creds", Namespace: "default"}},
			},
			wantPresent:   true,
			wantStatus:    metav1.ConditionTrue,
			wantReason:    "Ready",
			wantMessageIn: "Storage credentials Secret \"s3-creds\" is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithObjects(tt.objects...)
			}

			reconciler := &OpenBaoClusterReconciler{Client: builder.Build()}
			reconciler.setBackupConfigurationReadyCondition(context.Background(), tt.cluster)
			assertClusterCondition(
				t,
				tt.cluster,
				openbaov1alpha1.ConditionBackupConfigurationReady,
				tt.wantPresent,
				tt.wantStatus,
				tt.wantReason,
				tt.wantMessageIn,
			)
		})
	}
}
