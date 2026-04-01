package bluegreen

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade/raftops"
)

type ClusterOps interface {
	FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool)
}

type openBaoClusterOps struct {
	k8sClient     client.Client
	clientFactory raftops.OpenBaoClientFactory
}

func newOpenBaoClusterOps(k8sClient client.Client, clientFactory raftops.OpenBaoClientFactory) ClusterOps {
	return &openBaoClusterOps{
		k8sClient:     k8sClient,
		clientFactory: clientFactory,
	}
}

func (o *openBaoClusterOps) FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool) {
	return raftops.FindClusterLeaderPod(ctx, logger, o.k8sClient, o.clientFactory, cluster, pods, raftops.ClusterPodClientOptions{
		ConnectionTimeout:   2 * time.Second,
		RequestTimeout:      2 * time.Second,
		SmartClientDisabled: true,
	})
}
