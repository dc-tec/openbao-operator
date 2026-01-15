package openbaocluster

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
	security "github.com/dc-tec/openbao-operator/internal/security"
)

func TestInfraReconcilerVerifyMainImageDigest_DisabledDoesNotCallVerifier(t *testing.T) {
	t.Parallel()

	called := 0
	r := &infraReconciler{
		verifyImageFunc: func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
			called++
			return "", nil
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "example/openbao:test",
		},
	}

	digest, err := r.verifyMainImageDigest(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("verifyMainImageDigest() error = %v, want nil", err)
	}
	if digest != "" {
		t.Fatalf("verifyMainImageDigest() digest = %q, want empty", digest)
	}
	if called != 0 {
		t.Fatalf("verifyImageFunc called %d times, want 0", called)
	}
}

func TestInfraReconcilerVerifyMainImageDigest_BlockReturnsReasonedError(t *testing.T) {
	t.Parallel()

	r := &infraReconciler{
		verifyImageFunc: func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
			return "", errors.New("verification failed")
		},
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "example/openbao:test",
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: constants.ImageVerificationFailurePolicyBlock,
			},
		},
	}

	_, err := r.verifyMainImageDigest(context.Background(), logr.Discard(), cluster)
	if err == nil {
		t.Fatalf("verifyMainImageDigest() error = nil, want non-nil")
	}
	if reason, ok := operatorerrors.Reason(err); !ok || reason != constants.ReasonImageVerificationFailed {
		t.Fatalf("verifyMainImageDigest() reason = (%q,%t), want (%q,true)", reason, ok, constants.ReasonImageVerificationFailed)
	}
}

func TestInfraReconcilerVerifyMainImageDigest_WarnEmitsEvent(t *testing.T) {
	t.Parallel()

	recorder := record.NewFakeRecorder(1)
	r := &infraReconciler{
		verifyImageFunc: func(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
			return "", errors.New("verification failed")
		},
		recorder: recorder,
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "example/openbao:test",
			ImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: constants.ImageVerificationFailurePolicyWarn,
			},
		},
	}

	digest, err := r.verifyMainImageDigest(context.Background(), logr.Discard(), cluster)
	if err != nil {
		t.Fatalf("verifyMainImageDigest() error = %v, want nil", err)
	}
	if digest != "" {
		t.Fatalf("verifyMainImageDigest() digest = %q, want empty", digest)
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, constants.ReasonImageVerificationFailed) {
			t.Fatalf("event %q missing reason %q", evt, constants.ReasonImageVerificationFailed)
		}
		if !strings.Contains(evt, "Image verification failed but proceeding due to Warn policy") {
			t.Fatalf("event %q missing expected message", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected an event but none was emitted")
	}
}

func TestInfraReconcilerVerifyOperatorImageDigest_WarnEmitsEvent(t *testing.T) {
	t.Parallel()

	recorder := record.NewFakeRecorder(1)
	r := &infraReconciler{
		verifyOperatorImageFunc: func(ctx context.Context, logger logr.Logger, verifier *security.ImageVerifier, cluster *openbaov1alpha1.OpenBaoCluster, imageRef string) (string, error) {
			return "", errors.New("verification failed")
		},
		recorder: recorder,
	}

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "c1", Namespace: "ns1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Image: "example/openbao:test",
			OperatorImageVerification: &openbaov1alpha1.ImageVerificationConfig{
				Enabled:       true,
				FailurePolicy: constants.ImageVerificationFailurePolicyWarn,
			},
		},
	}

	digest, err := r.verifyOperatorImageDigest(context.Background(), logr.Discard(), cluster, "example/init:test", constants.ReasonInitContainerImageVerificationFailed, "Init container image verification failed")
	if err != nil {
		t.Fatalf("verifyOperatorImageDigest() error = %v, want nil", err)
	}
	if digest != "" {
		t.Fatalf("verifyOperatorImageDigest() digest = %q, want empty", digest)
	}

	select {
	case evt := <-recorder.Events:
		if !strings.Contains(evt, constants.ReasonInitContainerImageVerificationFailed) {
			t.Fatalf("event %q missing reason %q", evt, constants.ReasonInitContainerImageVerificationFailed)
		}
		if !strings.Contains(evt, "Init container image verification failed but proceeding due to Warn policy") {
			t.Fatalf("event %q missing expected message", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected an event but none was emitted")
	}
}
