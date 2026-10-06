package restore

import (
	"context"
	"fmt"
	"time"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func readTargetHealth(ctx context.Context, config portopenbao.ClientConfig) (*portopenbao.HealthStatus, error) {
	bao, err := openbao.NewClient(config)
	if err != nil {
		return nil, err
	}
	return bao.Health(ctx)
}

// targetHealth reads every expected voter directly. Health permits management
// resumption; it is never used as generic snapshot-application evidence.
func (m *Manager) targetHealth(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (*portopenbao.HealthStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	caCert, err := openbaotls.LoadClusterTrustBundle(ctx, m.reader, cluster)
	if err != nil {
		return nil, err
	}

	config := m.clientConfig
	config.CACert = caCert
	config.TLSServerName = portopenbao.ComputeTLSServerName(cluster)

	var leader *portopenbao.HealthStatus
	for i := int32(0); i < cluster.Spec.Replicas; i++ {
		config.BaseURL = fmt.Sprintf("https://%s-%d.%s.%s.svc:8200", restoreTargetStatefulSetName(cluster), i, cluster.Name, cluster.Namespace)
		health, err := m.readHealth(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("read target voter %d health: %w", i, err)
		}
		if !health.Initialized || health.Sealed {
			return nil, fmt.Errorf("target voter is not initialized and unsealed")
		}
		if !health.Standby && !health.PerformanceStandby {
			if leader != nil {
				return nil, fmt.Errorf("target reports multiple active voters")
			}
			leader = health
		}
	}

	if leader == nil {
		return nil, fmt.Errorf("target has no healthy active voter")
	}

	return leader, nil
}
