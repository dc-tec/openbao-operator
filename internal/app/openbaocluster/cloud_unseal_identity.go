package openbaocluster

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/workloadidentity"
)

func DescribeCloudUnsealIdentity(cluster *openbaov1alpha1.OpenBaoCluster) (workloadidentity.CloudUnsealIdentityDescription, bool) {
	return workloadidentity.DescribeCloudUnsealIdentity(cluster)
}

func AmbientCloudUnsealIdentityMessage(cluster *openbaov1alpha1.OpenBaoCluster) (string, bool) {
	description, ok := workloadidentity.DescribeCloudUnsealIdentity(cluster)
	if !ok || description.Mode != workloadidentity.CloudUnsealIdentityModeAmbient {
		return "", false
	}
	return description.Message, true
}

func EvaluateCloudUnsealIdentity(
	ctx context.Context,
	reader client.Reader,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (workloadidentity.CloudUnsealIdentityReadiness, bool, error) {
	return workloadidentity.EvaluateCloudUnsealIdentity(ctx, reader, cluster)
}
