package openbaocluster

import (
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

// ControllerRuntime groups the controller-runtime and operator process
// dependencies shared across workload, adminops, and status reconciliation.
type ControllerRuntime struct {
	APIReader        client.Reader
	AdmissionTracker *admission.Tracker
	Recorder         events.EventRecorder
	// SingleTenantMode indicates the controller is running in single-tenant mode.
	// When true, the controller uses Owns() watches for event-driven reconciliation
	// and caching is enabled for the watched namespace.
	SingleTenantMode bool
}

// OpenBaoClusterReconciler reconciles a OpenBaoCluster object.
type OpenBaoClusterReconciler struct {
	client.Client
	ControllerRuntime
	Applications *appopenbaocluster.Applications
}
