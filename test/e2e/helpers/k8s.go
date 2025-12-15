package helpers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RunWithImpersonation runs the provided action as the specified user/group(s)
// using Kubernetes impersonation. This is used in E2E tests to validate RBAC and
// ValidatingAdmissionPolicy enforcement without relying on system:masters.
func RunWithImpersonation(ctx context.Context, baseConfig *rest.Config, scheme *runtime.Scheme, username string, groups []string, action func(c client.Client) error) error {
	if baseConfig == nil {
		return fmt.Errorf("base Kubernetes REST config is required")
	}
	if scheme == nil {
		return fmt.Errorf("runtime Scheme is required")
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}
	if action == nil {
		return fmt.Errorf("action is required")
	}

	impersonatedConfig := rest.CopyConfig(baseConfig)
	impersonatedConfig.Impersonate = rest.ImpersonationConfig{
		UserName: username,
		Groups:   groups,
	}

	impersonatedClient, err := client.New(impersonatedConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create impersonated client for user %q: %w", username, err)
	}

	if err := action(impersonatedClient); err != nil {
		return err
	}

	return nil
}
