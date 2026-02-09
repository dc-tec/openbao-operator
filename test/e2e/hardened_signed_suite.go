//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
)

const (
	envEnableHardenedSignedSuite = "E2E_ENABLE_HARDENED_SIGNED_SUITE"
	envOpenBaoImage              = "E2E_OPENBAO_IMAGE"
	envHardenedConfigInitImage   = "E2E_HARDENED_CONFIG_INIT_IMAGE"
)

func requireHardenedSignedSuite() {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(envEnableHardenedSignedSuite)), "true") {
		Skip(fmt.Sprintf(
			"requires signed hardened suite; set %s=true and provide %s and %s (for example ghcr.io/openbao/openbao:2.5.0 and ghcr.io/dc-tec/openbao-init:edge)",
			envEnableHardenedSignedSuite,
			envOpenBaoImage,
			envHardenedConfigInitImage,
		))
	}

	if strings.TrimSpace(os.Getenv(envOpenBaoImage)) == "" || strings.TrimSpace(os.Getenv(envHardenedConfigInitImage)) == "" {
		Fail(fmt.Sprintf(
			"%s=true requires both %s and %s to be set",
			envEnableHardenedSignedSuite,
			envOpenBaoImage,
			envHardenedConfigInitImage,
		))
	}
}
