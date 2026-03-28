package provisioner

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// Manager handles the provisioning of RBAC resources for tenant namespaces.
type Manager struct {
	client     client.Client
	operatorSA OperatorServiceAccount
	logger     logr.Logger
}

// NewManager creates a new provisioner Manager.
func NewManager(c client.Client, logger logr.Logger) (*Manager, error) {
	// Get operator namespace from environment or use default
	saNamespace := os.Getenv("POD_NAMESPACE")
	if saNamespace == "" {
		saNamespace = os.Getenv("OPERATOR_NAMESPACE")
	}
	if saNamespace == "" {
		saNamespace = "openbao-operator-system"
	}

	// Discover the controller ServiceAccount name dynamically
	// The base name is "controller", which becomes "openbao-operator-controller" after kustomize prefix
	controllerSAName := os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME")
	if controllerSAName == "" {
		controllerSAName = "openbao-operator-controller"
	}
	controllerSANamespace := saNamespace

	return &Manager{
		client: c,
		operatorSA: OperatorServiceAccount{
			Name:      controllerSAName,
			Namespace: controllerSANamespace,
		},
		logger: logger,
	}, nil
}

// applyResource uses Server-Side Apply.
// Unlike infra.applyResource, this does NOT set owner references since
// tenant RBAC resources should not be garbage-collected with any single cluster.
func (m *Manager) applyResource(ctx context.Context, obj client.Object) error {
	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-provisioner"),
	}

	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(
				fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}
