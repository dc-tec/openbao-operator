package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

type ClusterOps interface {
	FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool)
}

type openBaoClusterOps struct {
	k8sClient     client.Client
	clientFactory upgrade.OpenBaoClientFactory
}

func newOpenBaoClusterOps(k8sClient client.Client, clientFactory upgrade.OpenBaoClientFactory) ClusterOps {
	return &openBaoClusterOps{
		k8sClient:     k8sClient,
		clientFactory: clientFactory,
	}
}

func (o *openBaoClusterOps) podURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	return upgrade.PodURL(cluster, podName)
}

func (o *openBaoClusterOps) clusterCACert(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	caCert, err := upgrade.LoadClusterCACert(ctx, o.k8sClient, cluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("cluster trust bundle not found: %w", err)
		}
		return nil, fmt.Errorf("failed to load cluster trust bundle: %w", err)
	}
	return caCert, nil
}

func (o *openBaoClusterOps) FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool) {
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}

		active, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelActive)
		if err != nil {
			logger.V(1).Info("Invalid OpenBao leader label value", "pod", pod.Name, "error", err)
			continue
		}
		if present && active {
			return pod.Name, "label", true
		}
	}

	caCert, err := o.clusterCACert(ctx, cluster)
	if err != nil {
		logger.V(1).Info("Failed to load cluster CA certificate; cannot use API leader fallback", "error", err)
		return "", "", false
	}

	clusterKey := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if !isPodReady(pod) {
			continue
		}

		sealed, present, err := portopenbao.ParseBoolLabel(pod.Labels, portopenbao.LabelSealed)
		if err == nil && present && sealed {
			continue
		}

		apiClient, err := o.clientFactory(portopenbao.ClientConfig{
			ClusterKey:          clusterKey,
			BaseURL:             o.podURL(cluster, pod.Name),
			CACert:              caCert,
			TLSServerName:       portopenbao.ComputeTLSServerName(cluster),
			ConnectionTimeout:   2 * time.Second,
			RequestTimeout:      2 * time.Second,
			SmartClientDisabled: true,
		})
		if err != nil {
			logger.V(1).Info("Failed to create OpenBao client for pod", "pod", pod.Name, "error", err)
			continue
		}

		isLeader, err := apiClient.IsLeader(ctx)
		if err != nil {
			logger.V(1).Info("Leader check failed for pod", "pod", pod.Name, "error", err)
			continue
		}
		if isLeader {
			return pod.Name, "api", true
		}
	}

	return "", "", false
}
