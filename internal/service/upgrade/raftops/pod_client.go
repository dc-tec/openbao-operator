package raftops

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ClusterPodClientOptions tunes controller-side OpenBao pod client behavior.
type ClusterPodClientOptions struct {
	ConnectionTimeout   time.Duration
	RequestTimeout      time.Duration
	SmartClientDisabled bool
}

// NewClusterPodClient builds an OpenBao API client targeting a specific cluster pod.
func NewClusterPodClient(
	cluster *openbaov1alpha1.OpenBaoCluster,
	podName string,
	caCert []byte,
	clientFactory OpenBaoClientFactory,
	opts ClusterPodClientOptions,
) (portopenbao.ClusterActions, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is required")
	}
	if podName == "" {
		return nil, fmt.Errorf("pod name is required")
	}
	if clientFactory == nil {
		clientFactory = DefaultOpenBaoClientFactory
	}

	apiClient, err := clientFactory(portopenbao.ClientConfig{
		ClusterKey:          fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name),
		BaseURL:             ClusterPodURL(cluster, podName),
		CACert:              caCert,
		TLSServerName:       portopenbao.ComputeTLSServerName(cluster),
		ConnectionTimeout:   opts.ConnectionTimeout,
		RequestTimeout:      opts.RequestTimeout,
		SmartClientDisabled: opts.SmartClientDisabled,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenBao client for pod %s: %w", podName, err)
	}
	return apiClient, nil
}

// IsClusterPodLeader checks whether a specific cluster pod currently serves as leader.
func IsClusterPodLeader(
	ctx context.Context,
	k8sClient client.Client,
	clientFactory OpenBaoClientFactory,
	cluster *openbaov1alpha1.OpenBaoCluster,
	podName string,
	opts ClusterPodClientOptions,
) (bool, error) {
	caCert, err := LoadClusterCACert(ctx, k8sClient, cluster)
	if err != nil {
		return false, fmt.Errorf("failed to load cluster trust bundle: %w", err)
	}

	apiClient, err := NewClusterPodClient(cluster, podName, caCert, clientFactory, opts)
	if err != nil {
		return false, err
	}

	isLeader, err := apiClient.IsLeader(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to check leadership for pod %s: %w", podName, err)
	}

	return isLeader, nil
}

// FindClusterLeaderPod first trusts an unambiguous leader label and then falls
// back to direct API probing across eligible pods.
func FindClusterLeaderPod(
	ctx context.Context,
	logger logr.Logger,
	k8sClient client.Client,
	clientFactory OpenBaoClientFactory,
	cluster *openbaov1alpha1.OpenBaoCluster,
	pods []corev1.Pod,
	opts ClusterPodClientOptions,
) (podName string, source string, ok bool) {
	leaderPod, found, err := FindLeaderPodByLabel(pods)
	if err != nil {
		logger.V(1).Info("Unable to determine leader from pod labels; falling back to API leader check", "error", err)
	} else if found {
		return leaderPod, "label", true
	}

	caCert, err := LoadClusterCACert(ctx, k8sClient, cluster)
	if err != nil {
		logger.V(1).Info("Failed to load cluster CA certificate; cannot use API leader fallback", "error", err)
		return "", "", false
	}

	leaderPod, found = ProbeLeaderPod(ctx, logger, pods, func(ctx context.Context, pod *corev1.Pod) (bool, error) {
		apiClient, err := NewClusterPodClient(cluster, pod.Name, caCert, clientFactory, opts)
		if err != nil {
			return false, err
		}
		return apiClient.IsLeader(ctx)
	})
	if found {
		return leaderPod, "api", true
	}

	return "", "", false
}
