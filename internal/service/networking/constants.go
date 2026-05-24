package networking

const (
	publicServiceSuffix    = "-public"
	acmeServiceSuffix      = "-acme"
	metricsServiceSuffix   = "-metrics"
	httpRouteSuffix        = "-httproute"
	tlsRouteSuffix         = "-tlsroute"
	backendTLSPolicySuffix = "-backend-tls-policy"
)

const (
	metricsServicePortName      = "https-metrics"
	metricsPath                 = "/v1/sys/metrics"
	metricsFormatParam          = "prometheus"
	metricsComponentLabelValue  = "metrics"
	metricsScrapeProfileLabel   = "openbao.org/scrape-profile"
	metricsScrapeProfileActive  = "Active"
	metricsScrapeProfileAllNode = "AllNodes"
	labelValueTrue              = "true"
	defaultServiceMonitorJobKey = "app.kubernetes.io/name"
)
