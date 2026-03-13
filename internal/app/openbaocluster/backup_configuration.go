package openbaocluster

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/workloadidentity"
)

type BackupConfigurationResult struct {
	Status  metav1.ConditionStatus
	Reason  string
	Message string
}

func EvaluateBackupConfiguration(ctx context.Context, reader client.Reader, cluster *openbaov1alpha1.OpenBaoCluster) (BackupConfigurationResult, error) {
	readiness, err := workloadidentity.EvaluateBackupReadiness(ctx, reader, cluster)
	if err != nil {
		return BackupConfigurationResult{}, err
	}

	return BackupConfigurationResult{
		Status:  readiness.Status,
		Reason:  readiness.Reason,
		Message: readiness.Message,
	}, nil
}
