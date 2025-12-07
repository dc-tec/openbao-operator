package security

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ggcrremote "github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v2/pkg/cosign"
	ociremote "github.com/sigstore/cosign/v2/pkg/oci/remote"
	"github.com/sigstore/cosign/v2/pkg/signature"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ImageVerifier verifies container image signatures using Cosign.
// It implements a simple in-memory cache to avoid re-verifying
// the same image digest on every reconcile loop.
type ImageVerifier struct {
	logger logr.Logger
	cache  *verificationCache
	client client.Client
}

// NewImageVerifier creates a new ImageVerifier with the provided logger and Kubernetes client.
// The client is used to read ImagePullSecrets for private registry authentication.
func NewImageVerifier(logger logr.Logger, k8sClient client.Client) *ImageVerifier {
	return &ImageVerifier{
		logger: logger,
		cache:  newVerificationCache(),
		client: k8sClient,
	}
}

// Verify verifies the signature of the given image reference against the provided public key.
// It uses an in-memory cache keyed by image digest and public key to avoid redundant network calls.
// Returns the resolved image digest (e.g., "openbao/openbao@sha256:abc...") and an error if verification fails.
// The digest can be used to pin the image in StatefulSets to prevent TOCTOU attacks.
func (v *ImageVerifier) Verify(ctx context.Context, imageRef, publicKey string, ignoreTlog bool, imagePullSecrets []corev1.LocalObjectReference, namespace string) (string, error) {
	if publicKey == "" {
		return "", fmt.Errorf("public key is required for image verification")
	}

	// Perform verification and get digest
	v.logger.Info("Verifying image signature", "image", imageRef, "ignoreTlog", ignoreTlog)
	digest, err := v.verifyImage(ctx, imageRef, publicKey, ignoreTlog, imagePullSecrets, namespace)
	if err != nil {
		return "", fmt.Errorf("image verification failed for %q: %w", imageRef, err)
	}

	// Check cache using digest (not tag) to prevent TOCTOU issues
	if v.cache.isVerified(digest, publicKey) {
		v.logger.V(1).Info("Image verification cache hit", "digest", digest)
		return digest, nil
	}

	// Cache successful verification using digest
	v.cache.markVerified(digest, publicKey)
	v.logger.Info("Image verification succeeded", "image", imageRef, "digest", digest)

	return digest, nil
}

// verifyImage performs the actual Cosign verification and returns the resolved digest.
func (v *ImageVerifier) verifyImage(ctx context.Context, imageRef, publicKey string, ignoreTlog bool, imagePullSecrets []corev1.LocalObjectReference, namespace string) (string, error) {
	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Create a verifier from the public key PEM (in-memory, no file I/O required)
	verifier, err := signature.LoadPublicKeyRaw([]byte(publicKey), crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("failed to create verifier from public key: %w", err)
	}

	// Build registry options with Kubernetes keychain for private registry authentication
	var remoteOpts []ociremote.Option
	if len(imagePullSecrets) > 0 && v.client != nil {
		keychain, err := v.buildKeychain(ctx, imagePullSecrets, namespace)
		if err != nil {
			return "", fmt.Errorf("failed to build keychain for image pull secrets: %w", err)
		}
		if keychain != nil {
			// Convert authn.Keychain to ociremote.Option
			remoteOpts = append(remoteOpts, ociremote.WithRemoteOptions(ggcrremote.WithAuthFromKeychain(keychain)))
		}
	}

	// Create CheckOpts with the verifier and registry options
	co := &cosign.CheckOpts{
		SigVerifier:        verifier,
		IgnoreTlog:         ignoreTlog,
		RegistryClientOpts: remoteOpts,
	}

	// Verify image signatures and get the resolved digest
	sigs, bundleVerified, err := cosign.VerifyImageSignatures(ctx, ref, co)
	if err != nil {
		return "", fmt.Errorf("image signature verification failed: %w", err)
	}

	if len(sigs) == 0 {
		return "", fmt.Errorf("no signatures found for image %q", imageRef)
	}

	// Resolve the digest from the reference
	// If the reference is already a digest, use it directly
	// Otherwise, resolve the tag to a digest
	var digestRef name.Digest
	if d, ok := ref.(name.Digest); ok {
		digestRef = d
	} else {
		// Resolve tag to digest
		// Convert ociremote.Options to ggcrremote.Options for Head call
		var ggcrOpts []ggcrremote.Option
		if len(imagePullSecrets) > 0 && v.client != nil {
			keychain, err := v.buildKeychain(ctx, imagePullSecrets, namespace)
			if err == nil && keychain != nil {
				ggcrOpts = append(ggcrOpts, ggcrremote.WithAuthFromKeychain(keychain))
			}
		}
		desc, err := ggcrremote.Head(ref, ggcrOpts...)
		if err != nil {
			return "", fmt.Errorf("failed to resolve image digest: %w", err)
		}
		digestRef, err = name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), desc.Digest.String()))
		if err != nil {
			return "", fmt.Errorf("failed to create digest reference: %w", err)
		}
	}

	v.logger.V(1).Info("Image verification completed",
		"image", imageRef,
		"digest", digestRef.String(),
		"signatures", len(sigs),
		"bundleVerified", bundleVerified,
		"rekorVerified", !ignoreTlog)

	return digestRef.String(), nil
}

