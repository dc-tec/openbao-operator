package workload

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

func TestGetInitContainerImage_DefaultImageConfigurationError(t *testing.T) {
	t.Setenv(constants.EnvOperatorVersion, "")

	cluster := newMinimalCluster("init-image-default", "default")
	cluster.Spec.InitContainer = &openbaov1alpha1.InitContainerConfig{}

	_, err := ResolveInitContainerImage(cluster)
	if err == nil {
		t.Fatal("getInitContainerImage() error = nil, want error")
	}

	if reason, ok := operatorerrors.Reason(err); !ok || reason != constants.ReasonHelperImageConfigurationInvalid {
		t.Fatalf("reason = %q,%v want %q,true", reason, ok, constants.ReasonHelperImageConfigurationInvalid)
	}
}
