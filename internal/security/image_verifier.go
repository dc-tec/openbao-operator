package security

import (
	"context"
	"crypto"
	_ "embed" // Required for go:embed
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ggcrremote "github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/sigstore/cosign/v3/pkg/signature"
	"github.com/sigstore/sigstore-go/pkg/root"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

//go:embed trusted_root.json
var embeddedTrustedRootJSON []byte

// VerifyConfig holds the configuration for image verification.
// Provide PublicKey for static verification OR Issuer/Subject for keyless.
type VerifyConfig struct {
	PublicKey        string
	Issuer           string
	Subject          string
	IgnoreTlog       bool
	ImagePullSecrets []corev1.LocalObjectReference
	Namespace        string
}

// ImageVerifier verifies container image signatures using Cosign.
// It implements a simple in-memory cache to avoid re-verifying
// the same image digest on every reconcile loop.
type ImageVerifier struct {
	logger            logr.Logger
	cache             *verificationCache
	client            client.Client
	trustedRootConfig *TrustedRootConfig
}

// TrustedRootConfig specifies where to load the trusted root material from.
// If ConfigMapName and ConfigMapNamespace are set, the trusted root will be
// loaded from that ConfigMap (key: "trusted_root.json"). Otherwise, the
// embedded trusted_root.json will be used.
type TrustedRootConfig struct {
	ConfigMapName      string
	ConfigMapNamespace string
}

// NewImageVerifier creates a new ImageVerifier with the provided logger and Kubernetes client.
// The client is used to read ImagePullSecrets for private registry authentication.
// trustedRootConfig is optional - if provided, the trusted root will be loaded from the
// specified ConfigMap instead of using the embedded version.
func NewImageVerifier(logger logr.Logger, k8sClient client.Client, trustedRootConfig *TrustedRootConfig) *ImageVerifier {
	return &ImageVerifier{
		logger:            logger,
		cache:             newVerificationCache(),
		client:            k8sClient,
		trustedRootConfig: trustedRootConfig,
	}
}

// Verify verifies the signature of the given image reference using the provided configuration.
// It uses an in-memory cache keyed by image digest and verification config to avoid redundant network calls.
// Returns the resolved image digest (e.g., "openbao/openbao@sha256:abc...") and an error if verification fails.
// The digest can be used to pin the image in StatefulSets to prevent TOCTOU attacks.
func (v *ImageVerifier) Verify(ctx context.Context, imageRef string, config VerifyConfig) (string, error) {
	// Validate that either PublicKey OR (Issuer and Subject) are provided
	if config.PublicKey == "" && (config.Issuer == "" || config.Subject == "") {
		return "", fmt.Errorf("either PublicKey OR (Issuer and Subject) must be provided for image verification")
	}

	// Step 1: Resolve tag to digest (cheap HEAD request)
	// This must happen BEFORE cache lookup to get the digest for the cache key
	digest, err := v.resolveDigest(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	// Step 2: Check cache BEFORE expensive cryptographic verification
	cacheKey := v.cacheKey(digest, config)
	if v.cache.isVerifiedByKey(cacheKey) {
		v.logger.V(1).Info("Image verification cache hit", "digest", digest)
		return digest, nil
	}

	// Step 3: Cache miss - perform expensive Cosign signature verification
	mode := "static-key"
	if config.PublicKey == "" {
		mode = "keyless"
	}
	v.logger.Info("Verifying image signature", "image", imageRef, "digest", digest, "mode", mode, "ignoreTlog", config.IgnoreTlog)
	if err := v.verifyImageSignature(ctx, digest, config); err != nil {
		return "", fmt.Errorf("image verification failed for %q: %w", imageRef, err)
	}

	// Step 4: Cache successful verification using digest
	v.cache.markVerifiedByKey(cacheKey)
	v.logger.Info("Image verification succeeded", "image", imageRef, "digest", digest)

	return digest, nil
}

// resolveDigest resolves an image reference (tag or digest) to a digest reference.
// For digest references, it returns them directly.
// For tag references, it performs a HEAD request to resolve the tag to a digest.
// This is a cheap operation that only fetches manifest metadata, not the full image.
func (v *ImageVerifier) resolveDigest(ctx context.Context, imageRef string, config VerifyConfig) (string, error) {
	// Parse the image reference
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("failed to parse image reference: %w", err)
	}

	// If already a digest reference, return it directly
	if d, ok := ref.(name.Digest); ok {
		return d.String(), nil
	}

	// Build registry options with Kubernetes keychain for private registry authentication
	var ggcrOpts []ggcrremote.Option
	if len(config.ImagePullSecrets) > 0 && v.client != nil {
		keychain, err := v.buildKeychain(ctx, config.ImagePullSecrets, config.Namespace)
		if err != nil {
			return "", fmt.Errorf("failed to build keychain for image pull secrets: %w", err)
		}
		if keychain != nil {
			ggcrOpts = append(ggcrOpts, ggcrremote.WithAuthFromKeychain(keychain))
		}
	}

	// Resolve tag to digest via HEAD request
	desc, err := ggcrremote.Head(ref, ggcrOpts...)
	if err != nil {
		// Wrap connection/network errors as transient
		if operatorerrors.IsTransientConnection(err) {
			return "", operatorerrors.WrapTransientConnection(fmt.Errorf("failed to resolve image digest: %w", err))
		}
		return "", fmt.Errorf("failed to resolve image digest: %w", err)
	}

	digestRef, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), desc.Digest.String()))
	if err != nil {
		return "", fmt.Errorf("failed to create digest reference: %w", err)
	}

	return digestRef.String(), nil
}

