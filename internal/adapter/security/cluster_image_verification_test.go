package security

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
)

type captureVerifier struct {
	config imageverify.VerifyConfig
	called bool
}

func (v *captureVerifier) Verify(_ context.Context, _ string, config imageverify.VerifyConfig) (string, error) {
	v.called = true
	v.config = config
	return "ghcr.io/dc-tec/openbao-operator@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
}

func TestVerifyImageForCluster_AppliesOfficialOpenBaoKeylessDefaults(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "openbao/openbao:2.4.4")
	if err != nil {
		t.Fatalf("VerifyImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
	if got := verifier.config.SubjectRegExp; got != openBaoReleaseSubjectRegExp {
		t.Fatalf("subjectRegExp = %q, want %q", got, openBaoReleaseSubjectRegExp)
	}
}

func TestVerifyImageForCluster_AppliesOfficialOpenBaoKeylessDefaultsForGHCR(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "ghcr.io/openbao/openbao:2.5.0")
	if err != nil {
		t.Fatalf("VerifyImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
	if got := verifier.config.SubjectRegExp; got != openBaoReleaseSubjectRegExp {
		t.Fatalf("subjectRegExp = %q, want %q", got, openBaoReleaseSubjectRegExp)
	}
}

func TestVerifyOperatorImageForCluster_AppliesOfficialOperatorKeylessDefaults(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyOperatorImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "ghcr.io/dc-tec/openbao-init:1.2.4")
	if err != nil {
		t.Fatalf("VerifyOperatorImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
	if got := verifier.config.SubjectRegExp; got != operatorSubjectRegExp {
		t.Fatalf("subjectRegExp = %q, want %q", got, operatorSubjectRegExp)
	}
}

func TestVerifyOperatorImageForCluster_AppliesDefaultsForEdgeTag(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyOperatorImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "ghcr.io/dc-tec/openbao-init:edge")
	if err != nil {
		t.Fatalf("VerifyOperatorImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
	if got := verifier.config.SubjectRegExp; got != operatorSubjectRegExp {
		t.Fatalf("subjectRegExp = %q, want %q", got, operatorSubjectRegExp)
	}
}

func TestOperatorSubjectRegExp_TrustedWorkflowIdentities(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(operatorSubjectRegExp)

	trusted := []string{
		"https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/v1.2.3",
		"https://github.com/dc-tec/openbao-operator/.github/workflows/publish-edge.yml@refs/heads/main",
		"https://github.com/dc-tec/openbao-operator/.github/workflows/publish-nightly.yml@refs/heads/main",
		"https://github.com/dc-tec/openbao-operator/.github/workflows/reusable-build.yml@refs/heads/main",
	}
	for _, subject := range trusted {
		if !re.MatchString(subject) {
			t.Fatalf("trusted subject %q did not match %q", subject, operatorSubjectRegExp)
		}
	}

	untrusted := []string{
		"https://github.com/dc-tec/openbao-operator/.github/workflows/reusable-build.yml@refs/heads/feature",
		"https://github.com/dc-tec/openbao-operator/.github/workflows/ci.yml@refs/heads/main",
		"https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/heads/main",
	}
	for _, subject := range untrusted {
		if re.MatchString(subject) {
			t.Fatalf("untrusted subject %q unexpectedly matched %q", subject, operatorSubjectRegExp)
		}
	}
}

func TestVerifyOperatorImageForCluster_MissingIdentityForUnknownImageReturnsError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyOperatorImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "example.com/acme/openbao-init:1.0.0")
	if err == nil {
		t.Fatal("expected an error for unknown image defaults")
	}
	if !strings.Contains(err.Error(), "neither public key nor keyless configuration") {
		t.Fatalf("unexpected error: %v", err)
	}
	if verifier.called {
		t.Fatal("verifier should not be called when configuration is incomplete")
	}
}

func TestVerifyImageForCluster_DigestReferenceAppliesOfficialDefaults(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: true,
			},
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyImageForCluster(
		context.Background(),
		logr.Discard(),
		verifier,
		cluster,
		"openbao/openbao@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	)
	if err != nil {
		t.Fatalf("VerifyImageForCluster() unexpected error for digest ref: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
	if got := verifier.config.SubjectRegExp; got != openBaoReleaseSubjectRegExp {
		t.Fatalf("subjectRegExp = %q, want %q", got, openBaoReleaseSubjectRegExp)
	}
}

func TestVerifyImageForCluster_HardenedWithOmittedConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "openbao/openbao:2.4.4")
	if err != nil {
		t.Fatalf("VerifyImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
}

func TestVerifyOperatorImageForCluster_HardenedWithOmittedConfigAppliesDefaults(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
		},
	}
	verifier := &captureVerifier{}

	_, err := VerifyOperatorImageForCluster(context.Background(), logr.Discard(), verifier, cluster, "ghcr.io/dc-tec/openbao-backup:1.2.4")
	if err != nil {
		t.Fatalf("VerifyOperatorImageForCluster() unexpected error: %v", err)
	}
	if !verifier.called {
		t.Fatal("expected verifier to be called")
	}
	if got := verifier.config.IssuerRegExp; got != defaultGitHubOIDCIssuerRegExp {
		t.Fatalf("issuerRegExp = %q, want %q", got, defaultGitHubOIDCIssuerRegExp)
	}
}

func TestIsImageVerificationEnabledHelpers(t *testing.T) {
	t.Parallel()

	hardenedImplicit := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{Profile: openbaov1alpha1.ProfileHardened},
	}
	if !IsMainImageVerificationEnabled(hardenedImplicit) {
		t.Fatal("expected main image verification to be enabled for Hardened by default")
	}
	if !IsOperatorImageVerificationEnabled(hardenedImplicit) {
		t.Fatal("expected operator image verification to be enabled for Hardened by default")
	}

	hardenedExplicitDisabled := &openbaov1alpha1.OpenBaoCluster{
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile: openbaov1alpha1.ProfileHardened,
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: false,
			},
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled: false,
			},
		},
	}
	if IsMainImageVerificationEnabled(hardenedExplicitDisabled) {
		t.Fatal("expected explicit disable to keep main image verification disabled")
	}
	if IsOperatorImageVerificationEnabled(hardenedExplicitDisabled) {
		t.Fatal("expected explicit disable to keep operator image verification disabled")
	}
}
