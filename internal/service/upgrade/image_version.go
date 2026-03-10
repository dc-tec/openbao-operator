package upgrade

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// ValidateImageRefMatchesVersion rejects image selections that can be proven
// incompatible with spec.version while still allowing digest-pinned images and
// custom non-semver tags.
func ValidateImageRefMatchesVersion(version, imageRef string) error {
	imageRef = strings.TrimSpace(imageRef)
	if imageRef == "" {
		return nil
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return operatorerrors.WithReason(
			ReasonImageVersionMismatch,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(MessageInvalidImageReference+": %w", imageRef, err)),
		)
	}

	if _, ok := ref.(name.Digest); ok {
		return nil
	}

	tagRef, ok := ref.(name.Tag)
	if !ok {
		return nil
	}

	imageVersion, err := ParseVersion(tagRef.TagStr())
	if err != nil {
		// Custom non-semver tags remain allowed in this release-hardening pass.
		return nil
	}

	targetVersion, err := ParseVersion(version)
	if err != nil {
		return operatorerrors.WithReason(
			ReasonInvalidVersion,
			operatorerrors.WrapPermanentConfig(fmt.Errorf(MessageInvalidVersion+": %w", version, err)),
		)
	}

	if imageVersion.String() == targetVersion.String() {
		return nil
	}

	return operatorerrors.WithReason(
		ReasonImageVersionMismatch,
		operatorerrors.WrapPermanentConfig(fmt.Errorf(MessageImageVersionMismatch, tagRef.TagStr(), version)),
	)
}
