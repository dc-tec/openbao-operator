package workload

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
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

func int32Ptr(v int32) *int32 {
	return &v
}
