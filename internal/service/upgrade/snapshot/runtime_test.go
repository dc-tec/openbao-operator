package snapshot

import (
	"context"
	"errors"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	portbackup "github.com/dc-tec/openbao-operator/internal/port/backup"
)

type runtimeStub struct {
	ensureServiceAccountErr error
	ensureRBACErr           error
}

func (r runtimeStub) EnsureServiceAccount(context.Context, *openbaov1alpha1.OpenBaoCluster) error {
	return r.ensureServiceAccountErr
}

func (r runtimeStub) EnsureRBAC(context.Context, *openbaov1alpha1.OpenBaoCluster) error {
	return r.ensureRBACErr
}

func (r runtimeStub) BuildPreUpgradeJob(*openbaov1alpha1.OpenBaoCluster, portbackup.JobBuildOptions) (*batchv1.Job, error) {
	return nil, nil
}

func TestEnsureRuntime(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{}

	if err := EnsureRuntime(context.Background(), nil, cluster); err == nil || !strings.Contains(err.Error(), "backup runtime is not configured") {
		t.Fatalf("EnsureRuntime(nil) error = %v, want missing runtime", err)
	}

	if err := EnsureRuntime(context.Background(), runtimeStub{ensureServiceAccountErr: errors.New("sa failed")}, cluster); err == nil || !strings.Contains(err.Error(), "failed to ensure backup ServiceAccount") {
		t.Fatalf("EnsureRuntime(serviceaccount) error = %v, want SA wrapper", err)
	}

	if err := EnsureRuntime(context.Background(), runtimeStub{ensureRBACErr: errors.New("rbac failed")}, cluster); err == nil || !strings.Contains(err.Error(), "failed to ensure backup RBAC") {
		t.Fatalf("EnsureRuntime(rbac) error = %v, want RBAC wrapper", err)
	}

	if err := EnsureRuntime(context.Background(), runtimeStub{}, cluster); err != nil {
		t.Fatalf("EnsureRuntime() unexpected error: %v", err)
	}
}
