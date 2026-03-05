package auth

import (
	"os"
	"strings"
)

const (
	envOpenBaoJWTAudience = "OPENBAO_JWT_AUDIENCE"

	// TokenAudienceOpenBaoInternal is the Kubernetes projected ServiceAccount token
	// audience used for OpenBao JWT authentication.
	TokenAudienceOpenBaoInternal = "openbao-internal"
)

// OpenBaoJWTAudience returns the configured JWT audience for OpenBao auth tokens.
// Defaults to TokenAudienceOpenBaoInternal when unset.
func OpenBaoJWTAudience() string {
	raw := strings.TrimSpace(os.Getenv(envOpenBaoJWTAudience))
	if raw == "" {
		return TokenAudienceOpenBaoInternal
	}
	return raw
}
