package openbaocluster

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
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
	OpenBaoClientFactory  portopenbao.ClientFactory
	DiscoverOIDCConfig    portauth.DiscoverConfigFunc
	OIDCStatusCode        portauth.DiscoveryStatusCodeFunc
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
	Platform              string
}

func (r *OpenBaoClusterReconciler) clientForPod(cluster *openbaov1alpha1.OpenBaoCluster, podName string) (portopenbao.ClusterActions, error) {
	if r.OpenBaoClientFactory == nil {
		return nil, fmt.Errorf("OpenBao client factory is not configured")
	}

	headlessServiceName := cluster.Name
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	cfg := r.SmartClientConfig
	cfg.BaseURL = "https://" + podDNS

	return r.OpenBaoClientFactory(cfg)
}
