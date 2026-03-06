package cluster

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// ComputeRequiredDNSSANs calculates DNS names that should be included in server certificate SANs
// based on cluster configuration and upgrade strategy status (e.g., Blue/Green revisions).
// This function extracts upgrade-strategy-specific logic from the certificate manager,
// allowing the cert manager to remain agnostic to upgrade strategy implementation details.
func ComputeRequiredDNSSANs(cluster *openbaov1alpha1.OpenBaoCluster) []string {
	if cluster == nil {
		return nil
	}

	namespace := cluster.Namespace
	clusterName := cluster.Name
	replicas := cluster.Spec.Replicas

	if namespace == "" || clusterName == "" {
		return nil
	}

	var dnsNames []string

	// For Blue/Green upgrades, explicitly add SANs for the revision-specific pod names.
	// Wildcards like *.bluegreen-cluster.svc work for standard pods, but for Green pods
	// like bluegreen-cluster-hash-0, we want to be explicit to ensure TLS validation works.
	if cluster.Status.BlueGreen != nil {
		revisions := []string{}
		if cluster.Status.BlueGreen.BlueRevision != "" {
			revisions = append(revisions, cluster.Status.BlueGreen.BlueRevision)
		}
		if cluster.Status.BlueGreen.GreenRevision != "" {
			revisions = append(revisions, cluster.Status.BlueGreen.GreenRevision)
		}

		for _, rev := range revisions {
			for i := int32(0); i < replicas; i++ {
				podName := fmt.Sprintf("%s-%s-%d", clusterName, rev, i)
				dnsNames = append(dnsNames,
					fmt.Sprintf("%s.%s.%s.svc", podName, clusterName, namespace),
					fmt.Sprintf("%s.%s.%s.svc.cluster.local", podName, clusterName, namespace),
				)
			}
		}
	}

	return dnsNames
}
