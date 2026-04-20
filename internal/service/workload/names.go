package workload

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

func statefulSetNameWithRevision(cluster *openbaov1alpha1.OpenBaoCluster, rev string) string {
	if rev == "" {
		return cluster.Name
	}
	return fmt.Sprintf("%s-%s", cluster.Name, rev)
}

func usesStaticSeal(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	if cluster.Spec.Unseal == nil {
		return true
	}
	if cluster.Spec.Unseal.Type == "" {
		return true
	}
	return cluster.Spec.Unseal.Type == "static"
}

func statefulSetNameForSpec(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) string {
	if spec.Name != "" {
		return spec.Name
	}
	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolReadReplica {
		return resourceidentity.ReadReplicaStatefulSetName(cluster)
	}
	return statefulSetNameWithRevision(cluster, spec.Revision)
}

func configMapNameForSpec(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) string {
	return resourceidentity.ConfigMapNameForPoolWithRevision(cluster, spec.Pool, spec.Revision)
}

func int32Ptr(v int32) *int32 {
	return &v
}
