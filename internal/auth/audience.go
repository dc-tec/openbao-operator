package auth

import (
	"os"
	"strings"
)

const (
	envOpenBaoJWTAudience        = "OPENBAO_JWT_AUDIENCE"
	tokenAudienceOpenBaoInternal = "openbao-internal"
)

// OpenBaoJWTAudience returns the configured JWT audience for OpenBao auth tokens.
// Defaults to tokenAudienceOpenBaoInternal when unset.
func OpenBaoJWTAudience() string {
	raw := strings.TrimSpace(os.Getenv(envOpenBaoJWTAudience))
	if raw == "" {
		return tokenAudienceOpenBaoInternal
	}
	return raw
}
