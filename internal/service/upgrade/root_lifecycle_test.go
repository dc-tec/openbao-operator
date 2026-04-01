package upgrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestStartRootUpgradeLifecycle(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			CurrentVersion: "2.4.4",
		},
	}

	var persisted bool
	var emittedFrom string
	var emittedTo string
	err := StartRootUpgradeLifecycle(
		context.Background(),
		logr.Discard(),
		cluster,
		nil,
		"rolling",
		RootUpgradeStartOptions{
			Persist: func(ctx context.Context, got *openbaov1alpha1.OpenBaoCluster, start RootUpgradeSessionStart) error {
				persisted = true
				if got.Status.Upgrade == nil {
					t.Fatal("expected upgrade status to be initialized before persistence")
				}
				if start.Replicas != 3 {
					t.Fatalf("start.Replicas=%d, want 3", start.Replicas)
				}
				return nil
			},
			EmitEvent: func(fromVersion, toVersion string) {
				emittedFrom = fromVersion
				emittedTo = toVersion
			},
		},
	)
	if err != nil {
		t.Fatalf("StartRootUpgradeLifecycle() error = %v", err)
	}
	if !persisted {
		t.Fatal("expected persist callback to run")
	}
	if emittedFrom != "2.4.4" || emittedTo != "2.5.0" {
		t.Fatalf("emit event args = (%q, %q), want (2.4.4, 2.5.0)", emittedFrom, emittedTo)
	}
}

func TestStartRootUpgradeLifecycle_PropagatesPersistError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.5.0",
			Replicas: 3,
		},
	}

	wantErr := errors.New("persist failed")
	err := StartRootUpgradeLifecycle(
		context.Background(),
		logr.Discard(),
		cluster,
		nil,
		"rolling",
		RootUpgradeStartOptions{
			Persist: func(ctx context.Context, got *openbaov1alpha1.OpenBaoCluster, start RootUpgradeSessionStart) error {
				return wantErr
			},
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("StartRootUpgradeLifecycle() error = %v, want %v", err, wantErr)
	}
}

func TestCompleteRootUpgradeLifecycle(t *testing.T) {
	t.Parallel()

	startedAt := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion: "2.4.4",
				StartedAt:   &startedAt,
			},
		},
	}

	var persisted bool
	var emittedFrom string
	var emittedTo string
	err := CompleteRootUpgradeLifecycle(
		context.Background(),
		logr.Discard(),
		cluster,
		nil,
		"rolling",
		RootUpgradeCompletionOptions{
			Persist: func(ctx context.Context, got *openbaov1alpha1.OpenBaoCluster, completion RootUpgradeSessionCompletion) error {
				persisted = true
				if completion.FromVersion != "2.4.4" {
					t.Fatalf("completion.FromVersion=%q, want 2.4.4", completion.FromVersion)
				}
				if completion.ToVersion != "2.5.0" {
					t.Fatalf("completion.ToVersion=%q, want 2.5.0", completion.ToVersion)
				}
				return nil
			},
			EmitEvent: func(fromVersion, toVersion string) {
				emittedFrom = fromVersion
				emittedTo = toVersion
			},
		},
	)
	if err != nil {
		t.Fatalf("CompleteRootUpgradeLifecycle() error = %v", err)
	}
	if !persisted {
		t.Fatal("expected persist callback to run")
	}
	if emittedFrom != "2.4.4" || emittedTo != "2.5.0" {
		t.Fatalf("emit event args = (%q, %q), want (2.4.4, 2.5.0)", emittedFrom, emittedTo)
	}
}

func TestCompleteRootUpgradeLifecycle_PropagatesPersistError(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "upgrade-cluster",
			Namespace: "default",
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version: "2.5.0",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				FromVersion: "2.4.4",
			},
		},
	}

	wantErr := errors.New("persist failed")
	err := CompleteRootUpgradeLifecycle(
		context.Background(),
		logr.Discard(),
		cluster,
		nil,
		"rolling",
		RootUpgradeCompletionOptions{
			Persist: func(ctx context.Context, got *openbaov1alpha1.OpenBaoCluster, completion RootUpgradeSessionCompletion) error {
				return wantErr
			},
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("CompleteRootUpgradeLifecycle() error = %v, want %v", err, wantErr)
	}
}
