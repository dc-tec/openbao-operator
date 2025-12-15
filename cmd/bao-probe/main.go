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

	"github.com/openbao/operator/internal/constants"
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

func main() {
	var (
		mode       string
		addr       string
		caFile     string
		serverName string
		timeout    time.Duration
	)

	flag.StringVar(&mode, "mode", "", "Probe mode: liveness or readiness")
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
		_, _ = fmt.Fprintf(os.Stderr, "invalid -mode %q (expected %q, %q, or %q)\n",
			mode, modeStartup, modeLiveness, modeReadiness)
		os.Exit(2)
	}

	parsedServerName, hostPort, isLoopback, err := parseAddr(addr)
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

	// Use explicit servername if provided (e.g., for ACME mode), otherwise use parsed serverName from addr
	tlsServerName := serverName
	if tlsServerName == "" {
		tlsServerName = parsedServerName
	}

	client, err := newHTTPClient(caFile, tlsServerName, timeout, isLoopback)
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

func newHTTPClient(
	caFile string,
	serverName string,
	timeout time.Duration,
	allowInsecureLoopbackFallback bool,
) (*http.Client, error) {
	var roots *x509.CertPool

	if caFile == "" {
		// Empty CA file means use system roots (e.g., for public ACME CAs like Let's Encrypt)
		roots = nil
	} else {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			// If the CA file doesn't exist and we have a serverName (ACME mode), use system roots.
			// This handles public ACME CAs where no CA file is provided.
			if os.IsNotExist(err) && serverName != "" {
				// Use system roots for public ACME CAs
				roots = nil
			} else if allowInsecureLoopbackFallback && os.IsNotExist(err) {
				// Fallback for non-ACME loopback probes when CA file is missing
				tlsConfig := &tls.Config{
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: true,
					VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
						return verifyLoopbackCertificate(rawCerts)
					},
				}
				// Use the provided serverName for SNI (e.g., ACME domain), or fall back to "localhost"
				// for backward compatibility with non-ACME loopback probes.
				if serverName != "" {
					tlsConfig.ServerName = serverName
				} else {
					tlsConfig.ServerName = loopbackHost
				}

				transport := &http.Transport{
					TLSClientConfig:       tlsConfig,
					TLSHandshakeTimeout:   timeout,
					ResponseHeaderTimeout: timeout,
					DisableKeepAlives:     true,
				}

				return &http.Client{Transport: transport}, nil
			} else {
				return nil, fmt.Errorf("failed to read CA file %q: %w", caFile, err)
			}
		} else {
			// CA file exists, parse it
			roots = x509.NewCertPool()
			if !roots.AppendCertsFromPEM(caPEM) {
				return nil, fmt.Errorf("failed to parse CA PEM from %q", caFile)
			}
		}
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots, // nil means use system roots
	}
	// Use the provided serverName for SNI (e.g., ACME domain). This is critical for ACME mode
	// to prevent OpenBao from attempting ACME for "localhost" when probes connect via loopback.
	if serverName != "" {
		tlsConfig.ServerName = serverName
	} else if allowInsecureLoopbackFallback {
		// Fallback for backward compatibility: use "localhost" for loopback when no explicit serverName
		tlsConfig.ServerName = loopbackHost
	}
	// If neither condition is met, let Go handle SNI automatically

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

func parseAddr(rawAddr string) (serverName string, hostPort string, isLoopback bool, err error) {
	parsed, err := url.Parse(rawAddr)
	if err != nil {
		return "", "", false, fmt.Errorf("parse url: %w", err)
	}

	if parsed.Scheme == "" {
		return "", "", false, fmt.Errorf("missing scheme (expected https)")
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if host == "" || port == "" {
		return "", "", false, fmt.Errorf("expected host:port in url")
	}

	hostPort = net.JoinHostPort(host, port)
	if net.ParseIP(host) != nil {
		ip := net.ParseIP(host)
		return "", hostPort, ip != nil && ip.IsLoopback(), nil
	}

	return host, hostPort, host == loopbackHost, nil
}

func verifyLoopbackCertificate(rawCerts [][]byte) error {
	if len(rawCerts) == 0 {
		return fmt.Errorf("no peer certificates presented")
	}

	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("failed to parse peer certificate: %w", err)
	}

	now := time.Now()
	if now.Before(cert.NotBefore) {
		return fmt.Errorf("certificate not valid yet (notBefore=%s)", cert.NotBefore.Format(time.RFC3339))
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("certificate expired (notAfter=%s)", cert.NotAfter.Format(time.RFC3339))
	}

	// For ACME mode we expect a CA-issued cert, not a self-signed placeholder.
	if err := cert.CheckSignatureFrom(cert); err == nil {
		return fmt.Errorf("certificate is self-signed")
	}

	return nil
}
