package security

import (
	"context"
	_ "embed" // Required for go:embed
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

//go:embed trusted_root.json
var embeddedTrustedRootJSON []byte

const (
	noSignaturesFoundErrorFragment = "no signatures found"
	dsseInTotoPayloadType          = "application/vnd.in-toto+json"
	cosignSignaturePredicateTypeV1 = "https://sigstore.dev/cosign/sign/v1"
)

// ImageVerifier verifies container image signatures using Cosign.
// It implements two caches to minimize network I/O:
//   - tagCache: TTL-based cache for tag→digest resolution (avoids HEAD requests)
//   - cache: LRU cache for verified digests (avoids signature verification)
type ImageVerifier struct {
	logger            logr.Logger
	cache             *verificationCache
	tagCache          *tagResolutionCache
	client            client.Client
	trustedRootConfig *TrustedRootConfig
}

// TrustedRootConfig specifies where to load the trusted root material from.
// If either ConfigMapName or ConfigMapNamespace is set, both values are required
// and the trusted root must be loaded from that ConfigMap (key: "trusted_root.json").
// When neither value is set, the embedded trusted_root.json is used.
type TrustedRootConfig struct {
	ConfigMapName      string
	ConfigMapNamespace string
}

// NewImageVerifier creates a new ImageVerifier with the provided logger and Kubernetes client.
// The client is used to read ImagePullSecrets for private registry authentication.
// trustedRootConfig is optional. When it names a ConfigMap, missing or invalid
// ConfigMap data fails closed instead of falling back to the embedded root.
func NewImageVerifier(logger logr.Logger, k8sClient client.Client, trustedRootConfig *TrustedRootConfig) *ImageVerifier {
	return &ImageVerifier{
		logger:            logger,
		cache:             newVerificationCache(),
		tagCache:          newTagResolutionCache(),
		client:            k8sClient,
		trustedRootConfig: trustedRootConfig,
	}
}

// Verify verifies the signature of the given image reference using the provided configuration.
// It uses two caches to minimize network I/O:
//   - Tag resolution cache: TTL-based cache to avoid repeated HEAD requests for tag→digest resolution
//   - Verification cache: LRU cache to avoid repeated signature verification for the same digest
//
// Returns the resolved image digest (e.g., "openbao/openbao@sha256:abc...") and an error if verification fails.
// The digest can be used to pin the image in StatefulSets to prevent TOCTOU attacks.
func (v *ImageVerifier) Verify(ctx context.Context, imageRef string, config imageverify.VerifyConfig) (string, error) {
	// Validate that either PublicKey OR keyless identity is provided.
	if config.PublicKey == "" && !hasKeylessConfig(config) {
		return "", fmt.Errorf("either PublicKey OR keyless identity (Issuer/Subject or IssuerRegExp/SubjectRegExp) must be provided for image verification")
	}

	// Step 1: Resolve tag to digest
	// For digest references, this returns immediately without network I/O.
	// For tag references, we first check the TTL cache to avoid HEAD requests on every reconcile.
	digest, err := v.resolveDigestWithCache(ctx, imageRef, config)
	if err != nil {
		return "", err
	}

	// Step 2: Check verification cache BEFORE expensive cryptographic verification
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
