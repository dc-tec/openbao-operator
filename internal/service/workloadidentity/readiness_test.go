package workloadidentity

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func testReadinessScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add openbao scheme: %v", err)
	}
	return scheme
}

func TestEvaluateBackupReadiness(t *testing.T) {
	scheme := testReadinessScheme(t)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
			Backup: &openbaov1alpha1.BackupSchedule{
				JWTAuthRole: "backup-role",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Bucket:   "backups",
				},
			},
		},
	}

	reader := fake.NewClientBuilder().WithScheme(scheme).Build()
	readiness, err := EvaluateBackupReadiness(context.Background(), reader, cluster)
	if err != nil {
		t.Fatalf("EvaluateBackupReadiness() error = %v", err)
	}
	if readiness.Status != metav1.ConditionTrue || readiness.Reason != constants.ReasonAmbientIdentityAssumed {
		t.Fatalf("readiness = %#v, want true/%s", readiness, constants.ReasonAmbientIdentityAssumed)
	}
	if !strings.Contains(readiness.Message, "generated ServiceAccount") {
		t.Fatalf("message = %q, want generated ServiceAccount guidance", readiness.Message)
	}
	if !strings.Contains(readiness.FailureHint, "ServiceAccount") {
		t.Fatalf("failure hint = %q, want ServiceAccount guidance", readiness.FailureHint)
	}
}

func TestEvaluateBackupReadiness_ExplicitWorkloadIdentityIsNotAmbient(t *testing.T) {
	scheme := testReadinessScheme(t)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
			Backup: &openbaov1alpha1.BackupSchedule{
				JWTAuthRole: "backup-role",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Bucket:   "backups",
					RoleARN:  "arn:aws:iam::123456789012:role/openbao-backup",
				},
			},
		},
	}

	reader := fake.NewClientBuilder().WithScheme(scheme).Build()
	readiness, err := EvaluateBackupReadiness(context.Background(), reader, cluster)
	if err != nil {
		t.Fatalf("EvaluateBackupReadiness() error = %v", err)
	}
	if readiness.Status != metav1.ConditionTrue || readiness.Reason != constants.ReasonWorkloadIdentityConfigured {
		t.Fatalf("readiness = %#v, want true/%s", readiness, constants.ReasonWorkloadIdentityConfigured)
	}
	if !strings.Contains(readiness.Message, "roleArn") {
		t.Fatalf("message = %q, want roleArn guidance", readiness.Message)
	}
}

func TestEvaluateBackupReadiness_CrossSurfaceIdentityHint(t *testing.T) {
	scheme := testReadinessScheme(t)

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileDevelopment,
			Unseal: &openbaov1alpha1.UnsealConfig{
				Type: "awskms",
				AWSKMS: &openbaov1alpha1.AWSKMSSealConfig{
					Region:   "eu-central-1",
					KMSKeyID: "alias/openbao",
				},
			},
			ServiceAccount: &openbaov1alpha1.ServiceAccountConfig{
				Annotations: map[string]string{
					"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/openbao",
				},
			},
			Backup: &openbaov1alpha1.BackupSchedule{
				JWTAuthRole: "backup-role",
				Target: openbaov1alpha1.BackupTarget{
					Provider: "s3",
					Bucket:   "backups",
				},
			},
		},
	}

	reader := fake.NewClientBuilder().WithScheme(scheme).Build()
	readiness, err := EvaluateBackupReadiness(context.Background(), reader, cluster)
	if err != nil {
		t.Fatalf("EvaluateBackupReadiness() error = %v", err)
	}
	if !strings.Contains(readiness.Message, "do not inherit that identity automatically") {
		t.Fatalf("message = %q, want split identity guidance", readiness.Message)
	}
}

func TestEvaluateExecutionReadiness_MissingCredentialsSecret(t *testing.T) {
	scheme := testReadinessScheme(t)
	reader := fake.NewClientBuilder().WithScheme(scheme).Build()

	readiness, err := EvaluateExecutionReadiness(context.Background(), reader, Input{
		Operation:          OperationRestore,
		Namespace:          "default",
		ServiceAccountName: "demo-restore-serviceaccount",
		JWTAuthRole:        "restore-role",
		Target: openbaov1alpha1.BackupTarget{
			Provider:             "s3",
			Bucket:               "backups",
			CredentialsSecretRef: &corev1.LocalObjectReference{Name: "missing-creds"},
		},
	})
	if err != nil {
		t.Fatalf("EvaluateExecutionReadiness() error = %v", err)
	}
	if readiness.Status != metav1.ConditionFalse || readiness.Reason != constants.ReasonCredentialsSecretMissing {
		t.Fatalf("readiness = %#v, want false/%s", readiness, constants.ReasonCredentialsSecretMissing)
	}
	if !strings.Contains(readiness.Message, "missing-creds") {
		t.Fatalf("message = %q, want missing Secret name", readiness.Message)
	}
}
