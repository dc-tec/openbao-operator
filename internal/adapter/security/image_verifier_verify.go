package security

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	ggcrremote "github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/v3/pkg/cosign"
	"github.com/sigstore/cosign/v3/pkg/oci"
	ociremote "github.com/sigstore/cosign/v3/pkg/oci/remote"
	"github.com/sigstore/cosign/v3/pkg/signature"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

func (v *ImageVerifier) verifyImageSignature(ctx context.Context, digestRef string, config imageverify.VerifyConfig) error {
	ref, err := name.ParseReference(digestRef)
	if err != nil {
		return fmt.Errorf("failed to parse digest reference: %w", err)
	}

	var remoteOpts []ociremote.Option
	if len(config.ImagePullSecrets) > 0 && v.client != nil {
		keychain, err := v.buildKeychain(ctx, config.ImagePullSecrets, config.Namespace)
		if err != nil {
			return fmt.Errorf("failed to build keychain for image pull secrets: %w", err)
		}
		if keychain != nil {
			remoteOpts = append(remoteOpts, ociremote.WithRemoteOptions(ggcrremote.WithAuthFromKeychain(keychain)))
		}
	}

	legacyCheckOpts, err := v.buildCheckOpts(ctx, config, remoteOpts, false)
	if err != nil {
		return err
	}

	sigs, bundleVerified, legacyErr := cosign.VerifyImageSignatures(ctx, ref, legacyCheckOpts)
	if legacyErr == nil && len(sigs) > 0 {
		v.logger.V(1).Info("Image signature verification completed",
			"digest", digestRef,
			"signatures", len(sigs),
			"bundleVerified", bundleVerified,
			"rekorVerified", !legacyCheckOpts.IgnoreTlog,
			"format", "legacy")
		return nil
	}
	if legacyErr == nil {
		legacyErr = fmt.Errorf("no signatures found for image %q", digestRef)
	}

	if !shouldAttemptBundleFallback(legacyErr) {
		if operatorerrors.IsTransientConnection(legacyErr) {
			return operatorerrors.WrapTransientConnection(fmt.Errorf("image signature verification failed: %w", legacyErr))
		}
		return fmt.Errorf("image signature verification failed: %w", legacyErr)
	}

	if err := v.verifyImageSignatureWithBundles(ctx, ref, digestRef, config, remoteOpts); err != nil {
		return fmt.Errorf("image signature verification failed (legacy+bundle): legacy=%v; bundle=%w", legacyErr, err)
	}

	return nil
}

func (v *ImageVerifier) buildCheckOpts(
	ctx context.Context,
	config imageverify.VerifyConfig,
	remoteOpts []ociremote.Option,
	newBundleFormat bool,
) (*cosign.CheckOpts, error) {
	co := &cosign.CheckOpts{
		RegistryClientOpts: remoteOpts,
		NewBundleFormat:    newBundleFormat,
	}

	if config.PublicKey != "" {
		verifier, err := signature.LoadPublicKeyRaw([]byte(config.PublicKey), crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("failed to load public key: %w", err)
		}
		co.SigVerifier = verifier
		co.IgnoreTlog = config.IgnoreTlog
		if !config.IgnoreTlog {
			trustedRoot, err := v.loadTrustedRoot(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to load trusted root material for transparency log verification: %w", err)
			}
			co.TrustedMaterial = trustedRoot
		}
	} else {
		trustedRoot, err := v.loadTrustedRoot(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load trusted root material for keyless verification: %w", err)
		}
		co.TrustedMaterial = trustedRoot
		identity := cosign.Identity{}
		if hasStrictKeylessConfig(config) {
			identity.Issuer = config.Issuer
			identity.Subject = config.Subject
		} else {
			identity.IssuerRegExp = config.IssuerRegExp
			identity.SubjectRegExp = config.SubjectRegExp
		}
		co.Identities = []cosign.Identity{identity}
		co.IgnoreTlog = false
	}

	if newBundleFormat {
		co.ClaimVerifier = cosign.IntotoSubjectClaimVerifier
	}

	return co, nil
}

func shouldAttemptBundleFallback(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), noSignaturesFoundErrorFragment)
}

func (v *ImageVerifier) verifyImageSignatureWithBundles(
	ctx context.Context,
	ref name.Reference,
	digestRef string,
	config imageverify.VerifyConfig,
	remoteOpts []ociremote.Option,
) error {
	bundleCheckOpts, err := v.buildCheckOpts(ctx, config, remoteOpts, true)
	if err != nil {
		return err
	}

	attestations, bundleVerified, err := cosign.VerifyImageAttestations(ctx, ref, bundleCheckOpts)
	if err != nil {
		return err
	}
	if len(attestations) == 0 {
		return fmt.Errorf("no bundle attestations found for image %q", digestRef)
	}

	signatureCount, observedPredicates, err := countBundleSignaturePredicates(attestations)
	if err != nil {
		return fmt.Errorf("failed to inspect bundle attestations: %w", err)
	}
	if signatureCount == 0 {
		return fmt.Errorf(
			"no signature bundle attestations found for image %q (expected predicate %q, observed=%s)",
			digestRef,
			cosignSignaturePredicateTypeV1,
			strings.Join(observedPredicates, ","),
		)
	}

	v.logger.V(1).Info("Image signature verification completed",
		"digest", digestRef,
		"attestations", len(attestations),
		"signatureAttestations", signatureCount,
		"bundleVerified", bundleVerified,
		"rekorVerified", !bundleCheckOpts.IgnoreTlog,
		"format", "bundle")

	return nil
}

type dsseEnvelope struct {
	PayloadType string `json:"payloadType"`
	Payload     string `json:"payload"`
}

type inTotoStatement struct {
	PredicateType string `json:"predicateType"`
}

func countBundleSignaturePredicates(attestations []oci.Signature) (int, []string, error) {
	signatureCount := 0
	observedPredicates := make([]string, 0, len(attestations))
	for _, attestation := range attestations {
		predicateType, err := bundlePredicateType(attestation)
		if err != nil {
			return 0, nil, err
		}
		observedPredicates = append(observedPredicates, predicateType)
		if predicateType == cosignSignaturePredicateTypeV1 {
			signatureCount++
		}
	}

	return signatureCount, observedPredicates, nil
}

func bundlePredicateType(attestation oci.Signature) (string, error) {
	payload, err := attestation.Payload()
	if err != nil {
		return "", fmt.Errorf("failed to read attestation payload: %w", err)
	}

	envelope := dsseEnvelope{}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return "", fmt.Errorf("failed to parse DSSE envelope: %w", err)
	}
	if strings.TrimSpace(envelope.PayloadType) != dsseInTotoPayloadType {
		return "", fmt.Errorf("unexpected DSSE payload type %q", envelope.PayloadType)
	}
	if strings.TrimSpace(envelope.Payload) == "" {
		return "", fmt.Errorf("DSSE envelope payload is empty")
	}

	statementBytes, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode DSSE payload: %w", err)
	}

	statement := inTotoStatement{}
	if err := json.Unmarshal(statementBytes, &statement); err != nil {
		return "", fmt.Errorf("failed to parse in-toto statement payload: %w", err)
	}

	predicateType := strings.TrimSpace(statement.PredicateType)
	if predicateType == "" {
		return "", fmt.Errorf("in-toto statement predicateType is empty")
	}

	return predicateType, nil
}
