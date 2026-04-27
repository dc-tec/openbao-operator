package controller

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"slices"
	"strconv"
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
	metricsAddr                                     string
	metricsCertPath                                 string
	metricsCertName                                 string
	metricsCertKey                                  string
	enableLeaderElection                            bool
	probeAddr                                       string
	secureMetrics                                   bool
	enableHTTP2                                     bool
	platform                                        string
	clientQPS                                       float64
	clientBurst                                     int
	clientCBFailureThreshold                        int
	clientCBOpenDuration                            time.Duration
	jwtAuthStrategy                                 string
	admissionEnforcement                            string
	admissionStartupTimeout                         time.Duration
	enableServiceClaims                             bool
	serviceClaimsAPIServerCIDR                      string
	serviceClaimsAPIServerEndpointIPs               []string
	serviceClaimsDNSEndpointIPs                     []string
	serviceClaimsTransitUnsealAddress               string
	serviceClaimsTransitUnsealKeyName               string
	serviceClaimsTransitUnsealMountPath             string
	serviceClaimsTransitUnsealNamespace             string
	serviceClaimsTransitUnsealTLSCACert             string
	serviceClaimsTransitUnsealTLSServerName         string
	serviceClaimsTransitUnsealCredentialsSecretName string
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

	enableServiceClaims, err := serviceClaimsEnabledFromEnv()
	if err != nil {
		return runConfig{}, err
	}
	cfg.enableServiceClaims = enableServiceClaims
	networkConfig, err := serviceClaimsNetworkConfigFromEnv()
	if err != nil {
		return runConfig{}, err
	}
	cfg.serviceClaimsAPIServerCIDR = networkConfig.apiServerCIDR
	cfg.serviceClaimsAPIServerEndpointIPs = networkConfig.apiServerEndpointIPs
	cfg.serviceClaimsDNSEndpointIPs = networkConfig.dnsEndpointIPs
	transitUnsealConfig, err := serviceClaimsTransitUnsealConfigFromEnv()
	if err != nil {
		return runConfig{}, err
	}
	cfg.serviceClaimsTransitUnsealAddress = transitUnsealConfig.address
	cfg.serviceClaimsTransitUnsealKeyName = transitUnsealConfig.keyName
	cfg.serviceClaimsTransitUnsealMountPath = transitUnsealConfig.mountPath
	cfg.serviceClaimsTransitUnsealNamespace = transitUnsealConfig.namespace
	cfg.serviceClaimsTransitUnsealTLSCACert = transitUnsealConfig.tlsCACert
	cfg.serviceClaimsTransitUnsealTLSServerName = transitUnsealConfig.tlsServerName
	cfg.serviceClaimsTransitUnsealCredentialsSecretName = transitUnsealConfig.credentialsSecretName

	return cfg, nil
}

func serviceClaimsEnabledFromEnv() (bool, error) {
	return boolEnv(constants.EnvOperatorEnableServiceClaims)
}

func boolEnv(key string) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return false, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}

	return value, nil
}

type serviceClaimsTransitUnsealEnvConfig struct {
	address               string
	keyName               string
	mountPath             string
	namespace             string
	tlsCACert             string
	tlsServerName         string
	credentialsSecretName string
}

func serviceClaimsTransitUnsealConfigFromEnv() (serviceClaimsTransitUnsealEnvConfig, error) {
	cfg := serviceClaimsTransitUnsealEnvConfig{
		address:       strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealAddress)),
		keyName:       strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealKeyName)),
		mountPath:     strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealMountPath)),
		namespace:     strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealNamespace)),
		tlsCACert:     strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealTLSCACert)),
		tlsServerName: strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealTLSServerName)),
		credentialsSecretName: strings.TrimSpace(
			os.Getenv(constants.EnvOperatorServiceClaimsTransitUnsealCredentialsSecretName),
		),
	}

	if cfg == (serviceClaimsTransitUnsealEnvConfig{}) {
		return cfg, nil
	}

	missing := make([]string, 0, 4)
	if cfg.address == "" {
		missing = append(missing, constants.EnvOperatorServiceClaimsTransitUnsealAddress)
	}
	if cfg.keyName == "" {
		missing = append(missing, constants.EnvOperatorServiceClaimsTransitUnsealKeyName)
	}
	if cfg.mountPath == "" {
		missing = append(missing, constants.EnvOperatorServiceClaimsTransitUnsealMountPath)
	}
	if cfg.credentialsSecretName == "" {
		missing = append(missing, constants.EnvOperatorServiceClaimsTransitUnsealCredentialsSecretName)
	}
	if len(missing) > 0 {
		return serviceClaimsTransitUnsealEnvConfig{}, fmt.Errorf(
			"service-claims transit unseal config is incomplete: missing %s",
			strings.Join(missing, ", "),
		)
	}

	return cfg, nil
}

type serviceClaimsNetworkEnvConfig struct {
	apiServerCIDR        string
	apiServerEndpointIPs []string
	dnsEndpointIPs       []string
}

func serviceClaimsNetworkConfigFromEnv() (serviceClaimsNetworkEnvConfig, error) {
	apiServerCIDR := strings.TrimSpace(os.Getenv(constants.EnvOperatorServiceClaimsAPIServerCIDR))
	if apiServerCIDR != "" {
		_, _, err := net.ParseCIDR(apiServerCIDR)
		if err != nil {
			return serviceClaimsNetworkEnvConfig{}, fmt.Errorf(
				"%s: invalid CIDR %q: %w",
				constants.EnvOperatorServiceClaimsAPIServerCIDR,
				apiServerCIDR,
				err,
			)
		}
	}

	apiServerEndpointIPs, err := parseIPListEnv(constants.EnvOperatorServiceClaimsAPIServerEndpointIPs)
	if err != nil {
		return serviceClaimsNetworkEnvConfig{}, err
	}
	dnsEndpointIPs, err := parseIPListEnv(constants.EnvOperatorServiceClaimsDNSEndpointIPs)
	if err != nil {
		return serviceClaimsNetworkEnvConfig{}, err
	}

	return serviceClaimsNetworkEnvConfig{
		apiServerCIDR:        apiServerCIDR,
		apiServerEndpointIPs: apiServerEndpointIPs,
		dnsEndpointIPs:       dnsEndpointIPs,
	}, nil
}

func parseIPListEnv(key string) ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	seen := map[string]struct{}{}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		ip := strings.TrimSpace(part)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed == nil {
			return nil, fmt.Errorf("%s: invalid IP address %q", key, ip)
		}
		canonical := parsed.String()
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		values = append(values, canonical)
	}
	slices.Sort(values)
	return values, nil
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
