package openbaocluster

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/admission"
	certmanager "github.com/dc-tec/openbao-operator/internal/certs"
	initmanager "github.com/dc-tec/openbao-operator/internal/init"
	"github.com/dc-tec/openbao-operator/internal/openbao"
	security "github.com/dc-tec/openbao-operator/internal/security"
)

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	TLSReload         certmanager.ReloadSignaler
	InitManager       *initmanager.Manager
	OperatorNamespace string
	OIDCIssuer        string // OIDC issuer URL discovered at startup
	OIDCJWTKeys       []string
	AdmissionStatus   *admission.Status
	Recorder          record.EventRecorder
	// SingleTenantMode indicates the controller is running in single-tenant mode.
	// When true, the controller uses Owns() watches for event-driven reconciliation
	// and caching is enabled for the watched namespace.
	SingleTenantMode      bool
	SmartClientConfig     openbao.ClientConfig
	ImageVerifier         *security.ImageVerifier
	OperatorImageVerifier *security.ImageVerifier
	Platform              string
}
