package controller

import (
	"crypto/tls"
	"flag"
	"os"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

type runConfig struct {
	metricsAddr              string
	metricsCertPath          string
	metricsCertName          string
	metricsCertKey           string
	enableLeaderElection     bool
	probeAddr                string
	secureMetrics            bool
	enableHTTP2              bool
	platform                 string
	clientQPS                float64
	clientBurst              int
	clientCBFailureThreshold int
	clientCBOpenDuration     time.Duration
	jwtAuthStrategy          string
	admissionEnforcement     string
	admissionStartupTimeout  time.Duration
}

func parseRunConfig() (runConfig, error) {
	cfg := runConfig{}

	entrypoint.BindManagerFlags(
		flag.CommandLine,
		&cfg.metricsAddr,
		&cfg.probeAddr,
		&cfg.enableLeaderElection,
		&cfg.secureMetrics,
	)
	flag.StringVar(&cfg.metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(
		&cfg.metricsCertName,
		"metrics-cert-name",
		"tls.crt",
		"The name of the metrics server certificate file.",
	)
	flag.StringVar(&cfg.metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&cfg.enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics server")
	flag.StringVar(&cfg.platform, "platform", constants.PlatformAuto,
		"The target platform (auto, kubernetes, openshift). Defaults to auto. "+
			"This flag is deprecated and will be removed in a future release. "+
			"Use the OPERATOR_PLATFORM environment variable instead.")

	flag.Float64Var(&cfg.clientQPS, "openbao-client-qps", 50.0,
		"The queries per second (QPS) limit for OpenBao API clients.")
	flag.IntVar(&cfg.clientBurst, "openbao-client-burst", 100,
		"The burst limit for OpenBao API clients.")
	flag.IntVar(&cfg.clientCBFailureThreshold, "openbao-client-cb-failure-threshold", 50,
		"The number of consecutive failures before opening the circuit breaker.")
	flag.DurationVar(&cfg.clientCBOpenDuration, "openbao-client-cb-open-duration", 30*time.Second,
		"The duration the circuit breaker remains open before testing the connection.")

	entrypoint.BindAdmissionFlags(flag.CommandLine, &cfg.admissionEnforcement, &cfg.admissionStartupTimeout)

	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	cfg.platform = strings.ToLower(strings.TrimSpace(cfg.platform))

	jwtAuthStrategy, err := portopenbao.NormalizeJWTAuthStrategy(os.Getenv(constants.EnvOpenBaoJWTAuthStrategy))
	if err != nil {
		return runConfig{}, err
	}
	cfg.jwtAuthStrategy = jwtAuthStrategy

	admissionEnforcement, err := entrypoint.NormalizeAdmissionEnforcement(cfg.admissionEnforcement)
	if err != nil {
		return runConfig{}, err
	}
	cfg.admissionEnforcement = admissionEnforcement

	return cfg, nil
}

func buildMetricsServerOptions(cfg runConfig) metricsserver.Options {
	metricsServerOptions := metricsserver.Options{
		BindAddress:   cfg.metricsAddr,
		SecureServing: cfg.secureMetrics,
		TLSOpts:       buildTLSOptions(cfg.enableHTTP2),
	}

	if cfg.secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if cfg.metricsCertPath != "" {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", cfg.metricsCertPath,
			"metrics-cert-name", cfg.metricsCertName,
			"metrics-cert-key", cfg.metricsCertKey,
		)
		metricsServerOptions.CertDir = cfg.metricsCertPath
		metricsServerOptions.CertName = cfg.metricsCertName
		metricsServerOptions.KeyName = cfg.metricsCertKey
	}

	return metricsServerOptions
}

func buildTLSOptions(enableHTTP2 bool) []func(*tls.Config) {
	if enableHTTP2 {
		return nil
	}

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	return []func(*tls.Config){disableHTTP2}
}