// verifyImageSignature performs the actual Cosign signature verification on an already-resolved digest.
// The digest resolution is handled separately by resolveDigest() to enable cache-first checking.
func (v *ImageVerifier) verifyImageSignature(ctx context.Context, digestRef string, config VerifyConfig) error {
	// Parse the digest reference
	ref, err := name.ParseReference(digestRef)
	if err != nil {
		return fmt.Errorf("failed to parse digest reference: %w", err)
	}

	// Build registry options with Kubernetes keychain for private registry authentication
	var remoteOpts []ociremote.Option
	if len(config.ImagePullSecrets) > 0 && v.client != nil {
		keychain, err := v.buildKeychain(ctx, config.ImagePullSecrets, config.Namespace)
		if err != nil {
			return fmt.Errorf("failed to build keychain for image pull secrets: %w", err)
		}
		if keychain != nil {
			// Convert authn.Keychain to ociremote.Option
			remoteOpts = append(remoteOpts, ociremote.WithRemoteOptions(ggcrremote.WithAuthFromKeychain(keychain)))
		}
	}

	// Create CheckOpts based on verification mode
	co := &cosign.CheckOpts{
		RegistryClientOpts: remoteOpts,
	}

	// Mode 1: Static Public Key (Custom Images)
	if config.PublicKey != "" {
		verifier, err := signature.LoadPublicKeyRaw([]byte(config.PublicKey), crypto.SHA256)
		if err != nil {
			return fmt.Errorf("failed to load public key: %w", err)
		}
		co.SigVerifier = verifier
		co.IgnoreTlog = config.IgnoreTlog
	} else {
		// Mode 2: Keyless / Identity (Official OpenBao Images)
		// Load trusted root material (Fulcio root certificates, Rekor public keys, etc.)
		// First attempts to load from ConfigMap if configured, otherwise uses embedded
		// trusted_root.json which is compiled into the binary at build time.
		trustedRoot, err := v.loadTrustedRoot(ctx)
		if err != nil {
			return fmt.Errorf("failed to load trusted root material for keyless verification: %w", err)
		}
		co.TrustedMaterial = trustedRoot
		co.Identities = []cosign.Identity{
			{
				Issuer:  config.Issuer,
				Subject: config.Subject,
			},
		}
		// IMPORTANT: For keyless, we MUST verify against the Transparency Log (Rekor).
		// Do NOT set IgnoreTlog = true here, as it would bypass the security guarantees of keyless verification.
		co.IgnoreTlog = false
	}

	// Verify image signatures
	sigs, bundleVerified, err := cosign.VerifyImageSignatures(ctx, ref, co)
	if err != nil {
		// Wrap connection/network errors as transient (registry connection issues)
		if operatorerrors.IsTransientConnection(err) {
			return operatorerrors.WrapTransientConnection(fmt.Errorf("image signature verification failed: %w", err))
		}
		return fmt.Errorf("image signature verification failed: %w", err)
	}

	if len(sigs) == 0 {
		return fmt.Errorf("no signatures found for image %q", digestRef)
	}

	v.logger.V(1).Info("Image signature verification completed",
		"digest", digestRef,
		"signatures", len(sigs),
		"bundleVerified", bundleVerified,
		"rekorVerified", !co.IgnoreTlog)

	return nil
}

