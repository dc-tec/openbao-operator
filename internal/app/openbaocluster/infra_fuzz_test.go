package openbaocluster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func FuzzInfraPolicyHelpers(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint8(0), true, "repo.example/openbao", "2.0.0")
	f.Add(uint8(1), uint8(1), uint8(1), uint8(1), false, "", " 2.4.4 ")

	f.Fuzz(func(t *testing.T, profileSeed, imagePolicySeed, operatorPolicySeed, unsealSeed uint8, oidcEnabled bool, repo, version string) {
		repo = sanitizeEnvValue(repo, "example.io/openbao")
		version = sanitizeInfraText(version, "2.4.4")
		t.Setenv(infraOpenBaoImageRepoEnv, repo)

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile: fuzzInfraProfile(profileSeed),
				ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
					Enabled:       imagePolicySeed%2 == 0,
					FailurePolicy: fuzzFailurePolicy(imagePolicySeed),
				},
				OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
					Enabled:       operatorPolicySeed%2 == 0,
					FailurePolicy: fuzzFailurePolicy(operatorPolicySeed),
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					Enabled: true,
					OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
						Enabled: oidcEnabled,
					},
				},
				Unseal: &openbaov1alpha1.UnsealConfig{
					Type: fuzzInfraUnseal(unsealSeed),
				},
			},
		}

		image := defaultOpenBaoImage(version)
		if !strings.Contains(image, ":") {
			t.Fatalf("expected default image %q to contain a tag separator", image)
		}

		if imageVerificationFailurePolicy(cluster) == "" {
			t.Fatalf("expected main image verification failure policy")
		}
		if operatorImageVerificationFailurePolicy(cluster) == "" {
			t.Fatalf("expected operator image verification failure policy")
		}
		_ = defaultIsMainImageVerificationEnabled(cluster)
		_ = defaultIsOperatorImageVerificationEnabled(cluster)
		_ = shouldBootstrapJWTAuth(cluster)
	})
}

func FuzzVerifyImageDigestWithPolicy(f *testing.F) {
	f.Add(true, "ghcr.io/dc-tec/openbao@sha256:abc", "Block", true, "sha256:def", "verify failed")
	f.Add(true, "ghcr.io/dc-tec/openbao:2.4.4", "Warn", false, "", "network error")
	f.Add(false, "", "Block", false, "", "")

	f.Fuzz(func(t *testing.T, enabled bool, imageRef, failurePolicy string, verifySucceeds bool, digest, verifyErr string) {
		cluster := &openbaov1alpha1.OpenBaoCluster{}
		r := &infraReconciler{}
		verifyCalls := 0
		opts := imageVerificationOptions{
			enabled:              enabled,
			imageRef:             imageRef,
			failurePolicy:        fuzzWarnOrBlock(failurePolicy),
			failureReason:        "ImageVerificationFailed",
			failureMessagePrefix: "image verification failed",
			successMessage:       "ok",
		}

		gotDigest, err := r.verifyImageDigestWithPolicy(context.Background(), logr.Discard(), cluster, opts, func(context.Context) (string, error) {
			verifyCalls++
			if verifySucceeds {
				return sanitizeInfraText(digest, "sha256:deadbeef"), nil
			}
			return "", errors.New(sanitizeInfraText(verifyErr, "verify failed"))
		})

		if !enabled || strings.TrimSpace(imageRef) == "" {
			if err != nil || gotDigest != "" {
				t.Fatalf("expected skipped verification, got digest=%q err=%v", gotDigest, err)
			}
			if verifyCalls != 0 {
				t.Fatalf("verify function should not have been called for skipped verification")
			}
			return
		}

		if verifySucceeds {
			if err != nil {
				t.Fatalf("expected no error on successful verification, got %v", err)
			}
			if gotDigest == "" {
				t.Fatalf("expected a digest on successful verification")
			}
			return
		}

		if opts.failurePolicy == defaultImageVerificationFailurePolicyBlock {
			if err == nil {
				t.Fatalf("expected blocking policy to return an error")
			}
			return
		}
		if err != nil {
			t.Fatalf("expected warn policy to suppress verification error, got %v", err)
		}
	})
}

func FuzzComputeStatefulSetSpec(f *testing.F) {
	f.Add("example", int32(3), uint8(0), uint8(0), "sha256:main", "sha256:init")
	f.Add("cluster-a", int32(0), uint8(1), uint8(2), "", "")

	f.Fuzz(func(t *testing.T, name string, replicas int32, strategySeed, phaseSeed uint8, mainDigest, initDigest string) {
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sanitizeInfraName(name, "example"),
				Namespace: "default",
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Replicas: clampInfraReplicas(replicas),
			},
		}
		if strategySeed%2 == 0 {
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{
				Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
			}
			cluster.Status.BlueGreen = &openbaov1alpha1.BlueGreenStatus{
				Phase:        fuzzInfraBlueGreenPhase(phaseSeed),
				BlueRevision: sanitizeInfraName(fmt.Sprintf("rev-%d", phaseSeed), "rev-a"),
			}
		}

		spec := (&infraReconciler{}).computeStatefulSetSpec(logr.Discard(), cluster, sanitizeInfraText(mainDigest, "sha256:main"), sanitizeInfraText(initDigest, "sha256:init"))
		if spec.Replicas != cluster.Spec.Replicas {
			t.Fatalf("replicas mismatch: got %d want %d", spec.Replicas, cluster.Spec.Replicas)
		}
		if spec.Image == "" {
			t.Fatalf("expected image to be carried into statefulset spec")
		}
		if !spec.SkipReconciliation && spec.Name == "" {
			t.Fatalf("expected statefulset spec name")
		}
		if spec.SkipReconciliation && spec.Revision == "" {
			t.Fatalf("skip reconciliation should only happen for revisioned blue/green specs")
		}
	})
}

func sanitizeEnvValue(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	trimmed = strings.ReplaceAll(trimmed, "\x00", "")
	trimmed = strings.ReplaceAll(trimmed, "=", "-")
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 80 {
		return trimmed[:80]
	}
	return trimmed
}

func sanitizeInfraText(input, fallback string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 120 {
		return trimmed[:120]
	}
	return trimmed
}

func sanitizeInfraName(input, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func clampInfraReplicas(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value % 6
}

func fuzzInfraProfile(seed uint8) openbaov1alpha1.Profile {
	if seed%2 == 0 {
		return openbaov1alpha1.ProfileHardened
	}
	return openbaov1alpha1.ProfileDevelopment
}

func fuzzInfraUnseal(seed uint8) string {
	switch seed % 3 {
	case 0:
		return ""
	case 1:
		return "transit"
	default:
		return "awskms"
	}
}

func fuzzFailurePolicy(seed uint8) string {
	if seed%2 == 0 {
		return defaultImageVerificationFailurePolicyBlock
	}
	return "Warn"
}

func fuzzWarnOrBlock(input string) string {
	if strings.EqualFold(strings.TrimSpace(input), "warn") {
		return "Warn"
	}
	return defaultImageVerificationFailurePolicyBlock
}

func fuzzInfraBlueGreenPhase(seed uint8) openbaov1alpha1.BlueGreenPhase {
	switch seed % 4 {
	case 0:
		return openbaov1alpha1.PhaseIdle
	case 1:
		return openbaov1alpha1.PhasePromoting
	case 2:
		return openbaov1alpha1.PhaseDemotingBlue
	default:
		return openbaov1alpha1.PhaseCleanup
	}
}
