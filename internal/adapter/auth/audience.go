package auth

import (
	"os"
	"strings"

	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

const (
	envOpenBaoJWTAudience = "OPENBAO_JWT_AUDIENCE"
)

// OpenBaoJWTAudience returns the configured JWT audience for OpenBao auth tokens.
// Defaults to portauth.TokenAudienceOpenBaoInternal when unset.
func OpenBaoJWTAudience() string {
	return portauth.OperatorJWTAudience(strings.TrimSpace(os.Getenv(envOpenBaoJWTAudience)))
}
