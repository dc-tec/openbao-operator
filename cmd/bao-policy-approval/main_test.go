package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	desiredPolicy = "path \"sys/policies/acl/openbao-operator\" { capabilities = [\"read\"] }\n"
	oldPolicy     = "path \"sys/policies/acl/openbao-operator\" { capabilities = [\"deny\"] }\n"
	fixtureToken  = "sensitive-client-token"
	fixtureJWT    = "sensitive-projected-jwt"
)

func TestApprovalJob(t *testing.T) {
	tests := []struct {
		name          string
		current       string
		expected      string
		serverVersion string
		conflict      bool
		verifyDrift   bool
		wantWrites    int
		wantError     string
	}{
		{name: "create", expected: absentPolicy, wantWrites: 1},
		{name: "upgrade", current: oldPolicy, expected: digestOf(oldPolicy), wantWrites: 1},
		{name: "retry after successful write", current: desiredPolicy, expected: digestOf(oldPolicy)},
		{name: "stale job", current: oldPolicy, expected: absentPolicy, wantError: "review before submitting"},
		{name: "deletion requires reviewed creation", expected: digestOf(oldPolicy), wantError: "review before submitting"},
		{name: "concurrent write", current: oldPolicy, expected: digestOf(oldPolicy),
			conflict: true, wantWrites: 1, wantError: "compare-and-set"},
		{name: "drift before verification", expected: absentPolicy,
			verifyDrift: true, wantWrites: 1, wantError: "changed before verification"},
		{name: "old server rejected before creation", expected: absentPolicy,
			serverVersion: "2.5.5", wantError: "2.6 or later"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			current, version, writes := tt.current, int64(7), 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/sys/health":
					v := tt.serverVersion
					if v == "" {
						v = "2.6.2"
					}
					_, _ = fmt.Fprintf(w, `{"version":%q}`, v)
				case "/v1/auth/jwt-operator/login":
					var login map[string]string
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&login))
					assert.Equal(t, map[string]string{"role": "approver", "jwt": fixtureJWT}, login)
					_, _ = fmt.Fprintf(w, `{"auth":{"client_token":%q}}`, fixtureToken)
				case approvalPath:
					assert.Equal(t, fixtureToken, r.Header.Get("X-Vault-Token"))
					if r.Method == http.MethodGet {
						if current == "" {
							w.WriteHeader(http.StatusNotFound)
							return
						}
						_, _ = fmt.Fprintf(w, `{"data":{"policy":%q,"version":%d}}`, current, version)
						return
					}
					assert.Equal(t, http.MethodPost, r.Method)
					writes++
					var payload struct {
						Policy   string `json:"policy"`
						CAS      int64  `json:"cas"`
						Required bool   `json:"cas_required"`
					}
					assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
					assert.Equal(t, desiredPolicy, payload.Policy)
					assert.True(t, payload.Required)
					if current == "" {
						assert.Equal(t, int64(-1), payload.CAS)
					} else {
						assert.Equal(t, version, payload.CAS)
					}
					if tt.conflict {
						http.Error(w, fixtureToken, http.StatusBadRequest)
						return
					}
					current, version = payload.Policy, version+1
					if tt.verifyDrift {
						current = oldPolicy
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					w.WriteHeader(http.StatusForbidden)
				}
			}))
			defer server.Close()
			args := jobArgs(t, server, tt.expected)
			var output bytes.Buffer
			err := run(context.Background(), args, &output)
			if tt.wantError != "" {
				require.ErrorContains(t, err, tt.wantError)
				require.NotContains(t, err.Error(), fixtureToken)
				require.Empty(t, output.String())
			} else {
				require.NoError(t, err)
				require.Contains(t, output.String(), fmt.Sprintf("changed=%t", tt.wantWrites > 0))
			}
			require.Equal(t, tt.wantWrites, writes)
		})
	}
}

func TestRejectInvalidInputsBeforeAuthentication(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { requests++ }))
	defer server.Close()
	for _, args := range [][]string{
		{"--sha256=" + strings.Repeat("0", 64)},
		{"--expected-current-sha256="},
		{"--address=http://example.test"},
		{"--address=https://user:secret@example.test"},
		{"--address=https://example.test?token=secret"},
		{"--auth-mount=../other"},
		{"--timeout=0s"},
	} {
		require.Error(t, run(context.Background(), append(jobArgs(t, server, absentPolicy), args...), &bytes.Buffer{}))
	}
	require.Zero(t, requests)
}

func TestRejectCredentialRedirect(t *testing.T) {
	forwarded := 0
	destination := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { forwarded++ }))
	defer destination.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/sys/health" {
			_, _ = fmt.Fprint(w, `{"version":"2.6.2"}`)
			return
		}
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	err := run(context.Background(), jobArgs(t, server, absentPolicy), &bytes.Buffer{})
	require.ErrorContains(t, err, "HTTP 307")
	require.Zero(t, forwarded)
	require.NotContains(t, err.Error(), fixtureJWT)
}

func TestReadinessRetryAndDeadline(t *testing.T) {
	for _, ready := range []bool{true, false} {
		t.Run(fmt.Sprintf("eventuallyReady=%t", ready), func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				if !ready || requests == 1 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_, _ = fmt.Fprint(w, `{"version":"2.6.2"}`)
			}))
			defer server.Close()
			client := &approvalClient{address: server.URL, http: server.Client()}
			timeout := 2 * time.Second
			if !ready {
				timeout = 20 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			err := client.checkVersion(ctx)
			if ready {
				require.NoError(t, err)
				require.Equal(t, 2, requests)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			}
		})
	}
}

func jobArgs(t *testing.T, server *httptest.Server, expected string) []string {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.crt")
	jwtPath, policyPath := filepath.Join(dir, "jwt"), filepath.Join(dir, "policy.hcl")
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NoError(t, os.WriteFile(caPath, cert, 0o644))
	require.NoError(t, os.WriteFile(jwtPath, []byte(fixtureJWT), 0o600))
	require.NoError(t, os.WriteFile(policyPath, []byte(desiredPolicy), 0o644))
	return []string{
		"--address=" + server.URL, "--ca-file=" + caPath, "--role=approver", "--jwt-file=" + jwtPath,
		"--policy-file=" + policyPath, "--sha256=" + digestOf(desiredPolicy), "--expected-current-sha256=" + expected,
	}
}
