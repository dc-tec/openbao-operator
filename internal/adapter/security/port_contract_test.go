package security

import (
	"testing"

	"github.com/kubebao/openbao-operator/internal/port/imageverify"
)

func TestImageVerifierSatisfiesVerifierPort(t *testing.T) {
	var _ imageverify.Verifier = (*ImageVerifier)(nil)
}
