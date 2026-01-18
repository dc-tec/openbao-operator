package auth

import (
	"os"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

// OpenBaoJWTAudience returns the configured JWT audience for OpenBao auth tokens.
// Defaults to constants.TokenAudienceOpenBaoInternal when unset.
func OpenBaoJWTAudience() string {
	raw := strings.TrimSpace(os.Getenv(constants.EnvOpenBaoJWTAudience))
	if raw == "" {
		return constants.TokenAudienceOpenBaoInternal
	}
	return raw
}
