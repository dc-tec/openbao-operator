package cluster

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
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

	readReplicaReplicas := readReplicaSANReplicaCount(cluster)
	if readReplicaReplicas > 0 {
		headlessServiceName := resourceidentity.HeadlessServiceName(cluster)
		readReplicaStatefulSetName := resourceidentity.ReadReplicaStatefulSetName(cluster)
		for i := int32(0); i < readReplicaReplicas; i++ {
			podName := fmt.Sprintf("%s-%d", readReplicaStatefulSetName, i)
			dnsNames = append(dnsNames,
				fmt.Sprintf("%s.%s.%s.svc", podName, headlessServiceName, namespace),
				fmt.Sprintf("%s.%s.%s.svc.cluster.local", podName, headlessServiceName, namespace),
			)
		}
	}

	if readReplicaServiceEnabled(cluster) {
		readServiceName := resourceidentity.ReadReplicaServiceName(cluster)
		dnsNames = append(dnsNames,
			fmt.Sprintf("%s.%s.svc", readServiceName, namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", readServiceName, namespace),
		)
	}

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

func readReplicaSANReplicaCount(cluster *openbaov1alpha1.OpenBaoCluster) int32 {
	var replicas int32
	if cluster.Spec.ReadReplicas != nil {
		replicas = cluster.Spec.ReadReplicas.Replicas
	}
	if cluster.Status.ReadReplicas != nil {
		if cluster.Status.ReadReplicas.DesiredReplicas > replicas {
			replicas = cluster.Status.ReadReplicas.DesiredReplicas
		}
		if cluster.Status.ReadReplicas.ReadyReplicas > replicas {
			replicas = cluster.Status.ReadReplicas.ReadyReplicas
		}
	}
	return replicas
}

func readReplicaServiceEnabled(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.ReadReplicas != nil &&
		cluster.Spec.ReadReplicas.Replicas > 0 &&
		cluster.Spec.ReadReplicas.Service != nil &&
		cluster.Spec.ReadReplicas.Service.Enabled
}
