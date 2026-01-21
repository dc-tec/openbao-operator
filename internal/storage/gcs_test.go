package storage

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestCACert creates a valid self-signed CA certificate for testing.
func generateTestCACert(t *testing.T) []byte {
	t.Helper()

	// Generate a new RSA key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create a self-signed certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test-ca",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	// Create the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return certPEM
}

func TestNormalizeGCSEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty endpoint returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "adds trailing slash and storage path",
			input:    "http://localhost:4443",
			expected: "http://localhost:4443/storage/v1/",
		},
		{
			name:     "handles endpoint with trailing slash",
			input:    "http://localhost:4443/",
			expected: "http://localhost:4443/storage/v1/",
		},
		{
			name:     "preserves existing storage/v1/ path",
			input:    "http://localhost:4443/storage/v1/",
			expected: "http://localhost:4443/storage/v1/",
		},
		{
			name:     "preserves existing storage/v1 path without trailing slash",
			input:    "http://localhost:4443/storage/v1",
			expected: "http://localhost:4443/storage/v1/",
		},
		{
			name:     "handles HTTPS endpoints",
			input:    "https://gcs.example.com",
			expected: "https://gcs.example.com/storage/v1/",
		},
		{
			name:     "handles endpoints with port",
			input:    "http://fake-gcs-server.gcs.svc.cluster.local:4443",
			expected: "http://fake-gcs-server.gcs.svc.cluster.local:4443/storage/v1/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeGCSEndpoint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildGCSHTTPClient_DefaultConfig(t *testing.T) {
	cfg := GCSClientConfig{}

	client, err := buildGCSHTTPClient(cfg)

	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, DefaultUploadTimeout, client.Timeout)
}

func TestBuildGCSHTTPClient_InsecureSkipVerify(t *testing.T) {
	cfg := GCSClientConfig{
		InsecureSkipVerify: true,
	}

	client, err := buildGCSHTTPClient(cfg)

	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.True(t, transport.TLSClientConfig.InsecureSkipVerify)
}

