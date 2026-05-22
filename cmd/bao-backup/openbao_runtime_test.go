package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	backupconfig "github.com/dc-tec/openbao-operator/internal/service/backup"
)

const (
	testInlineAuthPathHeader          = "X-Vault-Inline-Auth-Path"
	testInlineAuthOperationHeader     = "X-Vault-Inline-Auth-Operation"
	testInlineAuthRoleParameterHeader = "X-Vault-Inline-Auth-Parameter-role"
	testInlineAuthJWTParameterHeader  = "X-Vault-Inline-Auth-Parameter-jwt"
	testVaultTokenHeader              = "X-Vault-Token"

	testInlineAuthJWTPath       = "auth/jwt-operator/login"
	testInlineAuthOperation     = "update"
	testBackupSnapshotPath      = "/v1/sys/storage/raft/snapshot"
	testBackupRestorePath       = "/v1/sys/storage/raft/snapshot-force"
	testBackupJWTLoginPath      = "/v1/auth/jwt-operator/login"
	testBackupJWTAuthRole       = "backup-role"
	testBackupJWTToken          = "jwt-token"
	testBackupStandardAuthToken = "standard-token"
	testBackupSnapshotData      = "snapshot-data"
	testBackupRestoreData       = "restore-data"
)

type testInlineAuthParameter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func TestOpenClusterClient_JWTInlineSnapshotUsesInlineAuthHeaders(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	var snapshotRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testBackupJWTLoginPath:
			atomic.AddInt32(&loginRequests, 1)
			http.Error(w, "unexpected login", http.StatusInternalServerError)
		case testBackupSnapshotPath:
			atomic.AddInt32(&snapshotRequests, 1)
			if r.Method != http.MethodGet {
				t.Errorf("snapshot method=%s, want GET", r.Method)
			}
			requireInlineAuthHeaders(t, r, testBackupJWTAuthRole, testBackupJWTToken)
			_, _ = w.Write([]byte(testBackupSnapshotData))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := newInlineJWTBackupConfig()
	token, err := authenticate(context.Background(), cfg, server.URL)
	if err != nil {
		t.Fatalf("authenticate() error: %v", err)
	}
	if token != "" {
		t.Fatalf("authenticate() token=%q, want empty token for inline auth", token)
	}

	client, cleanup, err := openClusterClient(cfg, "backup", server.URL, token)
	if err != nil {
		t.Fatalf("openClusterClient() error: %v", err)
	}
	defer cleanup()

	var snapshot bytes.Buffer
	if err := client.Snapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	if snapshot.String() != testBackupSnapshotData {
		t.Fatalf("snapshot=%q, want %q", snapshot.String(), testBackupSnapshotData)
	}
	if got := atomic.LoadInt32(&loginRequests); got != 0 {
		t.Fatalf("login requests=%d, want 0", got)
	}
	if got := atomic.LoadInt32(&snapshotRequests); got != 1 {
		t.Fatalf("snapshot requests=%d, want 1", got)
	}
}

func TestOpenClusterClient_JWTInlineRestoreUsesInlineAuthHeaders(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	var restoreRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testBackupJWTLoginPath:
			atomic.AddInt32(&loginRequests, 1)
			http.Error(w, "unexpected login", http.StatusInternalServerError)
		case testBackupRestorePath:
			atomic.AddInt32(&restoreRequests, 1)
			if r.Method != http.MethodPost {
				t.Errorf("restore method=%s, want POST", r.Method)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("failed to read restore body: %v", err)
			}
			if string(body) != testBackupRestoreData {
				t.Errorf("restore body=%q, want %q", string(body), testBackupRestoreData)
			}
			requireInlineAuthHeaders(t, r, testBackupJWTAuthRole, testBackupJWTToken)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := newInlineJWTBackupConfig()
	client, cleanup, err := openClusterClient(cfg, "restore", server.URL, "")
	if err != nil {
		t.Fatalf("openClusterClient() error: %v", err)
	}
	defer cleanup()

	if err := client.Restore(context.Background(), bytes.NewBufferString(testBackupRestoreData)); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	if got := atomic.LoadInt32(&loginRequests); got != 0 {
		t.Fatalf("login requests=%d, want 0", got)
	}
	if got := atomic.LoadInt32(&restoreRequests); got != 1 {
		t.Fatalf("restore requests=%d, want 1", got)
	}
}

