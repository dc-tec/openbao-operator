package security

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/sigstore/cosign/v3/pkg/oci"
	"github.com/sigstore/cosign/v3/pkg/oci/static"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

const (
	OpenBaoImageRef = "ghcr.io/openbao/openbao:2.4.4"
	// Use a valid digest format: SHA256 requires 64 hex characters
	testImageDigest = "ghcr.io/test/image@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	testOIDCIssuer  = "https://token.actions.githubusercontent.com"
	testOIDCSubject = "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/2.0.0"
)

func TestNewImageVerifier(t *testing.T) {
	logger := logr.Discard()
	reader := &readerOnly{Reader: fake.NewClientBuilder().Build()}
	verifier := NewImageVerifier(logger, reader, nil)

	if verifier == nil {
		t.Fatal("NewImageVerifier() returned nil")
	}

	if verifier.logger != logger {
		t.Error("NewImageVerifier() logger not set correctly")
	}

	if verifier.cache == nil {
		t.Error("NewImageVerifier() cache not initialized")
	}

	if verifier.tagCache == nil {
		t.Error("NewImageVerifier() tagCache not initialized")
	}

	if verifier.reader != reader {
		t.Error("NewImageVerifier() reader not set correctly")
	}
}

type readerOnly struct {
	crclient.Reader
}

func TestImageVerifier_LoadTrustedRoot_UsesEmbeddedWhenConfigAbsent(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)

	trustedRoot, err := verifier.loadTrustedRoot(context.Background())
	if err != nil {
		t.Fatalf("loadTrustedRoot() error = %v", err)
	}
	if trustedRoot == nil {
		t.Fatal("loadTrustedRoot() returned nil trusted root")
	}
}

func TestImageVerifier_LoadTrustedRoot_ConfigMap(t *testing.T) {
	tests := []struct {
		name        string
		client      crclient.Client
		config      *TrustedRootConfig
		wantErrPart string
	}{
		{
			name: "loads configured configmap",
			client: newTrustedRootTestClient(t, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sigstore-root",
					Namespace: "openbao-system",
				},
				Data: map[string]string{
					trustedRootConfigMapKey: string(embeddedTrustedRootJSON),
				},
			}),
			config: &TrustedRootConfig{
				ConfigMapName:      "sigstore-root",
				ConfigMapNamespace: "openbao-system",
			},
		},
		{
			name:   "missing configured configmap fails closed",
			client: newTrustedRootTestClient(t),
			config: &TrustedRootConfig{
				ConfigMapName:      "sigstore-root",
				ConfigMapNamespace: "openbao-system",
			},
			wantErrPart: "failed to load trusted root ConfigMap openbao-system/sigstore-root",
		},
		{
			name: "missing trusted root key fails closed",
			client: newTrustedRootTestClient(t, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sigstore-root",
					Namespace: "openbao-system",
				},
				Data: map[string]string{"other": "{}"},
			}),
			config: &TrustedRootConfig{
				ConfigMapName:      "sigstore-root",
				ConfigMapNamespace: "openbao-system",
			},
			wantErrPart: `missing required key "trusted_root.json"`,
		},
		{
			name: "invalid trusted root json fails closed",
			client: newTrustedRootTestClient(t, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sigstore-root",
					Namespace: "openbao-system",
				},
				Data: map[string]string{trustedRootConfigMapKey: "not-json"},
			}),
			config: &TrustedRootConfig{
				ConfigMapName:      "sigstore-root",
				ConfigMapNamespace: "openbao-system",
			},
			wantErrPart: "failed to parse trusted_root.json from ConfigMap openbao-system/sigstore-root",
		},
		{
			name:        "partial config fails closed",
			client:      newTrustedRootTestClient(t),
			config:      &TrustedRootConfig{ConfigMapName: "sigstore-root"},
			wantErrPart: "requires both namespace and name",
		},
		{
			name:        "configured configmap without client fails closed",
			client:      nil,
			config:      &TrustedRootConfig{ConfigMapName: "sigstore-root", ConfigMapNamespace: "openbao-system"},
			wantErrPart: "Kubernetes client is not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := NewImageVerifier(logr.Discard(), tt.client, tt.config)

			trustedRoot, err := verifier.loadTrustedRoot(context.Background())
			if tt.wantErrPart != "" {
				if err == nil {
					t.Fatal("loadTrustedRoot() error = nil, want error")
				}
				if !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("loadTrustedRoot() error = %q, want substring %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("loadTrustedRoot() error = %v", err)
			}
			if trustedRoot == nil {
				t.Fatal("loadTrustedRoot() returned nil trusted root")
			}
		})
	}
}

