package provisioner

import (
	"flag"
	"fmt"
	"io"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
)

type runConfig struct {
	kubeconfig              string
	logOptions              zap.Options
	metricsAddr             string
	enableLeaderElection    bool
	probeAddr               string
	secureMetrics           bool
	admissionEnforcement    string
	admissionStartupTimeout time.Duration
	admissionCanary         bool
}

func parseRunConfig(args []string, output io.Writer) (runConfig, error) {
	cfg := runConfig{}
	fs := flag.NewFlagSet("provisioner", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&cfg.kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")

	entrypoint.BindManagerFlags(
		fs,
		&cfg.metricsAddr,
		&cfg.probeAddr,
		&cfg.enableLeaderElection,
		&cfg.secureMetrics,
	)
	entrypoint.BindAdmissionFlags(fs, &cfg.admissionEnforcement, &cfg.admissionStartupTimeout)
	fs.BoolVar(&cfg.admissionCanary, "admission-canary", false,
		"If set, perform an admission canary (dry-run) that must be denied "+
			"by the Provisioner RBAC ValidatingAdmissionPolicy. "+
			"This provides stronger assurance that enforcement is active.")

	cfg.logOptions = zap.Options{Development: false}
	cfg.logOptions.BindFlags(fs)
	if err := fs.Parse(args); err != nil {
		return runConfig{}, err
	}
	if fs.NArg() != 0 {
		return runConfig{}, fmt.Errorf("unexpected positional argument %q", fs.Arg(0))
	}

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
	}

	if cfg.secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	return metricsServerOptions
}
