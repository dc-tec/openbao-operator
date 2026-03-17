package security

import (
	"context"
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	"github.com/go-logr/logr"
)

type fuzzCaptureVerifier struct{}

func (f fuzzCaptureVerifier) Verify(_ context.Context, imageRef string, _ imageverify.VerifyConfig) (string, error) {
	return imageRef + "@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
}

func FuzzImageRepository(f *testing.F) {
	seeds := []string{
		"",
		"openbao/openbao:2.4.4",
		"openbao/openbao@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"ghcr.io/dc-tec/openbao-init:edge",
		"docker.io/openbao/openbao:latest",
		"not a ref",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, imageRef string) {
		if len(imageRef) > 2048 {
			t.Skip()
		}

		repo, ok := imageRepository(imageRef)
		if !ok {
			return
		}

		if strings.HasPrefix(repo, "docker.io/") || strings.HasPrefix(repo, "index.docker.io/") {
			t.Fatalf("repository %q was not normalized", repo)
		}

		_ = defaultSubjectRegExpForImage(repo, false)
		_ = defaultSubjectRegExpForImage(repo, true)
	})
}

func FuzzVerifyImageForCluster(f *testing.F) {
	seeds := []struct {
		imageRef      string
		namespace     string
		profile       string
		enableMain    bool
		enableOp      bool
		useOperatorFn bool
	}{
		{"openbao/openbao:2.4.4", "default", string(openbaov1alpha1.ProfileHardened), true, true, false},
		{"ghcr.io/dc-tec/openbao-init:edge", "operators", string(openbaov1alpha1.ProfileHardened), true, true, true},
		{"example.com/custom/image:1.0.0", "default", string(openbaov1alpha1.ProfileDevelopment), true, true, false},
		{"not a ref", "default", string(openbaov1alpha1.ProfileDevelopment), true, true, false},
	}
	for _, seed := range seeds {
		f.Add(seed.imageRef, seed.namespace, seed.profile, seed.enableMain, seed.enableOp, seed.useOperatorFn)
	}

	f.Fuzz(func(t *testing.T, imageRef, namespace, profile string, enableMain, enableOp, useOperatorFn bool) {
		if len(imageRef) > 2048 || len(namespace) > 128 || len(profile) > 64 {
			t.Skip()
		}

		cluster := &openbaov1alpha1.OpenBaoCluster{
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile: openbaov1alpha1.Profile(profile),
				ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
					Enabled: enableMain,
				},
				OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
					Enabled: enableOp,
				},
			},
		}
		cluster.Namespace = namespace

		verifier := fuzzCaptureVerifier{}
		if useOperatorFn {
			_, _ = VerifyOperatorImageForCluster(context.Background(), logr.Discard(), verifier, cluster, imageRef)
			return
		}
		_, _ = VerifyImageForCluster(context.Background(), logr.Discard(), verifier, cluster, imageRef)
	})
}
