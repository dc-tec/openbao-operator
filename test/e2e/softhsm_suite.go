//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"

	"github.com/onsi/ginkgo/v2"
)

const (
	envEnableSoftHSMSuite = "E2E_ENABLE_SOFTHSM_SUITE"
)

func requireSoftHSMSuite() {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(envEnableSoftHSMSuite)), "true") {
		ginkgo.Skip(fmt.Sprintf(
			"requires SoftHSM PKCS#11 suite; build the fixture with "+
				"`make docker-build-e2e-openbao-softhsm OPENBAO_SOFTHSM_IMG=openbao-softhsm:dev` "+
				"and run with %s=true E2E_OPENBAO_IMAGE=openbao-softhsm:dev",
			envEnableSoftHSMSuite,
		))
	}

	if strings.TrimSpace(os.Getenv(envOpenBaoImage)) == "" {
		ginkgo.Fail(fmt.Sprintf("%s=true requires %s to point at the SoftHSM-enabled OpenBao image", envEnableSoftHSMSuite, envOpenBaoImage))
	}
}
