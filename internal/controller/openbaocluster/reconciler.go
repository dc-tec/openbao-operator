package openbaocluster

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	certmanager "github.com/dc-tec/openbao-operator/internal/service/certs"
)

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	RestConfig        *rest.Config
	TLSReload         certmanager.ReloadSignaler
	InitManager       initmanagerport.Manager
	OperatorNamespace string
	OIDCIssuer        string // OIDC issuer URL discovered at startup (best-effort warmup)
	OIDCJWTKeys       []string
	AdmissionStatus   *admission.Status
	Recorder          events.EventRecorder
	// SingleTenantMode indicates the controller is running in single-tenant mode.
	// When true, the controller uses Owns() watches for event-driven reconciliation
	// and caching is enabled for the watched namespace.
	SingleTenantMode      bool
	SmartClientConfig     portopenbao.ClientConfig
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
	Platform              string
}
