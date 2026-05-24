package networking

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestBuildWorkloadServiceMonitor_IncludesAuthTLSAndSelector(t *testing.T) {
	cluster := newMinimalCluster("metrics-cluster", "security")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{
			Enabled: true,
			ServiceMonitor: &openbaov1alpha1.ServiceMonitorConfig{
				Enabled:       true,
				Interval:      "30s",
				ScrapeTimeout: "10s",
				Labels: map[string]string{
					"release": "kube-prometheus-stack",
				},
				Authorization: &openbaov1alpha1.ServiceMonitorAuthorizationConfig{
					CredentialsSecret: openbaov1alpha1.ServiceMonitorKeySelector{
						Name: "openbao-metrics-token",
					},
				},
				TLSConfig: &openbaov1alpha1.ServiceMonitorTLSConfig{
					ServerName:         "openbao-metrics.security.svc",
					InsecureSkipVerify: ptr.To(false),
					CAConfigMap: &openbaov1alpha1.ServiceMonitorKeySelector{
						Name: "openbao-metrics-ca",
					},
				},
			},
		},
	}

	monitor, err := buildWorkloadServiceMonitor(cluster)
	if err != nil {
		t.Fatalf("buildWorkloadServiceMonitor() error = %v", err)
	}
	if monitor == nil {
		t.Fatal("expected ServiceMonitor, got nil")
	}

	if got := monitor.GetLabels()["release"]; got != "kube-prometheus-stack" {
		t.Fatalf("release label = %q", got)
	}
	if got := monitor.GetLabels()[metricsScrapeProfileLabel]; got != metricsScrapeProfileActive {
		t.Fatalf("scrape profile label = %q", got)
	}

	endpoints, ok, err := unstructured.NestedSlice(monitor.Object, "spec", "endpoints")
	if err != nil || !ok || len(endpoints) != 1 {
		t.Fatalf("expected one endpoint, ok=%v err=%v endpoints=%#v", ok, err, endpoints)
	}
	endpoint, ok := endpoints[0].(map[string]interface{})
	if !ok {
		t.Fatalf("endpoint has unexpected type %T", endpoints[0])
	}

	if got := endpoint["port"]; got != metricsServicePortName {
		t.Fatalf("endpoint port = %v, want %q", got, metricsServicePortName)
	}
	if got := endpoint["path"]; got != metricsPath {
		t.Fatalf("endpoint path = %v, want %q", got, metricsPath)
	}
	if got := endpoint["scheme"]; got != "https" {
		t.Fatalf("endpoint scheme = %v, want https", got)
	}

	authorization := endpoint["authorization"].(map[string]interface{})
	credentials := authorization["credentials"].(map[string]interface{})
	if got := authorization["type"]; got != "Bearer" {
		t.Fatalf("authorization type = %v, want Bearer", got)
	}
	if got := credentials["name"]; got != "openbao-metrics-token" {
		t.Fatalf("credentials name = %v", got)
	}
	if got := credentials["key"]; got != "token" {
		t.Fatalf("credentials key = %v, want token", got)
	}

	tlsConfig := endpoint["tlsConfig"].(map[string]interface{})
	ca := tlsConfig["ca"].(map[string]interface{})
	caConfigMap := ca["configMap"].(map[string]interface{})
	if got := tlsConfig["serverName"]; got != "openbao-metrics.security.svc" {
		t.Fatalf("tls serverName = %v", got)
	}
	if got := caConfigMap["key"]; got != "ca.crt" {
		t.Fatalf("ca configmap key = %v, want ca.crt", got)
	}

	selector, ok, err := unstructured.NestedStringMap(monitor.Object, "spec", "selector", "matchLabels")
	if err != nil || !ok {
		t.Fatalf("expected selector matchLabels, ok=%v err=%v", ok, err)
	}
	if _, exists := selector["release"]; exists {
		t.Fatalf("metadata labels should not leak into Service selector: %#v", selector)
	}
	if got := selector[metricsScrapeProfileLabel]; got != metricsScrapeProfileActive {
		t.Fatalf("selector scrape profile = %q", got)
	}
}

