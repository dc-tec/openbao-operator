package workload

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// ResolveInitContainerImage returns the init container image to use.
// If not specified in the cluster spec, it returns the default image derived from
// the operator runtime configuration.
func ResolveInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	if cluster.Spec.InitContainer != nil && cluster.Spec.InitContainer.Image != "" {
		return cluster.Spec.InitContainer.Image, nil
	}

	image, err := constants.DefaultInitImage()
	if err != nil {
		return "", operatorerrors.WrapPermanentConfig(operatorerrors.WithReason(
			constants.ReasonHelperImageConfigurationInvalid,
			fmt.Errorf(
				"default init container image is unavailable; set spec.initContainer.image explicitly or configure OPERATOR_VERSION in the operator Deployment: %w",
				err,
			),
		))
	}

	return image, nil
}
