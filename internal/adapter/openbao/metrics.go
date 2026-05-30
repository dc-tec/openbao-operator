package openbao

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const metricLabelUnknown = "unknown"

var (
	clientRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "requests_total",
			Help:      "Total number of OpenBao API requests made by the operator.",
		},
		[]string{"method", "path", "status", "result"},
	)
	clientRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "request_duration_seconds",
			Help:      "Duration of OpenBao API requests made by the operator.",
			Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"method", "path", "status", "result"},
	)
	clientAuthLoginsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "auth_logins_total",
			Help:      "Total number of JWT auth login attempts made by the operator.",
		},
		[]string{"result"},
	)
	clientAuthLoginErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "auth_login_errors_total",
			Help:      "Total number of failed JWT auth login attempts made by the operator.",
		},
		[]string{"reason"},
	)
	clientAuthCacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "auth_cache_hits_total",
			Help:      "Total number of OpenBao JWT auth token cache hits.",
		},
		[]string{"role"},
	)
	clientAuthCacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "openbao",
			Subsystem: "client",
			Name:      "auth_cache_misses_total",
			Help:      "Total number of OpenBao JWT auth token cache misses.",
		},
		[]string{"role"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		clientRequestsTotal,
		clientRequestDurationSeconds,
		clientAuthLoginsTotal,
		clientAuthLoginErrorsTotal,
		clientAuthCacheHitsTotal,
		clientAuthCacheMissesTotal,
	)
}

func recordClientRequest(req *http.Request, statusCode int, result string, duration time.Duration) {
	method := "UNKNOWN"
	path := metricLabelUnknown
	if req != nil {
		if req.Method != "" {
			method = req.Method
		}
		if req.URL != nil && req.URL.Path != "" {
			path = req.URL.Path
		}
	}
	status := "transport_error"
	if statusCode > 0 {
		status = strconv.Itoa(statusCode)
	}
	clientRequestsTotal.WithLabelValues(method, path, status, result).Inc()
	clientRequestDurationSeconds.WithLabelValues(method, path, status, result).Observe(duration.Seconds())
}

func clientRequestResult(statusCode int) string {
	if statusCode >= 200 && statusCode < 400 {
		return "success"
	}
	return "error"
}

func recordAuthLoginSuccess() {
	clientAuthLoginsTotal.WithLabelValues("success").Inc()
}

func recordAuthLoginError(reason string) {
	if reason == "" {
		reason = metricLabelUnknown
	}
	clientAuthLoginsTotal.WithLabelValues("error").Inc()
	clientAuthLoginErrorsTotal.WithLabelValues(reason).Inc()
}

func recordAuthCacheHit(role string) {
	clientAuthCacheHitsTotal.WithLabelValues(metricRole(role)).Inc()
}

func recordAuthCacheMiss(role string) {
	clientAuthCacheMissesTotal.WithLabelValues(metricRole(role)).Inc()
}

func metricRole(role string) string {
	if role == "" {
		return metricLabelUnknown
	}
	return role
}
