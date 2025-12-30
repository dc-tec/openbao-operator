package admission

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var admissionDependenciesReadyGauge = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Namespace: "openbao",
		Name:      "admission_dependencies_ready",
		Help:      "Whether required admission policy dependencies are installed and correctly bound (1 = ready, 0 = not ready)",
	},
)

func init() {
	metrics.Registry.MustRegister(admissionDependenciesReadyGauge)
}

// SetAdmissionDependenciesReady sets the readiness gauge for admission dependencies.
func SetAdmissionDependenciesReady(ready bool) {
	if ready {
		admissionDependenciesReadyGauge.Set(1)
		return
	}
	admissionDependenciesReadyGauge.Set(0)
}