func newTrustedRootTestClient(t *testing.T, objects ...crclient.Object) crclient.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add corev1 scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestImageVerifier_Verify_EmptyConfig(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	ctx := context.Background()
	config := imageverify.VerifyConfig{}
	_, err := verifier.Verify(ctx, "test-image:latest", config)

	if err == nil {
		t.Error("Verify() with empty config should return error")
	}

	expectedError := "either PublicKey OR keyless identity (Issuer/Subject or IssuerRegExp/SubjectRegExp) must be provided for image verification"
	if err.Error() != expectedError {
		t.Errorf("Verify() error = %v, want '%s'", err, expectedError)
	}
}

func TestImageVerifier_Verify_KeylessMissingIssuer(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	ctx := context.Background()
	config := imageverify.VerifyConfig{
		Subject: testOIDCSubject,
	}
	_, err := verifier.Verify(ctx, "test-image:latest", config)

	if err == nil {
		t.Error("Verify() with keyless config missing issuer should return error")
	}
}

func TestImageVerifier_Verify_KeylessMissingSubject(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	ctx := context.Background()
	config := imageverify.VerifyConfig{
		Issuer: "https://token.actions.githubusercontent.com",
	}
	_, err := verifier.Verify(ctx, "test-image:latest", config)

	if err == nil {
		t.Error("Verify() with keyless config missing subject should return error")
	}
}

func TestImageVerifier_Verify_KeylessRegExpCacheHit(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	config := imageverify.VerifyConfig{
		IssuerRegExp:  "^https://token\\.actions\\.githubusercontent\\.com$",
		SubjectRegExp: "^https://github\\.com/dc-tec/openbao-operator/.+@refs/tags/.+$",
	}
	cacheKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)
	verifier.cache.markVerifiedByKey(cacheKey)

	result, err := verifier.Verify(context.Background(), testImageDigest, config)
	if err != nil {
		t.Fatalf("Verify() with cached regexp keyless config should succeed: %v", err)
	}
	if result != testImageDigest {
		t.Fatalf("Verify() = %q, want %q", result, testImageDigest)
	}
}

func TestImageVerifier_Verify_CacheHit(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	// Use a digest for cache key (cache lookup happens before verification now)
	digest := testImageDigest
	config := imageverify.VerifyConfig{
		PublicKey: "test-public-key",
	}

	// Mark as verified in cache using the cache key method
	// With the new cache-first implementation, if we pre-populate the cache
	// with the digest, the Verify method should return early without calling verifyImageSignature
	cacheKey := requireImageVerificationCacheKey(t, verifier, digest, config)
	verifier.cache.markVerifiedByKey(cacheKey)

	ctx := context.Background()
	// When calling Verify with a tag, it will first call resolveDigest which makes a HEAD request.
	// Since we can't mock the registry here, we test with the digest directly which
	// will match the cached entry and return immediately.
	// For a full integration test with registry mocking, see E2E tests.
	result, err := verifier.Verify(ctx, testImageDigest, config)

	// With a digest reference, resolveDigest returns it directly, then cache is checked
	// Since the cache key matches, we should get a cache hit and return the digest
	if err != nil {
		t.Errorf("Verify() with cached digest should succeed, got error: %v", err)
	}
	if result != digest {
		t.Errorf("Verify() returned %v, want %v", result, digest)
	}
}

func TestImageVerifier_Verify_CacheMiss(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	imageRef := "test-image:latest"
	config := imageverify.VerifyConfig{
		PublicKey: "invalid-public-key",
	}

	ctx := context.Background()
	// This will attempt actual verification which will fail with invalid key
	// but we're testing the cache miss path
	_, err := verifier.Verify(ctx, imageRef, config)

	// Should fail because the key is invalid and verification will fail
	if err == nil {
		t.Error("Verify() with invalid public key should return error")
	}

	// Verify it was not cached (since verification failed)
	// Note: The cache now uses digest, so we can't easily check without knowing the digest
	// This test verifies the error path works correctly
}