func TestOpenClusterClient_JWTStandardSnapshotUsesClientToken(t *testing.T) {
	t.Parallel()

	var loginRequests int32
	var snapshotRequests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case testBackupJWTLoginPath:
			atomic.AddInt32(&loginRequests, 1)
			if r.Method != http.MethodPost {
				t.Errorf("login method=%s, want POST", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"auth":{"client_token":"` + testBackupStandardAuthToken + `"}}`))
		case testBackupSnapshotPath:
			atomic.AddInt32(&snapshotRequests, 1)
			if got := r.Header.Get(testVaultTokenHeader); got != testBackupStandardAuthToken {
				t.Errorf("%s=%q, want %q", testVaultTokenHeader, got, testBackupStandardAuthToken)
			}
			if got := r.Header.Get(testInlineAuthPathHeader); got != "" {
				t.Errorf("%s=%q, want empty", testInlineAuthPathHeader, got)
			}
			_, _ = w.Write([]byte(testBackupSnapshotData))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := newInlineJWTBackupConfig()
	cfg.JWTAuthStrategy = portopenbao.JWTAuthStrategyStandard

	token, err := authenticate(context.Background(), cfg, server.URL)
	if err != nil {
		t.Fatalf("authenticate() error: %v", err)
	}
	if token != testBackupStandardAuthToken {
		t.Fatalf("authenticate() token=%q, want %q", token, testBackupStandardAuthToken)
	}

	client, cleanup, err := openClusterClient(cfg, "backup", server.URL, token)
	if err != nil {
		t.Fatalf("openClusterClient() error: %v", err)
	}
	defer cleanup()

	var snapshot bytes.Buffer
	if err := client.Snapshot(context.Background(), &snapshot); err != nil {
		t.Fatalf("Snapshot() error: %v", err)
	}

	if got := atomic.LoadInt32(&loginRequests); got != 1 {
		t.Fatalf("login requests=%d, want 1", got)
	}
	if got := atomic.LoadInt32(&snapshotRequests); got != 1 {
		t.Fatalf("snapshot requests=%d, want 1", got)
	}
}

func newInlineJWTBackupConfig() *backupconfig.ExecutorConfig {
	return &backupconfig.ExecutorConfig{
		AuthMethod:      constants.BackupAuthMethodJWT,
		JWTAuthRole:     testBackupJWTAuthRole,
		JWTAuthStrategy: portopenbao.JWTAuthStrategyInline,
		JWTToken:        testBackupJWTToken,
	}
}

func requireInlineAuthHeaders(t *testing.T, r *http.Request, role, jwtToken string) {
	t.Helper()

	if got := r.Header.Get(testVaultTokenHeader); got != "" {
		t.Fatalf("%s=%q, want empty", testVaultTokenHeader, got)
	}
	if got := r.Header.Get(testInlineAuthPathHeader); got != testInlineAuthJWTPath {
		t.Fatalf("%s=%q, want %q", testInlineAuthPathHeader, got, testInlineAuthJWTPath)
	}
	if got := r.Header.Get(testInlineAuthOperationHeader); got != testInlineAuthOperation {
		t.Fatalf("%s=%q, want %q", testInlineAuthOperationHeader, got, testInlineAuthOperation)
	}

	requireInlineAuthParameter(t, r.Header.Get(testInlineAuthRoleParameterHeader), "role", role)
	requireInlineAuthParameter(t, r.Header.Get(testInlineAuthJWTParameterHeader), "jwt", jwtToken)
}

func requireInlineAuthParameter(t *testing.T, encoded, key, value string) {
	t.Helper()

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("failed to decode inline auth parameter %q: %v", key, err)
	}
	var param testInlineAuthParameter
	if err := json.Unmarshal(decoded, &param); err != nil {
		t.Fatalf("failed to unmarshal inline auth parameter %q: %v", key, err)
	}
	if param.Key != key || param.Value != value {
		t.Fatalf("inline auth parameter=%#v, want key=%q value=%q", param, key, value)
	}
}
