package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	certservice "github.com/dc-tec/openbao-operator/internal/service/certs"
)

// TLSReloadSignaler triggers an in-cluster reload when operator-managed TLS
// assets change. The app layer keeps this contract local so controllers do not
// depend on the concrete certs service package directly.
type TLSReloadSignaler interface {
	SignalReload(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, certHash string) error
}

// CertificatesDependencies groups the collaborators required for the
// operator-managed TLS reconciler.
type CertificatesDependencies struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Reloader TLSReloadSignaler
}

// NewCertificatesReconciler constructs the operator-managed TLS reconciler
// behind the app facade so controller wiring does not import the certs service
// package directly.
func NewCertificatesReconciler(deps CertificatesDependencies) SubReconciler {
	return certservice.NewManagerWithReloader(deps.Client, deps.Scheme, deps.Reloader)
}
