//go:build integration
// +build integration

package infra

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func tlsServerSecretName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + constants.SuffixTLSServer
}

func acmeServiceName(cluster *openbaov1alpha1.OpenBaoCluster) string {
	return cluster.Name + "-acme"
}
