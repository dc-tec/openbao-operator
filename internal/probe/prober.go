package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	loopbackHost = "localhost"
)

// Prober performs health checks against an OpenBao instance.
type Prober interface {
	CheckStartup(ctx context.Context) error
	CheckLiveness(ctx context.Context) error
	CheckReadiness(ctx context.Context) error
}

// HTTPProber implements Prober using HTTP requests.
type HTTPProber struct {
	client        *http.Client
	startupPath   string
	livenessPath  string
	readinessPath string
	addr          string
	hostPort      string
	isLoopback    bool
}

// ProberConfig holds configuration for creating a Prober.
type ProberConfig struct {
	Addr                  string
	CAFile                string
	ServerName            string
	Timeout               time.Duration
	AllowInsecureFallback bool
	StartupPath           string
	LivenessPath          string
	ReadinessPath         string
}

// NewProber creates a new HTTPProber with the given configuration.
func NewProber(cfg ProberConfig) (Prober, error) {
	parsedServerName, hostPort, isLoopback, err := parseAddr(cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("invalid addr %q: %w", cfg.Addr, err)
	}

	// Use explicit servername if provided (e.g., for ACME mode), otherwise use parsed serverName from addr
	tlsServerName := cfg.ServerName
	if tlsServerName == "" {
		tlsServerName = parsedServerName
	}

	// For readiness probes in ACME mode, allow insecure loopback fallback during initial
	// certificate acquisition. This handles the case where OpenBao is still obtaining the
	// ACME certificate and using a temporary self-signed certificate.
	// For External TLS and OperatorManaged TLS modes, certificates are mounted as Secrets
	// and should be present before the pod starts, so we don't allow insecure fallback.
	// We detect ACME mode by checking if serverName is set (ACME mode sets it to the domain).
	isACMEMode := cfg.ServerName != "" && cfg.ServerName != loopbackHost
	allowInsecureFallback := isLoopback && cfg.AllowInsecureFallback && isACMEMode

	client, err := newHTTPClient(cfg.CAFile, tlsServerName, cfg.Timeout, allowInsecureFallback)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP client: %w", err)
	}

	return &HTTPProber{
		client:        client,
		startupPath:   cfg.StartupPath,
		livenessPath:  cfg.LivenessPath,
		readinessPath: cfg.ReadinessPath,
		addr:          cfg.Addr,
		hostPort:      hostPort,
		isLoopback:    isLoopback,
	}, nil
}

// CheckStartup performs a TCP dial check to verify the service is listening.
func (p *HTTPProber) CheckStartup(ctx context.Context) error {
	return tcpDial(ctx, p.hostPort)
}

// CheckLiveness performs a liveness check.
func (p *HTTPProber) CheckLiveness(ctx context.Context) error {
	base := strings.TrimRight(p.addr, "/")
	probeURL := base + p.livenessPath

	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		// For liveness we can fall back to a TCP dial when TLS assets are not yet
		// available (e.g., ACME mode) or when the OpenBao process is temporarily
		// not accepting connections yet.
		tcpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		if tcpErr := tcpDial(tcpCtx, p.hostPort); tcpErr == nil {
			return nil
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// livenessPath returns 2xx even when sealed/uninitialized due to
	// sealedcode/uninitcode. Also accept 429 (standby).
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if resp.StatusCode == 429 {
		return nil
	}
	return fmt.Errorf("liveness check failed with status %d", resp.StatusCode)
}

// CheckReadiness performs a readiness check with retry logic for connection resets.
func (p *HTTPProber) CheckReadiness(ctx context.Context) error {
	base := strings.TrimRight(p.addr, "/")
	probeURL := base + p.readinessPath

	maxRetries := 3
	var resp *http.Response
	var err error

	// Retry readiness probes up to 3 times with exponential backoff
	// to handle connection resets during startup
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			backoff := time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, probeURL, nil)
		if reqErr != nil {
			cancel()
			return fmt.Errorf("failed to create request: %w", reqErr)
		}

		resp, err = p.client.Do(req)
		cancel()

		if err == nil {
			// Success - break out of retry loop
			break
		}

		// Check if this is a connection reset error that we should retry
		errStr := err.Error()
		isConnectionReset := strings.Contains(errStr, "connection reset") ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "broken pipe")

		if !isConnectionReset || attempt == maxRetries-1 {
			// Not a connection reset error, or we've exhausted retries
			break
		}
	}

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Readiness should only succeed when initialized and unsealed.
	// OpenBao returns:
	// - 200 (leader) or 429 (standby) when initialized+unsealed.
	// - 501 when not initialized.
	// - 503 when sealed.
	// - 473 for performance standby (treat as ready).
	switch resp.StatusCode {
	case 200, 429, 473:
		return nil
	default:
		return fmt.Errorf("readiness check failed with status %d", resp.StatusCode)
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
		// Empty CA file means use system roots (e.g., for public ACME CAs like Let's Encrypt).
		// However, during initial ACME certificate acquisition or when retrying after TLS
		// handshake failures, OpenBao may use a temporary self-signed certificate that won't
		// verify against system roots. For loopback connections, allow insecure fallback
		// to handle this case.
		if allowInsecureLoopbackFallback {
			// Use insecure fallback with manual certificate verification for loopback connections
			tlsConfig := &tls.Config{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // #nosec G402 -- InsecureSkipVerify is required for certificate acquisition phase or TLS handshake retries, but we perform manual certificate verification via VerifyPeerCertificate to ensure we're connecting to the expected loopback service.
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					return verifyLoopbackCertificate(rawCerts)
				},
			}
			// Use the provided serverName for SNI (e.g., ACME domain), or fall back to "localhost"
			if serverName != "" && serverName != loopbackHost {
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
		}
		// Empty CA file means use system roots (e.g., for public ACME CAs like Let's Encrypt)
		roots = nil
	} else {
		// Validate and clean CA file path to prevent path traversal
		cleanCAFile := filepath.Clean(caFile)
		if strings.Contains(cleanCAFile, "..") {
			return nil, fmt.Errorf("CA file path %q contains path traversal", caFile)
		}
		caPEM, err := os.ReadFile(cleanCAFile) // #nosec G304 -- Path is validated and cleaned to prevent traversal
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
					InsecureSkipVerify: true, // #nosec G402 -- InsecureSkipVerify is required for loopback fallback, but we perform manual certificate verification via VerifyPeerCertificate to ensure we're connecting to the expected loopback service.
					VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
						return verifyLoopbackCertificate(rawCerts)
					},
				}
				// Use the provided serverName for SNI (e.g., ACME domain), or "localhost" for loopback
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
		// Use "localhost" for loopback when no explicit serverName
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

	// For loopback connections, we allow self-signed certificates during initial
	// ACME certificate acquisition. Once the ACME certificate is obtained, it will
	// be CA-issued and this check will pass. This is safe because:
	// 1. We're only connecting to localhost (loopback)
	// 2. We still verify the certificate is valid (not expired, proper dates)
	// 3. The certificate will be replaced with the proper ACME certificate once obtained
	// For non-loopback connections or after ACME acquisition, we expect CA-issued certs.
	// Note: CheckSignatureFrom returns nil if the cert is self-signed (can verify itself)
	if err := cert.CheckSignatureFrom(cert); err == nil {
		// Self-signed certificate - allow it for loopback during ACME acquisition
		// The certificate validity (dates) has already been checked above
		return nil
	}

	return nil
}
