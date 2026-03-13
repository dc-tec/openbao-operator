package infra

import (
	"errors"
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

var ErrOIDCBootstrapAudienceMismatch = errors.New("oidc bootstrap audience mismatch")

func validateBootstrapAudience(cluster *openbaov1alpha1.OpenBaoCluster, operatorAudience string) error {
	if !portauth.OperatorJWTBootstrapEnabled(cluster) {
		return nil
	}

	if portauth.BootstrapAudienceMatchesInstallation(cluster, operatorAudience) {
		return nil
	}

	override := portauth.BootstrapAudienceOverride(cluster)
	effective := portauth.OperatorJWTAudience(operatorAudience)

	return operatorerrors.WrapPermanentConfig(
		fmt.Errorf(
			"%w: spec.selfInit.oidc.audience=%q does not match the operator installation audience %q; configure OPENBAO_JWT_AUDIENCE and the controller projected openbao-token audience at install time",
			ErrOIDCBootstrapAudienceMismatch,
			override,
			effective,
		),
	)
}
