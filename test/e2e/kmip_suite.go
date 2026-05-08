//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/onsi/ginkgo/v2"

	"github.com/dc-tec/openbao-operator/test/utils"
)

const (
	envEnableKMIPSuite = "E2E_ENABLE_KMIP_SUITE"
	envKMIPServerImage = "E2E_KMIP_SERVER_IMAGE"

	defaultKMIPServerImage = "pykmip-server:dev"
)

func requireKMIPSuite() {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(envEnableKMIPSuite)), "true") {
		ginkgo.Skip(fmt.Sprintf(
			"requires KMIP suite; build the fixture with "+
				"`make docker-build-e2e-pykmip-server PYKMIP_SERVER_IMG=%s` "+
				"and run with %s=true %s=%s",
			defaultKMIPServerImage,
			envEnableKMIPSuite,
			envKMIPServerImage,
			defaultKMIPServerImage,
		))
	}
}

func kmipServerImage() string {
	if image := strings.TrimSpace(os.Getenv(envKMIPServerImage)); image != "" {
		return image
	}
	return defaultKMIPServerImage
}

func ensureKMIPServerImageLoaded() {
	if useExistingCluster {
		return
	}

	image := kmipServerImage()
	if _, err := utils.Run(exec.Command("docker", "image", "inspect", image)); err != nil { // #nosec G204 -- test harness
		_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Pulling KMIP server fixture image %q...\n", image)
		_, err = utils.Run(exec.Command("docker", "pull", image)) // #nosec G204 -- test harness
		if err != nil {
			ginkgo.Fail(fmt.Sprintf(
				"KMIP server fixture image %q is not available locally or remotely; build it with `make docker-build-e2e-pykmip-server PYKMIP_SERVER_IMG=%s`",
				image,
				image,
			))
		}
	}

	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Loading KMIP server fixture image %q into kind\n", image)
	if err := utils.LoadImageToKindClusterWithName(image); err != nil {
		ginkgo.Fail(fmt.Sprintf("failed to load KMIP server fixture image %q into kind: %v", image, err))
	}
}
