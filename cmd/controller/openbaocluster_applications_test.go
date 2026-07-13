package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestOpenBaoClusterPodClientFactoryConfiguresPodEndpoint(t *testing.T) {
	baseConfig := portopenbao.ClientConfig{BaseURL: "https://unused.example"}
	var received portopenbao.ClientConfig
	factory := openBaoClusterPodClientFactory(
		nil,
		baseConfig,
		func(config portopenbao.ClientConfig) (portopenbao.ClusterActions, error) {
			received = config
			return nil, nil
		},
	)
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "tenant-a"},
	}

	_, err := factory(context.Background(), cluster, "example-2")
	require.NoError(t, err)
	assert.Equal(t, "https://example-2.example.tenant-a.svc:8200", received.BaseURL)
	assert.Empty(t, received.TLSServerName)
	assert.Empty(t, received.CACert)
	assert.Equal(t, "https://unused.example", baseConfig.BaseURL)
}

func TestOpenBaoClusterPodClientFactoryRequiresFactory(t *testing.T) {
	factory := openBaoClusterPodClientFactory(nil, portopenbao.ClientConfig{}, nil)

	_, err := factory(context.Background(), &openbaov1alpha1.OpenBaoCluster{}, "example-0")
	assert.EqualError(t, err, "OpenBao client factory is not configured")
}