func TestImageVerifier_Verify_ContextCancellation(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	imageRef := "test-image:latest"
	config := imageverify.VerifyConfig{
		PublicKey: "test-public-key",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should respect context cancellation
	_, err := verifier.Verify(ctx, imageRef, config)

	// The error might be from context cancellation or from verification failure
	// Both are acceptable - we're testing that context is respected
	if err == nil {
		t.Error("Verify() with cancelled context should return error")
	}
}

func TestVerificationCache_IsVerifiedByKey(t *testing.T) {
	cache := newVerificationCache()

	cacheKey := testImageDigest + "@key:1234567890abcdef"

	// Initially not verified
	if cache.isVerifiedByKey(cacheKey) {
		t.Error("isVerifiedByKey() should return false for unverified image")
	}

	// Mark as verified
	cache.markVerifiedByKey(cacheKey)

	// Should now be verified
	if !cache.isVerifiedByKey(cacheKey) {
		t.Error("isVerifiedByKey() should return true for verified image")
	}
}

func TestVerificationCache_MarkVerifiedByKey(t *testing.T) {
	cache := newVerificationCache()

	cacheKey1 := "test-image-1@sha256:abc123@key:1234567890abcdef"
	cacheKey2 := "test-image-2@sha256:def456@key:fedcba0987654321"
	cacheKey3 := "test-image-1@sha256:abc123@oidc:" + testOIDCIssuer + "|" + testOIDCSubject

	// Mark first image as verified
	cache.markVerifiedByKey(cacheKey1)

	// First image should be verified
	if !cache.isVerifiedByKey(cacheKey1) {
		t.Error("markVerifiedByKey() did not mark image as verified")
	}

	// Second image should not be verified
	if cache.isVerifiedByKey(cacheKey2) {
		t.Error("markVerifiedByKey() should not affect other images")
	}

	// Third image (same digest, different verification mode) should not be verified
	if cache.isVerifiedByKey(cacheKey3) {
		t.Error("markVerifiedByKey() should be keyed by both digest and verification config")
	}

	// Mark second image
	cache.markVerifiedByKey(cacheKey2)

	// Both should now be verified
	if !cache.isVerifiedByKey(cacheKey1) {
		t.Error("markVerifiedByKey() should not affect previously verified images")
	}
	if !cache.isVerifiedByKey(cacheKey2) {
		t.Error("markVerifiedByKey() should mark second image as verified")
	}
}

func TestVerificationCache_ConcurrentAccess(t *testing.T) {
	cache := newVerificationCache()

	cacheKey := testImageDigest + "@key:1234567890abcdef"

	// Test concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			cache.markVerifiedByKey(cacheKey)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be verified (no race condition)
	if !cache.isVerifiedByKey(cacheKey) {
		t.Error("Concurrent markVerifiedByKey() calls should not cause race conditions")
	}

	// Test concurrent reads
	readDone := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_ = cache.isVerifiedByKey(cacheKey)
			readDone <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-readDone
	}

	// Should still be verified
	if !cache.isVerifiedByKey(cacheKey) {
		t.Error("Concurrent isVerifiedByKey() calls should not cause race conditions")
	}
}

func TestTagResolutionCache_GetSet(t *testing.T) {
	cache := newTagResolutionCache()

	imageRef := OpenBaoImageRef
	digest := "ghcr.io/openbao/openbao@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Initially not in cache
	if cached, ok := cache.get(imageRef); ok {
		t.Errorf("get() should return false for uncached image, got %s", cached)
	}

	// Set the mapping
	cache.set(imageRef, digest)

	// Should now be in cache
	cached, ok := cache.get(imageRef)
	if !ok {
		t.Error("get() should return true for cached image")
	}
	if cached != digest {
		t.Errorf("get() = %v, want %v", cached, digest)
	}
}

func TestTagResolutionCache_DifferentTags(t *testing.T) {
	cache := newTagResolutionCache()

	imageRef1 := OpenBaoImageRef
	digest1 := "ghcr.io/openbao/openbao@sha256:abc123"
	imageRef2 := "ghcr.io/openbao/openbao:2.1.0"
	digest2 := "ghcr.io/openbao/openbao@sha256:def456"

	// Set both mappings
	cache.set(imageRef1, digest1)
	cache.set(imageRef2, digest2)

	// Both should be retrievable independently
	cached1, ok1 := cache.get(imageRef1)
	if !ok1 || cached1 != digest1 {
		t.Errorf("get(%s) = (%v, %v), want (%v, true)", imageRef1, cached1, ok1, digest1)
	}

	cached2, ok2 := cache.get(imageRef2)
	if !ok2 || cached2 != digest2 {
		t.Errorf("get(%s) = (%v, %v), want (%v, true)", imageRef2, cached2, ok2, digest2)
	}
}

