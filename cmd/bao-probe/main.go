package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/probe"
)

const (
	modeStartup   = "startup"
	modeLiveness  = "liveness"
	modeReadiness = "readiness"

	livenessPath  = constants.APIPathSysHealth + "?standbyok=true&sealedcode=204&uninitcode=204"
	readinessPath = constants.APIPathSysHealth + "?standbyok=true"

	loopbackHost = "localhost"

	caFileUsage = "Path to the CA certificate used to verify OpenBao TLS (empty to use system roots)"
)

func run(ctx context.Context) error {
	var (
		mode       string
		addr       string
		caFile     string
		serverName string
		timeout    time.Duration
	)

	flag.StringVar(&mode, "mode", "", "Probe mode: startup, liveness, or readiness")
	defaultAddr := fmt.Sprintf("https://%s:%d", loopbackHost, constants.PortAPI)
	flag.StringVar(&addr, "addr", defaultAddr,
		"Base address for OpenBao (must be reachable from inside the pod)")
	flag.StringVar(&caFile, "ca-file", constants.PathTLSCACert, caFileUsage)
	flag.StringVar(&serverName, "servername", "", "TLS SNI server name (overrides hostname from addr)")
	flag.DurationVar(&timeout, "timeout", 4*time.Second,
		"Timeout for the HTTP request (should be less than the Kubernetes probe timeoutSeconds)")
	flag.Parse()

	switch mode {
	case modeStartup, modeLiveness, modeReadiness:
	default:
		return fmt.Errorf("invalid -mode %q (expected %q, %q, or %q)",
			mode, modeStartup, modeLiveness, modeReadiness)
	}

	// Determine if we should allow insecure fallback for ACME mode
	// For readiness probes in ACME mode, allow insecure loopback fallback during initial
	// certificate acquisition. This handles the case where OpenBao is still obtaining the
	// ACME certificate and using a temporary self-signed certificate.
	isACMEMode := serverName != "" && serverName != loopbackHost
	allowInsecureFallback := mode == modeLiveness || (mode == modeReadiness && isACMEMode)

	prober, err := probe.NewProber(probe.ProberConfig{
		Addr:                  addr,
		CAFile:                caFile,
		ServerName:            serverName,
		Timeout:               timeout,
		AllowInsecureFallback: allowInsecureFallback,
		StartupPath:           "",
		LivenessPath:          livenessPath,
		ReadinessPath:         readinessPath,
	})
	if err != nil {
		return fmt.Errorf("failed to create prober: %w", err)
	}

	switch mode {
	case modeStartup:
		return prober.CheckStartup(ctx)
	case modeLiveness:
		return prober.CheckLiveness(ctx)
	case modeReadiness:
		return prober.CheckReadiness(ctx)
	default:
		return fmt.Errorf("unknown mode: %q", mode)
	}
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "probe failed: %v\n", err)
		os.Exit(1)
	}
}
