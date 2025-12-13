package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	modeStartup   = "startup"
	modeLiveness  = "liveness"
	modeReadiness = "readiness"

	livenessPath  = "/v1/sys/health?standbyok=true&sealedcode=204&uninitcode=204"
	readinessPath = "/v1/sys/health?standbyok=true"
)

func main() {
	var (
		mode    string
		addr    string
		caFile  string
		timeout time.Duration
	)

	flag.StringVar(&mode, "mode", "", "Probe mode: liveness or readiness")
	flag.StringVar(&addr, "addr", "https://localhost:8200",
		"Base address for OpenBao (must be reachable from inside the pod)")
	flag.StringVar(&caFile, "ca-file", "/etc/bao/tls/ca.crt", "Path to the CA certificate used to verify OpenBao TLS")
	flag.DurationVar(&timeout, "timeout", 4*time.Second,
		"Timeout for the HTTP request (should be less than the Kubernetes probe timeoutSeconds)")
	flag.Parse()

	switch mode {
	case modeStartup, modeLiveness, modeReadiness:
	default:
		_, _ = fmt.Fprintf(os.Stderr, "invalid -mode %q (expected %q, %q, or %q)\n",
			mode, modeStartup, modeLiveness, modeReadiness)
		os.Exit(2)
	}

	serverName, hostPort, err := parseAddr(addr)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "invalid -addr %q: %v\n", addr, err)
		os.Exit(1)
	}

	if mode == modeStartup {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		if err := tcpDial(ctx, hostPort); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "tcp dial failed: %v\n", err)
			os.Exit(1)
		}

		os.Exit(0)
	}

	path := readinessPath
	if mode == modeLiveness {
		path = livenessPath
	}

	base := strings.TrimRight(addr, "/")
	probeURL := base + path

	allowInsecureLocalFallback := serverName == "localhost"
	client, err := newHTTPClient(caFile, serverName, timeout, allowInsecureLocalFallback)
	if err != nil {
		if mode == modeLiveness {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			if tcpErr := tcpDial(ctx, hostPort); tcpErr == nil {
				os.Exit(0)
			}
		}

		_, _ = fmt.Fprintf(os.Stderr, "failed to create HTTP client: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create request: %v\n", err)
		os.Exit(2)
	}

	resp, err := client.Do(req)
	if err != nil {
		// For liveness we can fall back to a TCP dial when TLS assets are not yet
		// available (e.g., ACME mode) or when the OpenBao process is temporarily
		// not accepting connections yet. Readiness remains strict.
		if mode == modeLiveness {
			tcpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			if tcpErr := tcpDial(tcpCtx, hostPort); tcpErr == nil {
				os.Exit(0)
			}
		}

		_, _ = fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch mode {
	case modeLiveness:
		// livenessPath returns 2xx even when sealed/uninitialized due to
		// sealedcode/uninitcode. Also accept 429 (standby).
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			os.Exit(0)
		}
		if resp.StatusCode == 429 {
			os.Exit(0)
		}
		os.Exit(1)
	case modeReadiness:
		// Readiness should only succeed when initialized and unsealed.
		// OpenBao returns:
		// - 200 (leader) or 429 (standby) when initialized+unsealed.
		// - 501 when not initialized.
		// - 503 when sealed.
		// - 473 for performance standby (treat as ready).
		switch resp.StatusCode {
		case 200, 429, 473:
			os.Exit(0)
		default:
			os.Exit(1)
		}
	default:
		os.Exit(2)
	}
}

func newHTTPClient(caFile string, serverName string, timeout time.Duration, allowInsecureLocalFallback bool) (*http.Client, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		// In ACME TLS mode, the operator does not mount TLS Secrets and OpenBao
		// manages certificates internally. For localhost probes, allow a fallback
		// to insecure TLS when the CA file is missing so readiness can still
		// accurately reflect OpenBao health (initialized/unsealed).
		if allowInsecureLocalFallback && os.IsNotExist(err) {
			tlsConfig := &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			}
			if serverName != "" {
				tlsConfig.ServerName = serverName
			}

			transport := &http.Transport{
				TLSClientConfig:       tlsConfig,
				TLSHandshakeTimeout:   timeout,
				ResponseHeaderTimeout: timeout,
				DisableKeepAlives:     true,
			}

			return &http.Client{Transport: transport}, nil
		}

		return nil, fmt.Errorf("failed to read CA file %q: %w", caFile, err)
	}

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("failed to parse CA PEM from %q", caFile)
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}
	if serverName != "" {
		tlsConfig.ServerName = serverName
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

func tcpDial(ctx context.Context, address string) error {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

func parseAddr(rawAddr string) (serverName string, hostPort string, err error) {
	parsed, err := url.Parse(rawAddr)
	if err != nil {
		return "", "", fmt.Errorf("parse url: %w", err)
	}

	if parsed.Scheme == "" {
		return "", "", fmt.Errorf("missing scheme (expected https)")
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if host == "" || port == "" {
		return "", "", fmt.Errorf("expected host:port in url")
	}

	hostPort = net.JoinHostPort(host, port)
	if net.ParseIP(host) != nil {
		return "", hostPort, nil
	}

	return host, hostPort, nil
}
