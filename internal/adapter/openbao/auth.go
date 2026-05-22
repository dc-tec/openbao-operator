package openbao

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	headerVaultToken              = "X-Vault-Token"
	headerInlineAuthPath          = "X-Vault-Inline-Auth-Path"
	headerInlineAuthOperation     = "X-Vault-Inline-Auth-Operation"
	headerInlineAuthParameterRole = "X-Vault-Inline-Auth-Parameter-role"
	headerInlineAuthParameterJWT  = "X-Vault-Inline-Auth-Parameter-jwt"

	inlineAuthJWTPath      = "jwt-operator/login"
	inlineAuthOperationPut = "update"
)

type requestAuthorizer interface {
	authorize(req *http.Request) error
	requiresAuth() bool
	kind() string
}

type noAuthAuthorizer struct{}

func (noAuthAuthorizer) authorize(*http.Request) error { return nil }
func (noAuthAuthorizer) requiresAuth() bool            { return false }
func (noAuthAuthorizer) kind() string                  { return "none" }

type tokenAuthorizer struct {
	token string
}

func newTokenAuthorizer(token string) requestAuthorizer {
	if strings.TrimSpace(token) == "" {
		return noAuthAuthorizer{}
	}
	return tokenAuthorizer{token: token}
}

func (a tokenAuthorizer) authorize(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if strings.TrimSpace(a.token) == "" {
		return fmt.Errorf("OpenBao token is required")
	}
	req.Header.Set(headerVaultToken, a.token)
	return nil
}

func (a tokenAuthorizer) requiresAuth() bool { return strings.TrimSpace(a.token) != "" }
func (a tokenAuthorizer) kind() string       { return "token" }

type inlineJWTAuthorizer struct {
	role string
	jwt  string
}

func newInlineJWTAuthorizer(role, jwtToken string) (requestAuthorizer, error) {
	role = strings.TrimSpace(role)
	jwtToken = strings.TrimSpace(jwtToken)
	if role == "" {
		return nil, fmt.Errorf("JWT auth role is required for inline authentication")
	}
	if jwtToken == "" {
		return nil, fmt.Errorf("JWT token is required for inline authentication")
	}
	return inlineJWTAuthorizer{role: role, jwt: jwtToken}, nil
}

func (a inlineJWTAuthorizer) authorize(req *http.Request) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	roleHeader, err := encodeInlineAuthParameter("role", a.role)
	if err != nil {
		return err
	}
	jwtHeader, err := encodeInlineAuthParameter("jwt", a.jwt)
	if err != nil {
		return err
	}
	req.Header.Del(headerVaultToken)
	req.Header.Set(headerInlineAuthPath, inlineAuthJWTPath)
	req.Header.Set(headerInlineAuthOperation, inlineAuthOperationPut)
	req.Header.Set(headerInlineAuthParameterRole, roleHeader)
	req.Header.Set(headerInlineAuthParameterJWT, jwtHeader)
	return nil
}

func (a inlineJWTAuthorizer) requiresAuth() bool { return true }
func (a inlineJWTAuthorizer) kind() string       { return "inline-jwt" }

type inlineAuthParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func encodeInlineAuthParameter(key, value string) (string, error) {
	payload, err := json.Marshal(inlineAuthParameter{Key: key, Value: value})
	if err != nil {
		return "", fmt.Errorf("failed to encode inline auth parameter %q: %w", key, err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeInlineAuthParameter(value string) (inlineAuthParameter, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return inlineAuthParameter{}, err
	}
	var param inlineAuthParameter
	if err := json.Unmarshal(decoded, &param); err != nil {
		return inlineAuthParameter{}, err
	}
	return param, nil
}
