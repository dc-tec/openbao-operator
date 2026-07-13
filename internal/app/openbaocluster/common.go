package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	recon "github.com/dc-tec/openbao-operator/internal/platform/reconcile"
)

// SubReconciler is the shared contract for OpenBaoCluster app orchestration steps.
type SubReconciler interface {
	Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) (recon.Result, error)
}

// ErrorRecorder captures errors for metric bookkeeping in controller wrappers.
type ErrorRecorder func(error)

// StatusIntegrationDependencies groups the shared Kubernetes collaborators
// used to evaluate operator-managed network integrations.
type StatusIntegrationDependencies struct {
	Client            client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	OperatorNamespace string
	Platform          string
}

// ReconcileErrorReason extracts a stable status reason from typed errors.
func ReconcileErrorReason(err error) string {
	if err == nil {
		return ""
	}
	if reason, ok := operatorerrors.Reason(err); ok {
		return reason
	}
	return "Error"
}

func controllerErrorStatus(err error) *openbaov1alpha1.ControllerErrorStatus {
	if err == nil {
		return nil
	}

	reason := ReconcileErrorReason(err)
	now := metav1.Now()
	return &openbaov1alpha1.ControllerErrorStatus{
		Reason:  reason,
		Message: err.Error(),
		At:      &now,
	}
}
