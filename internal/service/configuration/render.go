package configuration

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// RenderOptions controls how the shared config.hcl render contract is specialized
// for callers such as blue/green upgrades.
type RenderOptions struct {
	TargetRevisionForJoin  string
	RetryJoinLabelSelector string
	RetryJoinAsNonVoter    bool
}

// Render renders the operator-managed config.hcl for an OpenBaoCluster.
func Render(cluster *openbaov1alpha1.OpenBaoCluster, opts RenderOptions) (string, error) {
	renderedConfig, err := configbuilder.RenderHCL(cluster, configbuilder.InfrastructureDetails{
		HeadlessServiceName:    cluster.Name,
		Namespace:              cluster.Namespace,
		APIPort:                constants.PortAPI,
		ClusterPort:            constants.PortCluster,
		TargetRevisionForJoin:  opts.TargetRevisionForJoin,
		RetryJoinLabelSelector: opts.RetryJoinLabelSelector,
		RetryJoinAsNonVoter:    opts.RetryJoinAsNonVoter,
	})
	if err != nil {
		return "", fmt.Errorf("render config.hcl: %w", err)
	}

	return string(renderedConfig), nil
}
