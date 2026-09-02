package provisioner

import (
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestSetupWithManagerRequiresProvisionerApplication(t *testing.T) {
	tests := []struct {
		name  string
		setup func(ctrl.Manager) error
	}{
		{
			name:  "namespace provisioner",
			setup: (&NamespaceProvisionerReconciler{}).SetupWithManager,
		},
		{
			name:  "tenant Secrets RBAC",
			setup: (&TenantSecretsRBACReconciler{}).SetupWithManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.setup(nil)
			if err == nil {
				t.Fatal("SetupWithManager() error = nil, want missing provisioner error")
			}
			if got, want := err.Error(), "provisioner application is not configured"; got != want {
				t.Fatalf("SetupWithManager() error = %q, want %q", got, want)
			}
		})
	}
}
