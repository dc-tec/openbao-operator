package observability

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const kubeLabelOther = "other"

var kubeClientRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "openbao_kube_client_requests_total",
	Help: "Kubernetes HTTP requests by operation, resource, subresource, and result; excludes cache hits.",
}, []string{"verb", "resource", "subresource", "result"})

func init() {
	metrics.Registry.MustRegister(kubeClientRequestsTotal)
}

// WrapKubernetesTransport attributes actual HTTP attempts, including retries.
// Labels exclude object names, namespaces, URLs, and error text.
func WrapKubernetesTransport(next http.RoundTripper) http.RoundTripper {
	return &kubernetesTransport{next: next}
}

type kubernetesTransport struct{ next http.RoundTripper }

func (t *kubernetesTransport) WrappedRoundTripper() http.RoundTripper { return t.next }

func (t *kubernetesTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	verb, resource, subresource := kubernetesRequestLabels(req)
	response, err := t.next.RoundTrip(req)
	result := "error"
	if err == nil && response != nil {
		switch {
		case response.StatusCode >= 200 && response.StatusCode < 300:
			result = "success"
		case response.StatusCode == http.StatusConflict:
			result = "conflict"
		case response.StatusCode == http.StatusNotFound:
			result = "not_found"
		case response.StatusCode == http.StatusForbidden:
			result = "forbidden"
		}
	}
	kubeClientRequestsTotal.WithLabelValues(verb, resource, subresource, result).Inc()
	return response, err
}

func kubernetesRequestLabels(req *http.Request) (verb, resource, subresource string) {
	verb, resource, subresource = kubeLabelOther, kubeLabelOther, "none"
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	switch {
	case len(parts) >= 3 && parts[0] == "api":
		parts = parts[2:]
	case len(parts) >= 4 && parts[0] == "apis":
		parts = parts[3:]
	default:
		return verb, resource, subresource
	}
	if len(parts) >= 3 && parts[0] == "namespaces" && parts[2] != "status" && parts[2] != "finalize" {
		parts = parts[2:]
	}
	switch parts[0] {
	case "configmaps", "secrets", "services", "serviceaccounts", "pods", "persistentvolumeclaims",
		"namespaces", "nodes", "events", "endpointslices", "statefulsets", "deployments", "replicasets", "jobs", "cronjobs",
		"leases", "roles", "rolebindings", "clusterroles", "clusterrolebindings", "networkpolicies",
		"poddisruptionbudgets", "ingresses", "gateways", "httproutes", "tlsroutes", "backendtlspolicies", "referencegrants",
		"servicemonitors", "podmonitors", "openbaoclusters", "openbaotenants", "openbaorestores",
		"validatingadmissionpolicies", "validatingadmissionpolicybindings", "customresourcedefinitions":
		resource = parts[0]
	}
	if len(parts) > 2 {
		switch parts[2] {
		case "status", "scale", "token", "eviction", "finalize":
			subresource = parts[2]
		default:
			subresource = kubeLabelOther
		}
	}
	switch req.Method {
	case http.MethodGet:
		verb = "get"
		if len(parts) == 1 {
			verb = "list"
		}
		if req.URL.Query().Get("watch") == "true" {
			verb = "watch"
		}
	case http.MethodPost:
		verb = "create"
	case http.MethodPut:
		verb = "update"
	case http.MethodDelete:
		verb = "delete"
	case http.MethodPatch:
		verb = "patch"
		contentType := req.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "application/apply-patch+") {
			verb = "apply"
		}
	}
	return verb, resource, subresource
}
