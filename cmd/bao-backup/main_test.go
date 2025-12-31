package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backupconfig "github.com/dc-tec/openbao-operator/internal/backup"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestAuthenticate_Token(t *testing.T) {
	cfg := &backupconfig.ExecutorConfig{
		AuthMethod:   constants.BackupAuthMethodToken,
		OpenBaoToken: "test-token",
	}

	token, err := authenticate(context.Background(), cfg, "http://localhost:8200")
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
}

func TestAuthenticate_JWT(t *testing.T) {
	// Mock OpenBao server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "/v1/auth/jwt/login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "test-jwt", body["jwt"])
		assert.Equal(t, "test-role", body["role"])

		// Response
		resp := map[string]interface{}{
			"auth": map[string]interface{}{
				"client_token": "login-token",
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer server.Close()

	cfg := &backupconfig.ExecutorConfig{
		AuthMethod:  constants.BackupAuthMethodJWT,
		JWTAuthRole: "test-role",
		JWTToken:    "test-jwt",
	}

	token, err := authenticate(context.Background(), cfg, server.URL)
	require.NoError(t, err)
	assert.Equal(t, "login-token", token)
}
