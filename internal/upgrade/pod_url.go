package upgrade

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// PodURLForService returns the OpenBao API base URL for the given pod name via a headless Service.
func PodURLForService(namespace, serviceName, podName string) string {
	return fmt.Sprintf("https://%s.%s.%s.svc:%d", podName, serviceName, namespace, constants.PortAPI)
}

// PodURL returns the OpenBao API base URL for the given pod name via the cluster's headless Service.
func PodURL(cluster *openbaov1alpha1.OpenBaoCluster, podName string) string {
	return PodURLForService(cluster.Namespace, cluster.Name, podName)
}
