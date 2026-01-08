package bluegreen

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaoapi "github.com/dc-tec/openbao-operator/internal/openbao"
)

type ClusterOps interface {
	FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool)
}

type openBaoClusterOps struct {
	k8sClient     client.Client
	clientFactory OpenBaoClientFactory
}

func newOpenBaoClusterOps(k8sClient client.Client, clientFactory OpenBaoClientFactory) ClusterOps {
	return &openBaoClusterOps{
		k8sClient:     k8sClient,
		clientFactory: clientFactory,
	}
}

func (o *openBaoClusterOps) podURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	return fmt.Sprintf("https://%s.%s.%s.svc:%d", podName, cluster.Name, cluster.Namespace, constants.PortAPI)
}

func (o *openBaoClusterOps) clusterCACert(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) ([]byte, error) {
	for _, suffix := range []string{constants.SuffixTLSCA, constants.SuffixTLSServer} {
		secretName := cluster.Name + suffix
		secret := &corev1.Secret{}
		if err := o.k8sClient.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      secretName,
		}, secret); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, fmt.Errorf("failed to get CA secret %s/%s: %w", cluster.Namespace, secretName, err)
		}

		caCert, ok := secret.Data["ca.crt"]
		if !ok {
			return nil, fmt.Errorf("CA certificate not found in secret %s/%s", cluster.Namespace, secretName)
		}
		return caCert, nil
	}

	return nil, fmt.Errorf("no CA secret found for cluster %s/%s (tried %q and %q)", cluster.Namespace, cluster.Name, cluster.Name+constants.SuffixTLSCA, cluster.Name+constants.SuffixTLSServer)
}

func (o *openBaoClusterOps) FindLeaderPod(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, pods []corev1.Pod) (podName string, source string, ok bool) {
	for i := range pods {
		pod := &pods[i]
		if pod.DeletionTimestamp != nil {
			continue
		}

		active, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelActive)
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

		sealed, present, err := openbaoapi.ParseBoolLabel(pod.Labels, openbaoapi.LabelSealed)
		if err == nil && present && sealed {
			continue
		}

		apiClient, err := o.clientFactory(openbaoapi.ClientConfig{
			ClusterKey:          clusterKey,
			BaseURL:             o.podURL(cluster, pod.Name),
			CACert:              caCert,
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
