package backup

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// loadTLSConfig loads the TLS CA certificate from a file.
func loadTLSConfig(cfg *ExecutorConfig) error {
	if envPath, ok := os.LookupEnv(constants.EnvTLSCAPath); ok {
		caCertPath := strings.TrimSpace(envPath)
		if caCertPath != "" {
			caCert, err := os.ReadFile(caCertPath) // #nosec G304 -- Path from environment variable
			if err != nil {
				return fmt.Errorf("failed to read TLS CA certificate from %q: %w", caCertPath, err)
			}
			cfg.TLSCACert = caCert
		}
	} else {
		caCert, err := os.ReadFile(constants.PathTLSCACert) // #nosec G304 -- Constant path
		if err == nil {
			cfg.TLSCACert = caCert
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to read TLS CA certificate from %q: %w", constants.PathTLSCACert, err)
		}
	}

	if skipVerifyStr := strings.TrimSpace(os.Getenv(constants.EnvBackupInsecureSkipVerify)); skipVerifyStr != "" {
		skipVerify, err := strconv.ParseBool(skipVerifyStr)
		if err != nil {
			return fmt.Errorf("invalid %s value %q: %w", constants.EnvBackupInsecureSkipVerify, skipVerifyStr, err)
		}
		cfg.InsecureSkipVerify = skipVerify
	}
	return nil
}

// loadAuthConfig loads authentication configuration (JWT or static token).
func loadAuthConfig(cfg *ExecutorConfig) error {
	cfg.AuthMethod = strings.TrimSpace(os.Getenv(constants.EnvBackupAuthMethod))

	jwtTokenPath := constants.PathBackupJWTToken
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvJWTTokenPath)); envPath != "" {
		jwtTokenPath = envPath
	}
	jwtToken, err := os.ReadFile(jwtTokenPath) // #nosec G304 -- Path from constant or environment variable
	if err == nil && len(jwtToken) > 0 {
		cfg.JWTToken = strings.TrimSpace(string(jwtToken))
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = constants.BackupAuthMethodJWT
		}
	}

	if cfg.AuthMethod == constants.BackupAuthMethodJWT || (cfg.AuthMethod == "" && cfg.JWTToken != "") {
		cfg.JWTAuthRole = strings.TrimSpace(os.Getenv(constants.EnvBackupJWTAuthRole))
		if cfg.JWTAuthRole == "" {
			return fmt.Errorf("BACKUP_JWT_AUTH_ROLE is required when using JWT authentication")
		}
		if cfg.JWTToken == "" {
			return fmt.Errorf("JWT token not found at %q", jwtTokenPath)
		}
		jwtAuthStrategy, err := portopenbao.NormalizeJWTAuthStrategy(os.Getenv(constants.EnvOpenBaoJWTAuthStrategy))
		if err != nil {
			return fmt.Errorf("invalid %s value: %w", constants.EnvOpenBaoJWTAuthStrategy, err)
		}
		cfg.JWTAuthStrategy = jwtAuthStrategy
		cfg.AuthMethod = constants.BackupAuthMethodJWT
		return nil
	}

	tokenPath := constants.PathBackupToken
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvBackupTokenPath)); envPath != "" {
		tokenPath = envPath
	}
	token, err := os.ReadFile(tokenPath) // #nosec G304 -- Path from constant or environment variable
	if err != nil {
		return fmt.Errorf("failed to read OpenBao token from %q: %w", tokenPath, err)
	}
	cfg.OpenBaoToken = strings.TrimSpace(string(token))
	if cfg.OpenBaoToken == "" {
		return fmt.Errorf("OpenBao token is empty")
	}
	cfg.AuthMethod = constants.BackupAuthMethodToken
	return nil
}
