package security

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	OpenBaoImageRef = "ghcr.io/openbao/openbao:2.4.4"
	// Use a valid digest format: SHA256 requires 64 hex characters
	testImageDigest = "ghcr.io/test/image@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	testOIDCIssuer  = "https://token.actions.githubusercontent.com"
	testOIDCSubject = "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/v2.0.0"
)

func TestNewImageVerifier(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

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

	if verifier.client != client {
		t.Error("NewImageVerifier() client not set correctly")
	}
}

func TestImageVerifier_Verify_EmptyConfig(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	ctx := context.Background()
	config := VerifyConfig{}
	_, err := verifier.Verify(ctx, "test-image:latest", config)

	if err == nil {
		t.Error("Verify() with empty config should return error")
	}

	expectedError := "either PublicKey OR (Issuer and Subject) must be provided for image verification"
	if err.Error() != expectedError {
		t.Errorf("Verify() error = %v, want '%s'", err, expectedError)
	}
}

func TestImageVerifier_Verify_KeylessMissingIssuer(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	ctx := context.Background()
	config := VerifyConfig{
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
	config := VerifyConfig{
		Issuer: "https://token.actions.githubusercontent.com",
	}
	_, err := verifier.Verify(ctx, "test-image:latest", config)

	if err == nil {
		t.Error("Verify() with keyless config missing subject should return error")
	}
}

func TestImageVerifier_Verify_CacheHit(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	// Use a digest for cache key (cache lookup happens before verification now)
	digest := testImageDigest
	config := VerifyConfig{
		PublicKey: "test-public-key",
	}

	// Mark as verified in cache using the cache key method
	// With the new cache-first implementation, if we pre-populate the cache
	// with the digest, the Verify method should return early without calling verifyImageSignature
	cacheKey := verifier.cacheKey(digest, config)
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
	config := VerifyConfig{
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
	config := VerifyConfig{
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

func TestImageVerifier_CacheKey_StaticKey(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	tests := []struct {
		name       string
		digest     string
		config     VerifyConfig
		wantPrefix string
	}{
		{
			name:   "simple digest and key",
			digest: testImageDigest,
			config: VerifyConfig{
				PublicKey: "test-key",
			},
			wantPrefix: testImageDigest + "@key:",
		},
		{
			name:   "digest with full hash",
			digest: "test-image@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			config: VerifyConfig{
				PublicKey: "test-key",
			},
			wantPrefix: "test-image@sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890@key:",
		},
		{
			name:   "long public key",
			digest: testImageDigest,
			config: VerifyConfig{
				PublicKey: "very-long-public-key-that-should-be-truncated-in-cache-key",
			},
			wantPrefix: testImageDigest + "@key:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := verifier.cacheKey(tt.digest, tt.config)

			if !startsWith(key, tt.wantPrefix) {
				t.Errorf("cacheKey() = %v, want prefix %v", key, tt.wantPrefix)
			}

			// Key should be consistent for same inputs
			key2 := verifier.cacheKey(tt.digest, tt.config)
			if key != key2 {
				t.Errorf("cacheKey() should be deterministic, got %v and %v", key, key2)
			}
		})
	}
}

func TestImageVerifier_CacheKey_Keyless(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	digest := testImageDigest
	config := VerifyConfig{
		Issuer:  testOIDCIssuer,
		Subject: testOIDCSubject,
	}

	key := verifier.cacheKey(digest, config)
	expectedPrefix := testImageDigest + "@oidc:" + testOIDCIssuer + "|" + testOIDCSubject

	if key != expectedPrefix {
		t.Errorf("cacheKey() = %v, want %v", key, expectedPrefix)
	}

	// Key should be consistent for same inputs
	key2 := verifier.cacheKey(digest, config)
	if key != key2 {
		t.Errorf("cacheKey() should be deterministic, got %v and %v", key, key2)
	}
}

func TestImageVerifier_CacheKey_DifferentModes(t *testing.T) {
	logger := logr.Discard()
	client := fake.NewClientBuilder().Build()
	verifier := NewImageVerifier(logger, client, nil)

	digest := testImageDigest
	staticKeyConfig := VerifyConfig{
		PublicKey: "test-key",
	}
	keylessConfig := VerifyConfig{
		Issuer:  testOIDCIssuer,
		Subject: testOIDCSubject,
	}

	key1 := verifier.cacheKey(digest, staticKeyConfig)
	key2 := verifier.cacheKey(digest, keylessConfig)

	if key1 == key2 {
		t.Error("cacheKey() should generate different keys for static key vs keyless modes")
	}
}

// startsWith is a helper function to check if a string starts with a prefix
func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
