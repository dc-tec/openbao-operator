package infra

import (
	"context"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// BlueGreenRuntime defines infrastructure operations needed by the blue/green strategy manager.
type BlueGreenRuntime interface {
	EnsureBlueGreenStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster)
	EnsureStatefulSetWithRevision(
		ctx context.Context,
		logger logr.Logger,
		cluster *openbaov1alpha1.OpenBaoCluster,
		configContent string,
		verifiedImageDigest string,
		verifiedInitContainerDigest string,
		revision string,
		disableSelfInit bool,
	) error
}
