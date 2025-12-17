//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"
)

const defaultOpenBaoVersion = "2.4.4"

// defaultUpgradeFromVersion is the version to start with for upgrade tests.
// This should be set to a stable, known-good version.
const defaultUpgradeFromVersion = "2.4.3"

// defaultUpgradeToVersion is the target version for upgrade tests.
// This should be set to a newer version than defaultUpgradeFromVersion.
// Update this when new OpenBao versions are released.
const defaultUpgradeToVersion = "2.4.4"

var (
	openBaoVersion string
	openBaoImage   string
)

// kindDefaultServiceCIDR is the default service CIDR used by kind clusters.
// This is used in e2e tests to configure spec.network.apiServerCIDR to avoid
// requiring permissions to read the kubernetes service in the default namespace.
const kindDefaultServiceCIDR = "10.96.0.0/12"

func init() {
	openBaoVersion = strings.TrimSpace(os.Getenv("E2E_OPENBAO_VERSION"))
	if openBaoVersion == "" {
		openBaoVersion = defaultOpenBaoVersion
	}

	openBaoImage = strings.TrimSpace(os.Getenv("E2E_OPENBAO_IMAGE"))
	if openBaoImage == "" {
		openBaoImage = fmt.Sprintf("openbao/openbao:%s", openBaoVersion)
	}
}