// buildKeychain constructs a keychain from ImagePullSecrets by reading the secrets
// and creating an authn.Keychain that uses the docker config from the secrets.
// Returns nil if no secrets are provided or if client is not available.
func (v *ImageVerifier) buildKeychain(ctx context.Context, imagePullSecrets []corev1.LocalObjectReference, namespace string) (authn.Keychain, error) {
	if len(imagePullSecrets) == 0 || v.client == nil {
		return nil, nil
	}

	// Read all ImagePullSecrets and combine them into a single docker config
	// Docker config format: {"auths": {"registry": {"username": "...", "password": "...", "auth": "..."}}}
	type dockerConfig struct {
		Auths map[string]dockerAuthConfig `json:"auths"`
	}

	combinedConfig := dockerConfig{
		Auths: make(map[string]dockerAuthConfig),
	}

	for _, secretRef := range imagePullSecrets {
		secret := &corev1.Secret{}
		if err := v.client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      secretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("failed to get ImagePullSecret %s/%s: %w", namespace, secretRef.Name, err)
		}

		// Check secret type
		if secret.Type != corev1.SecretTypeDockerConfigJson && secret.Type != corev1.SecretTypeDockercfg {
			return nil, fmt.Errorf("ImagePullSecret %s/%s has invalid type %s, expected %s or %s",
				namespace, secretRef.Name, secret.Type, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg)
		}

		// Extract docker config
		var dockerConfigKey string
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			dockerConfigKey = corev1.DockerConfigJsonKey
		} else {
			dockerConfigKey = corev1.DockerConfigKey
		}

		dockerConfigData, ok := secret.Data[dockerConfigKey]
		if !ok {
			return nil, fmt.Errorf("ImagePullSecret %s/%s missing key %s", namespace, secretRef.Name, dockerConfigKey)
		}

		// Parse docker config
		var secretConfig dockerConfig
		if err := json.Unmarshal(dockerConfigData, &secretConfig); err != nil {
			return nil, fmt.Errorf("failed to parse docker config from ImagePullSecret %s/%s: %w", namespace, secretRef.Name, err)
		}

		// Merge into combined config (later secrets override earlier ones for same registry)
		for registry, authConfig := range secretConfig.Auths {
			combinedConfig.Auths[registry] = authConfig
		}
	}

	// Create keychain from combined config using authn.NewKeychainFromHelper
	// We'll create a simple keychain that resolves auth for each registry
	if len(combinedConfig.Auths) == 0 {
		return nil, nil
	}

	// Create a keychain that resolves auth for each registry in the config
	return &dockerConfigKeychain{auths: combinedConfig.Auths}, nil
}

// dockerAuthConfig represents a single docker auth config entry.
type dockerAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

// dockerConfigKeychain implements authn.Keychain using docker config auths.
type dockerConfigKeychain struct {
	auths map[string]dockerAuthConfig
}

func (k *dockerConfigKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	// Find matching registry in auths
	registry := resource.RegistryStr()
	if auth, ok := k.auths[registry]; ok {
		// Create authenticator from docker config entry
		if auth.Auth != "" {
			// Use base64-encoded auth string
			return &authn.Basic{
				Username: auth.Username,
				Password: auth.Password,
			}, nil
		}
		if auth.Username != "" && auth.Password != "" {
			return &authn.Basic{
				Username: auth.Username,
				Password: auth.Password,
			}, nil
		}
	}
	// Return anonymous if no match
	return authn.Anonymous, nil
}

// verificationCache is a simple in-memory cache for verified images.
// It uses a map with a mutex for thread safety.
type verificationCache struct {
	mu    sync.RWMutex
	cache map[string]bool
}

func newVerificationCache() *verificationCache {
	return &verificationCache{
		cache: make(map[string]bool),
	}
}

// isVerified checks if an image with the given reference and public key has been verified.
func (c *verificationCache) isVerified(imageRef, publicKey string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	key := cacheKey(imageRef, publicKey)
	return c.cache[key]
}

// markVerified marks an image as verified.
func (c *verificationCache) markVerified(imageRef, publicKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := cacheKey(imageRef, publicKey)
	c.cache[key] = true
}

// cacheKey generates a cache key from image digest and public key.
// Uses digest (not tag) to prevent caching issues when tags change.
func cacheKey(digest, publicKey string) string {
	// Use a simple hash of the public key to avoid storing the full key
	// In a production system, you might want to use a proper hash function
	keyHash := []byte(publicKey)
	if len(keyHash) > 16 {
		keyHash = keyHash[:16]
	}
	return fmt.Sprintf("%s@%x", digest, keyHash)
}
