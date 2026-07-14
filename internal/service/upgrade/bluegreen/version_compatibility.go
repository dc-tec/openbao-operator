package bluegreen

import (
	"fmt"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/service/upgrade"
)

func validateVersionCompatibility(currentVersion, targetVersion string) error {
	current, err := upgrade.ParseVersion(currentVersion)
	if err != nil {
		return nil
	}
	target, err := upgrade.ParseVersion(targetVersion)
	if err != nil {
		return nil
	}

	if !isPre26Version(current) || !is26OrNewerVersion(target) {
		return nil
	}

	return operatorerrors.WithReason(
		upgrade.ReasonBlueGreenVersionIncompatible,
		operatorerrors.WrapPermanentConfig(fmt.Errorf(
			upgrade.MessageBlueGreenVersionIncompatible,
			currentVersion,
			targetVersion,
		)),
	)
}

func isPre26Version(version *upgrade.SemVer) bool {
	if version == nil {
		return false
	}
	return version.Major < 2 || (version.Major == 2 && version.Minor < 6)
}

func is26OrNewerVersion(version *upgrade.SemVer) bool {
	// Fail closed until an OpenBao release is explicitly verified to preserve
	// mixed-version request-forwarding compatibility with pre-2.6 leaders.
	return version != nil &&
		(version.Major > 2 || (version.Major == 2 && version.Minor >= 6))
}