func TestTagResolutionCache_ConcurrentAccess(t *testing.T) {
	cache := newTagResolutionCache()

	imageRef := OpenBaoImageRef
	digest := "ghcr.io/openbao/openbao@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	// Test concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			cache.set(imageRef, digest)
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be cached (no race condition)
	cached, ok := cache.get(imageRef)
	if !ok || cached != digest {
		t.Errorf("Concurrent set() calls caused issues: get() = (%v, %v)", cached, ok)
	}

	// Test concurrent reads
	readDone := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, _ = cache.get(imageRef)
			readDone <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-readDone
	}

	// Should still be cached
	cached, ok = cache.get(imageRef)
	if !ok || cached != digest {
		t.Errorf("Concurrent get() calls caused issues: get() = (%v, %v)", cached, ok)
	}
}

func requireImageVerificationCacheKey(
	t *testing.T,
	verifier *ImageVerifier,
	digest string,
	config imageverify.VerifyConfig,
) string {
	t.Helper()

	trustedRoot, err := verifier.verificationTrustedRoot(context.Background(), config)
	if err != nil {
		t.Fatalf("verificationTrustedRoot() error = %v", err)
	}
	key, err := verifier.cacheKey(digest, config, trustedRoot.identity)
	if err != nil {
		t.Fatalf("cacheKey() error = %v", err)
	}
	return key
}

func TestImageVerifier_CacheKey_DeterministicAndOpaque(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)
	config := imageverify.VerifyConfig{
		PublicKey:        "sensitive-public-key-material",
		Issuer:           testOIDCIssuer,
		Subject:          testOIDCSubject,
		IssuerRegExp:     "issuer-regexp",
		SubjectRegExp:    "subject-regexp",
		IgnoreTlog:       true,
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
		Namespace:        "tenant-a",
	}

	firstKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)
	secondKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)

	if firstKey != secondKey {
		t.Fatalf("cacheKey() must be deterministic, got %q and %q", firstKey, secondKey)
	}
	if len(firstKey) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(firstKey, "sha256:") {
		t.Fatalf("cacheKey() = %q, want an opaque SHA-256 identity", firstKey)
	}
	for _, rawValue := range []string{testImageDigest, config.PublicKey, config.Issuer, config.Namespace} {
		if strings.Contains(firstKey, rawValue) {
			t.Fatalf("cacheKey() exposes raw identity value %q", rawValue)
		}
	}
}

func TestImageVerifier_CacheKey_PartitionsCompletePolicy(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)
	baseConfig := imageverify.VerifyConfig{
		PublicKey:        "public-key-a",
		Issuer:           "issuer-a",
		Subject:          "subject-a",
		IssuerRegExp:     "issuer-regexp-a",
		SubjectRegExp:    "subject-regexp-a",
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "pull-secret-a"}},
		Namespace:        "tenant-a",
	}
	baseKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, baseConfig)

	tests := []struct {
		name         string
		digest       string
		mutateConfig func(*imageverify.VerifyConfig)
	}{
		{name: "digest", digest: strings.Replace(testImageDigest, "e3b0", "a3b0", 1)},
		{name: "public key", mutateConfig: func(config *imageverify.VerifyConfig) { config.PublicKey = "public-key-b" }},
		{name: "issuer", mutateConfig: func(config *imageverify.VerifyConfig) { config.Issuer = "issuer-b" }},
		{name: "subject", mutateConfig: func(config *imageverify.VerifyConfig) { config.Subject = "subject-b" }},
		{name: "issuer regexp", mutateConfig: func(config *imageverify.VerifyConfig) {
			config.IssuerRegExp = "issuer-regexp-b"
		}},
		{name: "subject regexp", mutateConfig: func(config *imageverify.VerifyConfig) {
			config.SubjectRegExp = "subject-regexp-b"
		}},
		{name: "ignore transparency log", mutateConfig: func(config *imageverify.VerifyConfig) { config.IgnoreTlog = true }},
		{name: "image pull secrets", mutateConfig: func(config *imageverify.VerifyConfig) {
			config.ImagePullSecrets = []corev1.LocalObjectReference{{Name: "pull-secret-b"}}
		}},
		{name: "namespace", mutateConfig: func(config *imageverify.VerifyConfig) { config.Namespace = "tenant-b" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest := testImageDigest
			if tt.digest != "" {
				digest = tt.digest
			}
			config := baseConfig
			if tt.mutateConfig != nil {
				tt.mutateConfig(&config)
			}
			otherKey := requireImageVerificationCacheKey(t, verifier, digest, config)
			if baseKey == otherKey {
				t.Fatal("cacheKey() must change when the digest or verification policy changes")
			}
		})
	}
}

