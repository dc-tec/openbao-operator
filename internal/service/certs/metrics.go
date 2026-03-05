package certs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	tlsCertExpiryTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openbao",
			Name:      "tls_cert_expiry_timestamp",
			Help:      "Unix timestamp when the current server certificate expires",
		},
		[]string{"namespace", "name", "type"},
	)

	tlsRotationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Name:      "tls_rotation_total",
			Help:      "Total number of server certificate rotations",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		tlsCertExpiryTimestamp,
		tlsRotationTotal,
	)
}

// tlsMetrics provides helpers to record TLS-related metrics for a cluster.
type tlsMetrics struct {
	namespace string
	name      string
}

func newTLSMetrics(namespace, name string) *tlsMetrics {
	return &tlsMetrics{
		namespace: namespace,
		name:      name,
	}
}

// setServerCertExpiry records the expiry time for the current server certificate.
// The certType label is used to distinguish between OperatorManaged and External
// certificate sources.
func (m *tlsMetrics) setServerCertExpiry(expiry time.Time, certType string) {
	tlsCertExpiryTimestamp.
		WithLabelValues(m.namespace, m.name, certType).
		Set(float64(expiry.Unix()))
}

// incrementRotation increments the rotation counter for server certificates.
func (m *tlsMetrics) incrementRotation() {
	tlsRotationTotal.
		WithLabelValues(m.namespace, m.name).
		Inc()
}
