package raftops

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// ClusterPodURLForService returns the OpenBao API base URL for the given pod
// name via a headless Service.
func ClusterPodURLForService(namespace, serviceName, podName string) string {
	return fmt.Sprintf("https://%s.%s.%s.svc:%d", podName, serviceName, namespace, constants.PortAPI)
}

// ClusterPodURL returns the OpenBao API base URL for the given pod name via
// the cluster's headless Service.
func ClusterPodURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	return ClusterPodURLForService(cluster.Namespace, cluster.Name, podName)
}
