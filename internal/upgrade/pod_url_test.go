package upgrade

import (
	"testing"

	"github.com/stretchr/testify/require"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestPodURLForService(t *testing.T) {
	require.Equal(t, "https://p1.svc1.ns1.svc:8200", PodURLForService("ns1", "svc1", "p1"))
}

func TestPodURL(t *testing.T) {
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	cluster.Name = "c1"
	cluster.Namespace = "ns1"

	require.Equal(t, "https://p1.c1.ns1.svc:8200", PodURL(cluster, "p1"))
}
