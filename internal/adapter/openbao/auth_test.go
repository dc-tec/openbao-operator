package openbao

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInlineJWTAuthorizerAddsInlineAuthHeaders(t *testing.T) {
	t.Parallel()

	authorizer, err := newInlineJWTAuthorizer("operator-role", "jwt-token")
	if err != nil {
		t.Fatalf("newInlineJWTAuthorizer() error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://openbao.example/v1/sys/raft/configuration", nil)
	req.Header.Set(headerVaultToken, "stale-token")

	if err := authorizer.authorize(req); err != nil {
		t.Fatalf("authorize() error: %v", err)
	}

	if got := req.Header.Get(headerVaultToken); got != "" {
		t.Fatalf("%s=%q, expected empty", headerVaultToken, got)
	}
	if got := req.Header.Get(headerInlineAuthPath); got != inlineAuthJWTPath {
		t.Fatalf("%s=%q, expected %q", headerInlineAuthPath, got, inlineAuthJWTPath)
	}
	if got := req.Header.Get(headerInlineAuthOperation); got != inlineAuthOperationPut {
		t.Fatalf("%s=%q, expected %q", headerInlineAuthOperation, got, inlineAuthOperationPut)
	}

	role, err := decodeInlineAuthParameter(req.Header.Get(headerInlineAuthParameterRole))
	if err != nil {
		t.Fatalf("decode role parameter: %v", err)
	}
	if role.Key != "role" || role.Value != "operator-role" {
		t.Fatalf("role parameter=%#v, expected role/operator-role", role)
	}

	jwt, err := decodeInlineAuthParameter(req.Header.Get(headerInlineAuthParameterJWT))
	if err != nil {
		t.Fatalf("decode jwt parameter: %v", err)
	}
	if jwt.Key != "jwt" || jwt.Value != "jwt-token" {
		t.Fatalf("jwt parameter=%#v, expected jwt/jwt-token", jwt)
	}
}

func TestInlineJWTAuthorizerRequiresCredentials(t *testing.T) {
	t.Parallel()

	if _, err := newInlineJWTAuthorizer("", "jwt-token"); err == nil {
		t.Fatalf("expected error for empty role")
	}
	if _, err := newInlineJWTAuthorizer("operator-role", ""); err == nil {
		t.Fatalf("expected error for empty jwt")
	}
}

func TestTokenAuthorizerAddsTokenHeader(t *testing.T) {
	t.Parallel()

	authorizer := newTokenAuthorizer("s.token")
	req := httptest.NewRequest(http.MethodGet, "http://openbao.example/v1/sys/health", nil)

	if err := authorizer.authorize(req); err != nil {
		t.Fatalf("authorize() error: %v", err)
	}
	if got := req.Header.Get(headerVaultToken); got != "s.token" {
		t.Fatalf("%s=%q, expected s.token", headerVaultToken, got)
	}
}
