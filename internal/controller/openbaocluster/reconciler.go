package openbaocluster

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/admission"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	"github.com/dc-tec/openbao-operator/internal/interfaces"
	"github.com/dc-tec/openbao-operator/internal/openbao"
)

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	TLSReload         certmanager.ReloadSignaler
	InitManager       interfaces.InitManager
	OperatorNamespace string
	OIDCIssuer        string // OIDC issuer URL discovered at startup
	OIDCJWTKeys       []string
	AdmissionStatus   *admission.Status
	Recorder          events.EventRecorder
	// SingleTenantMode indicates the controller is running in single-tenant mode.
	// When true, the controller uses Owns() watches for event-driven reconciliation
	// and caching is enabled for the watched namespace.
	SingleTenantMode      bool
	SmartClientConfig     openbao.ClientConfig
	ImageVerifier         interfaces.ImageVerifier
	OperatorImageVerifier interfaces.ImageVerifier
	Platform              string
}
