package raftops

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestClusterPodURLForService(t *testing.T) {
	require.Equal(t, "https://p1.svc1.ns1.svc:8200", ClusterPodURLForService("ns1", "svc1", "p1"))
}

func TestClusterPodURL(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = "c1"
	cluster.Namespace = "ns1"

	require.Equal(t, "https://p1.c1.ns1.svc:8200", ClusterPodURL(cluster, "p1"))
}
