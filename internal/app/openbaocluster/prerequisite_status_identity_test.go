package openbaocluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestEvaluateBackupConfigurationConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)

	newBackupCluster := func() *openbaov1alpha1.OpenBaoCluster {
		cluster := newPrerequisiteStatusTestCluster()
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
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		objects     []client.Object
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name:        "ambient identity assumption is visible",
			cluster:     newBackupCluster(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonAmbientIdentityAssumed,
			wantMessage: "generated ServiceAccount",
		},
		{
			name: "explicit workload identity is visible",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/openbao-backup"
				return cluster
			}(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonWorkloadIdentityConfigured,
			wantMessage: "roleArn",
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
				cluster.Spec.ServiceAccount = &openbaov1alpha1.ServiceAccountConfig{Annotations: map[string]string{
					"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao",
				}}
				return cluster
			}(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonAmbientIdentityAssumed,
			wantMessage: "do not inherit that identity automatically",
		},
		{
			name: "missing authentication is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.SelfInit = nil
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonAuthenticationRequired,
			wantMessage: "configure jwtAuthRole or tokenSecretRef",
		},
		{
			name: "missing token Secret is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.JWTAuthRole = ""
				cluster.Spec.Backup.TokenSecretRef = &corev1.LocalObjectReference{Name: "backup-token"}
				cluster.Spec.SelfInit = nil
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonTokenSecretMissing,
			wantMessage: "Backup token Secret",
		},
		{
			name: "Hardened profile requires egress rules",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Profile = openbaov1alpha1.ProfileHardened
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonNetworkEgressRulesRequired,
			wantMessage: "spec.network.egressRules",
		},
		{
			name: "configured credentials Secret reports ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newBackupCluster()
				cluster.Spec.Backup.Target.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "s3-creds"}
				return cluster
			}(),
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s3-creds", Namespace: "default"}},
			},
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "Ready",
			wantMessage: "Storage credentials Secret \"s3-creds\" is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...)
			result, err := EvaluateBackupConfiguration(t.Context(), builder.Build(), tt.cluster)
			if err != nil {
				t.Fatalf("EvaluateBackupConfiguration() error = %v", err)
			}
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestEvaluateCloudUnsealIdentityConditionPolicy(t *testing.T) {
	scheme := newPrerequisiteStatusTestScheme(t)

	tests := []struct {
		name        string
		cluster     *openbaov1alpha1.OpenBaoCluster
		objects     []client.Object
		wantStatus  metav1.ConditionStatus
		wantReason  string
		wantMessage string
	}{
		{
			name: "ambient AWS identity is surfaced",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newAWSUnsealPrerequisiteCluster()
				cluster.Spec.ServiceAccount = &openbaov1alpha1.ServiceAccountConfig{Annotations: map[string]string{
					"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao",
				}}
				return cluster
			}(),
			wantStatus:  metav1.ConditionTrue,
			wantReason:  constants.ReasonWorkloadIdentityConfigured,
			wantMessage: "standard AWS credential chain",
		},
		{
			name: "missing Secret is false",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newAWSUnsealPrerequisiteCluster()
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "aws-creds"}
				return cluster
			}(),
			wantStatus:  metav1.ConditionFalse,
			wantReason:  constants.ReasonCredentialsSecretMissing,
			wantMessage: "aws-creds",
		},
		{
			name: "Secret-backed configuration reports ready",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newAWSUnsealPrerequisiteCluster()
				cluster.Spec.Unseal.CredentialsSecretRef = &corev1.LocalObjectReference{Name: "aws-creds"}
				return cluster
			}(),
			objects: []client.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "aws-creds", Namespace: "default"}},
			},
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "Ready",
			wantMessage: "credentials Secret default/aws-creds",
		},
		{
			name: "inline GCP credentials are explicit",
			cluster: func() *openbaov1alpha1.OpenBaoCluster {
				cluster := newPrerequisiteStatusTestCluster()
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
			wantStatus:  metav1.ConditionTrue,
			wantReason:  "Ready",
			wantMessage: "operator only projects it automatically",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...)
			result, applicable, err := EvaluateCloudUnsealIdentity(t.Context(), builder.Build(), tt.cluster)
			if err != nil {
				t.Fatalf("EvaluateCloudUnsealIdentity() error = %v", err)
			}
			if !applicable {
				t.Fatal("EvaluateCloudUnsealIdentity() applicable = false, want true")
			}
			if result.Status != tt.wantStatus || result.Reason != tt.wantReason {
				t.Errorf("result = status %s, reason %q; want status %s, reason %q", result.Status, result.Reason, tt.wantStatus, tt.wantReason)
			}
			if !contains(result.Message, tt.wantMessage) {
				t.Errorf("message = %q, want substring %q", result.Message, tt.wantMessage)
			}
		})
	}
}

func newAWSUnsealPrerequisiteCluster() *openbaov1alpha1.OpenBaoCluster {
	cluster := newPrerequisiteStatusTestCluster()
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "awskms",
		AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
			Region:   "eu-central-1",
			KMSKeyID: "alias/openbao",
		},
	}
	return cluster
}