func TestBuildGCSHTTPClient_ValidCACert(t *testing.T) {
	// Generate a valid self-signed CA certificate for testing
	validCACert := generateTestCACert(t)

	cfg := GCSClientConfig{
		CACert: validCACert,
	}

	client, err := buildGCSHTTPClient(cfg)

	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestBuildGCSHTTPClient_InvalidCACert(t *testing.T) {
	cfg := GCSClientConfig{
		CACert: []byte("not a valid certificate"),
	}

	_, err := buildGCSHTTPClient(cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse CA certificate")
}

func TestOpenGCSBucket_MissingBucket(t *testing.T) {
	ctx := context.Background()

	_, err := OpenGCSBucket(ctx, GCSClientConfig{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

func TestOpenGCSBucket_EmulatorRequiresEndpoint(t *testing.T) {
	ctx := context.Background()

	_, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:      "test-bucket",
		UseEmulator: true,
		// Endpoint is missing
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required when useEmulator is true")
}

func TestOpenGCSBucket_EmulatorWithEndpoint(t *testing.T) {
	ctx := context.Background()
	var requestCount int64

	// Create a test server that simulates fake-gcs-server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		// Respond to bucket check
		if r.Method == http.MethodGet && r.URL.Path == "/storage/v1/b/test-bucket" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test-bucket"}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	store, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:             "test-bucket",
		UseEmulator:        true,
		Endpoint:           server.URL,
		InsecureSkipVerify: true,
		Project:            "test-project",
		EnsureExists:       true,
	})

	require.NoError(t, err)
	require.NotNil(t, store)
	defer func() { _ = store.Close() }()

	// Verify that requests went to our test server (endpoint was used)
	assert.Greater(t, atomic.LoadInt64(&requestCount), int64(0), "expected endpoint to be called")
}

func TestOpenGCSBucket_EmulatorWithoutEnsureExists(t *testing.T) {
	ctx := context.Background()

	// Create a test server - it shouldn't be called if EnsureExists is false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept any request during bucket opening
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	store, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:             "test-bucket",
		UseEmulator:        true,
		Endpoint:           server.URL,
		InsecureSkipVerify: true,
		EnsureExists:       false, // Don't verify bucket exists
	})

	require.NoError(t, err)
	require.NotNil(t, store)
	defer func() { _ = store.Close() }()
}

func TestOpenGCSBucket_EndpointNormalization(t *testing.T) {
	ctx := context.Background()
	var receivedPath string

	// Create a test server to capture the request path
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if r.Method == http.MethodGet && r.URL.Path == "/storage/v1/b/test-bucket" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"test-bucket"}`))
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	store, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:             "test-bucket",
		UseEmulator:        true,
		Endpoint:           server.URL, // No /storage/v1/ suffix
		InsecureSkipVerify: true,
		Project:            "test-project",
		EnsureExists:       true,
	})

	require.NoError(t, err)
	require.NotNil(t, store)
	defer func() { _ = store.Close() }()

	// Verify the path was normalized to include /storage/v1/
	assert.Contains(t, receivedPath, "/storage/v1/", "endpoint should be normalized to include /storage/v1/")
}

func TestGCSClientConfig_Defaults(t *testing.T) {
	cfg := GCSClientConfig{}

	assert.Empty(t, cfg.Bucket)
	assert.Empty(t, cfg.Project)
	assert.Empty(t, cfg.Endpoint)
	assert.Nil(t, cfg.CredentialsJSON)
	assert.False(t, cfg.UseEmulator)
	assert.False(t, cfg.InsecureSkipVerify)
	assert.Nil(t, cfg.CACert)
	assert.False(t, cfg.EnsureExists)
}

// ============================================================================
// Production Mode Tests
// ============================================================================

func TestBuildGCSCredentials_InvalidJSON(t *testing.T) {
	ctx := context.Background()

	_, err := buildGCSCredentials(ctx, []byte("not valid json"))

	require.Error(t, err)
	// The error comes from the Google credentials library
	assert.Contains(t, err.Error(), "invalid")
}

func TestBuildGCSCredentials_EmptyUsesADC(t *testing.T) {
	ctx := context.Background()

	// When no credentials JSON is provided, it tries to use ADC
	// This will fail in test environment without GCP credentials
	_, err := buildGCSCredentials(ctx, nil)

	// In test environment without ADC, this should error
	// but the error message should indicate it tried to find default credentials
	if err != nil {
		assert.Contains(t, err.Error(), "default credentials")
	}
	// If it somehow succeeds (e.g., running on GCE), that's also valid
}

func TestBuildGCSCredentials_ValidServiceAccountJSON(t *testing.T) {
	ctx := context.Background()

	// Generate a valid-looking service account JSON (structure is correct, but keys are fake)
	serviceAccountJSON := generateTestServiceAccountJSON(t)

	creds, err := buildGCSCredentials(ctx, serviceAccountJSON)

	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "test-project", creds.ProjectID)
}

func TestOpenGCSBucket_ProductionModeWithoutCredentials(t *testing.T) {
	ctx := context.Background()

	// Production mode (UseEmulator=false) requires credentials
	// Without explicit credentials, it tries ADC which will fail in test env
	_, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:      "test-bucket",
		UseEmulator: false,
		// No credentials provided
	})

	// Should fail because no credentials available
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials")
}

func TestOpenGCSBucket_ProductionModeWithInvalidCredentials(t *testing.T) {
	ctx := context.Background()

	_, err := OpenGCSBucket(ctx, GCSClientConfig{
		Bucket:          "test-bucket",
		UseEmulator:     false,
		CredentialsJSON: []byte("invalid json"),
	})

	require.Error(t, err)
	// Should fail during credential parsing
	assert.Contains(t, err.Error(), "invalid")
}

// Note: Production mode integration tests with custom endpoints require
// proper TLS setup for OAuth2 token fetching, which is complex to mock.
// These scenarios are better covered by E2E tests with real GCS or emulators.

// ============================================================================
// Test Helpers
// ============================================================================

// generateTestServiceAccountJSON creates a valid service account JSON for testing.
func generateTestServiceAccountJSON(t *testing.T) []byte {
	t.Helper()
	return generateTestServiceAccountJSONWithTokenURI(t, "https://oauth2.googleapis.com/token")
}

// generateTestServiceAccountJSONWithTokenURI creates a valid service account JSON with a custom token URI.
func generateTestServiceAccountJSONWithTokenURI(t *testing.T, tokenURI string) []byte {
	t.Helper()

	// Generate a valid RSA private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Encode the private key in PKCS8 format (required by Google credentials library)
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	// Create a valid service account JSON structure
	saJSON := map[string]interface{}{
		"type":                        "service_account",
		"project_id":                  "test-project",
		"private_key_id":              "test-key-id",
		"private_key":                 string(privateKeyPEM),
		"client_email":                "test@test-project.iam.gserviceaccount.com",
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   tokenURI,
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/test%40test-project.iam.gserviceaccount.com",
	}

	jsonBytes, err := json.Marshal(saJSON)
	require.NoError(t, err)

	return jsonBytes
}