func TestImageVerifier_CacheKey_DistinctPEMKeys(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)
	firstKey := `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEADzj0BZr2PNIi6MV6zHTJJxePXUxxcseBO7TJoEJbYlw=
-----END PUBLIC KEY-----`
	secondKey := `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEALw1Zfprgjhns+qGZibqV852Eo+BQpmzMWMjyE0YB6Vc=
-----END PUBLIC KEY-----`

	firstCacheKey := requireImageVerificationCacheKey(
		t,
		verifier,
		testImageDigest,
		imageverify.VerifyConfig{PublicKey: firstKey},
	)
	secondCacheKey := requireImageVerificationCacheKey(
		t,
		verifier,
		testImageDigest,
		imageverify.VerifyConfig{PublicKey: secondKey},
	)

	if firstCacheKey == secondCacheKey {
		t.Fatal("cacheKey() must isolate distinct PEM public keys")
	}
}

func TestImageVerifier_CacheKey_IgnoreTlog(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)
	config := imageverify.VerifyConfig{PublicKey: "test-public-key"}

	withTlog := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)
	config.IgnoreTlog = true
	withoutTlog := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)

	if withTlog == withoutTlog {
		t.Fatal("cacheKey() must isolate transparency log policies")
	}
}

func TestVerificationCache_PolicyIsolation(t *testing.T) {
	verifier := NewImageVerifier(logr.Discard(), fake.NewClientBuilder().Build(), nil)
	verifiedPolicy := imageverify.VerifyConfig{
		PublicKey: "test-public-key",
		Namespace: "tenant-a",
	}
	otherPolicy := verifiedPolicy
	otherPolicy.Namespace = "tenant-b"

	verifiedKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, verifiedPolicy)
	otherKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, otherPolicy)
	verifier.cache.markVerifiedByKey(verifiedKey)

	if verifier.cache.isVerifiedByKey(otherKey) {
		t.Fatal("verification cache must miss for a different policy")
	}
}

func TestImageVerifier_CacheKey_ExternalTrustedRootChange(t *testing.T) {
	ctx := context.Background()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sigstore-root",
			Namespace: "openbao-system",
		},
		Data: map[string]string{trustedRootConfigMapKey: string(embeddedTrustedRootJSON)},
	}
	client := newTrustedRootTestClient(t, configMap)
	verifier := NewImageVerifier(logr.Discard(), client, &TrustedRootConfig{
		ConfigMapName:      configMap.Name,
		ConfigMapNamespace: configMap.Namespace,
	})
	config := imageverify.VerifyConfig{
		Issuer:  testOIDCIssuer,
		Subject: testOIDCSubject,
	}

	firstKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)
	var current corev1.ConfigMap
	if err := client.Get(ctx, crclient.ObjectKeyFromObject(configMap), &current); err != nil {
		t.Fatalf("get trusted root ConfigMap: %v", err)
	}
	current.Data[trustedRootConfigMapKey] += "\n"
	if err := client.Update(ctx, &current); err != nil {
		t.Fatalf("update trusted root ConfigMap: %v", err)
	}
	secondKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)

	if firstKey == secondKey {
		t.Fatal("cacheKey() must change when external trusted-root content changes")
	}
}