func TestBuildWorkloadServiceMonitor_PreservesOperatorIdentityLabels(t *testing.T) {
	cluster := newMinimalCluster("metrics-cluster", "security")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{
			Enabled: true,
			ServiceMonitor: &openbaov1alpha1.ServiceMonitorConfig{
				Enabled: true,
				Labels: map[string]string{
					constants.LabelAppManagedBy:   "user",
					constants.LabelOpenBaoCluster: "other-cluster",
					constants.LabelAppComponent:   "custom",
					"release":                     "kube-prometheus-stack",
				},
			},
		},
	}

	monitor, err := buildWorkloadServiceMonitor(cluster)
	if err != nil {
		t.Fatalf("buildWorkloadServiceMonitor() error = %v", err)
	}
	labels := monitor.GetLabels()
	if got := labels[constants.LabelAppManagedBy]; got != constants.LabelValueAppManagedByOpenBaoOperator {
		t.Fatalf("managed-by label = %q, want %q", got, constants.LabelValueAppManagedByOpenBaoOperator)
	}
	if got := labels[constants.LabelOpenBaoCluster]; got != cluster.Name {
		t.Fatalf("cluster label = %q, want %q", got, cluster.Name)
	}
	if got := labels[constants.LabelAppComponent]; got != metricsComponentLabelValue {
		t.Fatalf("component label = %q, want %q", got, metricsComponentLabelValue)
	}
	if got := labels["release"]; got != "kube-prometheus-stack" {
		t.Fatalf("user label = %q", got)
	}
}

func TestBuildWorkloadServiceMonitor_RejectsInvalidTLSCAConfig(t *testing.T) {
	cluster := newMinimalCluster("metrics-invalid", "default")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{
			Enabled: true,
			ServiceMonitor: &openbaov1alpha1.ServiceMonitorConfig{
				Enabled: true,
				TLSConfig: &openbaov1alpha1.ServiceMonitorTLSConfig{
					CAConfigMap: &openbaov1alpha1.ServiceMonitorKeySelector{Name: "ca-config"},
					CASecret:    &openbaov1alpha1.ServiceMonitorKeySelector{Name: "ca-secret"},
				},
			},
		},
	}

	_, err := buildWorkloadServiceMonitor(cluster)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildWorkloadServiceMonitor_AllNodesAddsRelabeling(t *testing.T) {
	cluster := newMinimalCluster("metrics-all-nodes", "default")
	cluster.Spec.Observability = &openbaov1alpha1.ObservabilityConfig{
		Metrics: &openbaov1alpha1.MetricsConfig{
			Enabled:       true,
			ScrapeProfile: metricsScrapeProfileAllNode,
		},
	}

	monitor, err := buildWorkloadServiceMonitor(cluster)
	if err != nil {
		t.Fatalf("buildWorkloadServiceMonitor() error = %v", err)
	}
	endpoints, ok, err := unstructured.NestedSlice(monitor.Object, "spec", "endpoints")
	if err != nil || !ok || len(endpoints) != 1 {
		t.Fatalf("expected one endpoint, ok=%v err=%v endpoints=%#v", ok, err, endpoints)
	}
	endpoint := endpoints[0].(map[string]interface{})
	relabelings, ok := endpoint["relabelings"].([]interface{})
	if !ok || len(relabelings) != 2 {
		t.Fatalf("expected pod and node relabelings, got %#v", endpoint["relabelings"])
	}

	selector, ok, err := unstructured.NestedStringMap(monitor.Object, "spec", "selector", "matchLabels")
	if err != nil || !ok {
		t.Fatalf("expected selector matchLabels, ok=%v err=%v", ok, err)
	}
	if got := selector[metricsScrapeProfileLabel]; got != metricsScrapeProfileAllNode {
		t.Fatalf("selector scrape profile = %q, want %q", got, metricsScrapeProfileAllNode)
	}
}