// loadTrustedRoot loads the trusted root material for keyless verification.
// It first attempts to load from a ConfigMap if configured, otherwise falls back
// to the embedded trusted_root.json. The trusted root includes Fulcio root certificates,
// Rekor public keys, and CT log keys.
func (v *ImageVerifier) loadTrustedRoot(ctx context.Context) (root.TrustedMaterial, error) {
	// Try loading from ConfigMap if configured
	if v.trustedRootConfig != nil && v.trustedRootConfig.ConfigMapName != "" && v.trustedRootConfig.ConfigMapNamespace != "" {
		if v.client != nil {
			configMap := &corev1.ConfigMap{}
			err := v.client.Get(ctx, types.NamespacedName{
				Namespace: v.trustedRootConfig.ConfigMapNamespace,
				Name:      v.trustedRootConfig.ConfigMapName,
			}, configMap)
			if err == nil {
				// ConfigMap found - try to read trusted_root.json key
				if trustedRootJSON, ok := configMap.Data["trusted_root.json"]; ok {
					v.logger.Info("Loading trusted root from ConfigMap",
						"configmap", v.trustedRootConfig.ConfigMapName,
						"namespace", v.trustedRootConfig.ConfigMapNamespace)
					trustedRoot, err := root.NewTrustedRootFromJSON([]byte(trustedRootJSON))
					if err != nil {
						return nil, fmt.Errorf("failed to parse trusted_root.json from ConfigMap %s/%s: %w",
							v.trustedRootConfig.ConfigMapNamespace, v.trustedRootConfig.ConfigMapName, err)
					}
					return trustedRoot, nil
				}
				v.logger.Info("ConfigMap found but missing 'trusted_root.json' key, falling back to embedded",
					"configmap", v.trustedRootConfig.ConfigMapName,
					"namespace", v.trustedRootConfig.ConfigMapNamespace)
			} else {
				v.logger.V(1).Info("Failed to load ConfigMap, falling back to embedded trusted root",
					"configmap", v.trustedRootConfig.ConfigMapName,
					"namespace", v.trustedRootConfig.ConfigMapNamespace,
					"error", err)
			}
		}
	}

	// Fallback to embedded trusted root
	v.logger.V(1).Info("Using embedded trusted_root.json")
	trustedRoot, err := root.NewTrustedRootFromJSON(embeddedTrustedRootJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded trusted_root.json: %w", err)
	}

	return trustedRoot, nil
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

// isVerifiedByKey checks if an image has been verified using the provided cache key.
func (c *verificationCache) isVerifiedByKey(cacheKey string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cache[cacheKey]
}

// markVerifiedByKey marks an image as verified using the provided cache key.
func (c *verificationCache) markVerifiedByKey(cacheKey string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[cacheKey] = true
}

// cacheKey generates a cache key from image digest and verification config.
// Uses digest (not tag) to prevent caching issues when tags change.
// Supports both static key and keyless verification modes.
func (v *ImageVerifier) cacheKey(digest string, config VerifyConfig) string {
	if config.PublicKey != "" {
		// Static key mode: use hash of public key
		keyHash := []byte(config.PublicKey)
		if len(keyHash) > 16 {
			keyHash = keyHash[:16]
		}
		return fmt.Sprintf("%s@key:%x", digest, keyHash)
	}
	// Keyless mode: use issuer and subject
	return fmt.Sprintf("%s@oidc:%s|%s", digest, config.Issuer, config.Subject)
}
