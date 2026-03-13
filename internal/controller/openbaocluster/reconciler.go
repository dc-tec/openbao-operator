package openbaocluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
	"github.com/dc-tec/openbao-operator/internal/platform/openbaotls"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	"github.com/dc-tec/openbao-operator/internal/port/imageverify"
	initmanagerport "github.com/dc-tec/openbao-operator/internal/port/initmanager"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	certmanager "github.com/dc-tec/openbao-operator/internal/service/certs"
)

// ControllerRuntime groups the controller-runtime and operator process
// dependencies shared across workload, adminops, and status reconciliation.
type ControllerRuntime struct {
	APIReader         client.Reader
	Scheme            *runtime.Scheme
	RestConfig        *rest.Config
	OperatorNamespace string
	AdmissionTracker  *admission.Tracker
	Recorder          events.EventRecorder
	Platform          string
	// SingleTenantMode indicates the controller is running in single-tenant mode.
	// When true, the controller uses Owns() watches for event-driven reconciliation
	// and caching is enabled for the watched namespace.
	SingleTenantMode bool
}

// OIDCRuntime groups OIDC discovery configuration and warmup state.
type OIDCRuntime struct {
	OIDCIssuer         string // OIDC issuer URL discovered at startup (best-effort warmup)
	OIDCJWTKeys        []string
	DiscoverOIDCConfig portauth.DiscoverConfigFunc
	OIDCStatusCode     portauth.DiscoveryStatusCodeFunc
}

// OpenBaoRuntime groups OpenBao-specific collaborators used by the controller.
type OpenBaoRuntime struct {
	TLSReload            certmanager.ReloadSignaler
	InitManager          initmanagerport.Manager
	SmartClientConfig    portopenbao.ClientConfig
	OpenBaoClientFactory portopenbao.ClientFactory
}

// ImageVerificationRuntime groups the image verifiers used by cluster and
// operator-managed executor workflows.
type ImageVerificationRuntime struct {
	ImageVerifier         imageverify.Verifier
	OperatorImageVerifier imageverify.Verifier
}

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	ControllerRuntime
	OIDCRuntime
	OpenBaoRuntime
	ImageVerificationRuntime
}

func (r *OpenBaoClusterReconciler) clientForPod(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, podName string) (portopenbao.ClusterActions, error) {
	if r.OpenBaoClientFactory == nil {
		return nil, fmt.Errorf("OpenBao client factory is not configured")
	}

	headlessServiceName := cluster.Name
	podDNS := fmt.Sprintf("%s.%s.%s.svc:8200", podName, headlessServiceName, cluster.Namespace)
	cfg := r.SmartClientConfig
	cfg.BaseURL = "https://" + podDNS
	cfg.TLSServerName = portopenbao.ComputeTLSServerName(cluster)

	caCert, err := openbaotls.LoadClusterTrustBundle(ctx, r.Client, cluster)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster trust bundle for %s/%s: %w", cluster.Namespace, cluster.Name, err)
	}
	cfg.CACert = caCert

	return r.OpenBaoClientFactory(cfg)
}