func TestImageVerifier_Verify_DoesNotReuseCacheAfterExternalTrustedRootChange(t *testing.T) {
	ctx := context.Background()
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sigstore-root",
			Namespace: "openbao-system",
		},
		Data: map[string]string{trustedRootConfigMapKey: string(embeddedTrustedRootJSON)},
	}
	client := newTrustedRootTestClient(t, configMap)
	verifier := NewImageVerifier(logr.Discard(), client, &TrustedRootConfig{
		ConfigMapName:      configMap.Name,
		ConfigMapNamespace: configMap.Namespace,
	})
	config := imageverify.VerifyConfig{
		Issuer:  testOIDCIssuer,
		Subject: testOIDCSubject,
	}
	cacheKey := requireImageVerificationCacheKey(t, verifier, testImageDigest, config)
	verifier.cache.markVerifiedByKey(cacheKey)

	var current corev1.ConfigMap
	if err := client.Get(ctx, crclient.ObjectKeyFromObject(configMap), &current); err != nil {
		t.Fatalf("get trusted root ConfigMap: %v", err)
	}
	current.Data[trustedRootConfigMapKey] = "not-json"
	if err := client.Update(ctx, &current); err != nil {
		t.Fatalf("update trusted root ConfigMap: %v", err)
	}

	_, err := verifier.Verify(ctx, testImageDigest, config)
	if err == nil {
		t.Fatal("Verify() reused a result from the previous trusted root")
	}
	if !strings.Contains(err.Error(), "failed to parse trusted_root.json") {
		t.Fatalf("Verify() error = %q, want trusted-root parse failure", err.Error())
	}
}

func TestShouldAttemptBundleFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "explicit no signatures found",
			err:  fmt.Errorf("no signatures found"),
			want: true,
		},
		{
			name: "wrapped no signatures found",
			err:  fmt.Errorf("legacy verification failed: %w", fmt.Errorf("no signatures found")),
			want: true,
		},
		{
			name: "different verification failure",
			err:  fmt.Errorf("signature verification failed"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAttemptBundleFallback(tt.err); got != tt.want {
				t.Fatalf("shouldAttemptBundleFallback() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBundlePredicateType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		attestation oci.Signature
		want        string
		wantErr     bool
	}{
		{
			name:        "valid signature predicate",
			attestation: mustNewBundleAttestation(t, dsseInTotoPayloadType, cosignSignaturePredicateTypeV1),
			want:        cosignSignaturePredicateTypeV1,
			wantErr:     false,
		},
		{
			name:        "invalid payload type",
			attestation: mustNewBundleAttestation(t, "application/json", cosignSignaturePredicateTypeV1),
			wantErr:     true,
		},
		{
			name:        "empty predicate type",
			attestation: mustNewBundleAttestation(t, dsseInTotoPayloadType, ""),
			wantErr:     true,
		},
		{
			name:        "invalid dsse payload",
			attestation: mustNewRawAttestation(t, []byte(`{"payloadType":"application/vnd.in-toto+json","payload":"%%%","signatures":[]}`)),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := bundlePredicateType(tt.attestation)
			if tt.wantErr {
				if err == nil {
					t.Fatal("bundlePredicateType() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("bundlePredicateType() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("bundlePredicateType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountBundleSignaturePredicates(t *testing.T) {
	t.Parallel()

	attestations := []oci.Signature{
		mustNewBundleAttestation(t, dsseInTotoPayloadType, cosignSignaturePredicateTypeV1),
		mustNewBundleAttestation(t, dsseInTotoPayloadType, "https://slsa.dev/provenance/v1"),
	}

	signatureCount, observedPredicates, err := countBundleSignaturePredicates(attestations)
	if err != nil {
		t.Fatalf("countBundleSignaturePredicates() unexpected error: %v", err)
	}
	if signatureCount != 1 {
		t.Fatalf("countBundleSignaturePredicates() signatureCount = %d, want 1", signatureCount)
	}
	if len(observedPredicates) != 2 {
		t.Fatalf("countBundleSignaturePredicates() observedPredicates len = %d, want 2", len(observedPredicates))
	}
}

func mustNewBundleAttestation(t *testing.T, payloadType, predicateType string) oci.Signature {
	t.Helper()

	statement := inTotoStatement{
		PredicateType: predicateType,
	}
	statementJSON, err := json.Marshal(statement)
	if err != nil {
		t.Fatalf("json.Marshal(statement) failed: %v", err)
	}

	envelope := dsseEnvelope{
		PayloadType: payloadType,
		Payload:     base64.StdEncoding.EncodeToString(statementJSON),
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal(envelope) failed: %v", err)
	}

	return mustNewRawAttestation(t, envelopeJSON)
}

func mustNewRawAttestation(t *testing.T, payload []byte) oci.Signature {
	t.Helper()

	attestation, err := static.NewAttestation(payload)
	if err != nil {
		t.Fatalf("static.NewAttestation() failed: %v", err)
	}

	return attestation
}
