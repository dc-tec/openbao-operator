package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// This administrative client has one fixed write target. It never manages auth or operational policies.
const approvalPath = "/v1/sys/policies/acl/openbao-operator-policy-approval"

type approvalClient struct {
	address string
	http    *http.Client
	token   string
}

type storedPolicy struct {
	Policy  *string `json:"policy"`
	Version *int64  `json:"version"`
}

func newApprovalClient(opts options) (*approvalClient, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: opts.serverName}
	if opts.caFile != "" {
		pem, err := os.ReadFile(opts.caFile)
		if err != nil {
			return nil, fmt.Errorf("read public CA: %w", err)
		}
		tlsConfig.RootCAs = x509.NewCertPool()
		if !tlsConfig.RootCAs.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("public CA file contains no certificates")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &approvalClient{
		address: strings.TrimSuffix(opts.address, "/"),
		http: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
			// Never forward the JWT or administrative token to a redirect destination.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *approvalClient) checkVersion(ctx context.Context) error {
	var health struct {
		Version string `json:"version"`
	}
	if err := c.waitForHealth(ctx, &health); err != nil {
		return fmt.Errorf("check OpenBao version: %w", err)
	}
	// Older servers can silently ignore unknown CAS parameters when creating a missing policy.
	parts := strings.Split(strings.SplitN(health.Version, "-", 2)[0], ".")
	if len(parts) < 3 {
		return fmt.Errorf("OpenBao 2.6 or later is required for approval Jobs")
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 2 || (major == 2 && minor < 6) {
		return fmt.Errorf("OpenBao 2.6 or later is required for approval Jobs")
	}
	return nil
}

func (c *approvalClient) waitForHealth(ctx context.Context, health any) error {
	for {
		status, err := c.request(ctx, http.MethodGet,
			"/v1/sys/health?standbyok=true&perfstandbyok=true", nil, health)
		if err == nil {
			return nil
		}
		var certError *tls.CertificateVerificationError
		if errors.As(err, &certError) || (status != 0 && status < 500 && status != http.StatusTooManyRequests) {
			return err
		}
		// Pod networking and OpenBao readiness can lag behind Job creation. Retry only this unauthenticated read.
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for a healthy endpoint: %w (last attempt: %v)", ctx.Err(), err)
		case <-time.After(time.Second):
		}
	}
}

func (c *approvalClient) login(ctx context.Context, mount, role, jwt string) error {
	body, err := json.Marshal(struct {
		Role string `json:"role"`
		JWT  string `json:"jwt"`
	}{Role: role, JWT: jwt})
	if err != nil {
		return fmt.Errorf("encode JWT login: %w", err)
	}
	var response struct {
		Auth struct {
			Token string `json:"client_token"`
		} `json:"auth"`
	}
	status, err := c.request(ctx, http.MethodPost, "/v1/auth/"+mount+"/login", body, &response)
	if err != nil {
		return fmt.Errorf("JWT login: %w", err)
	}
	if status != http.StatusOK || response.Auth.Token == "" {
		return fmt.Errorf("JWT login returned no token (HTTP %d)", status)
	}
	c.token = response.Auth.Token
	return nil
}

func (c *approvalClient) apply(ctx context.Context, desired, expectedCurrent string) (bool, error) {
	current, err := c.read(ctx)
	if err != nil {
		return false, err
	}
	if current != nil && *current.Policy == desired {
		return false, nil // Also handles a retry after a successful write with a lost response.
	}
	currentDigest, version := absentPolicy, int64(-1)
	if current != nil {
		currentDigest, version = digestOf(*current.Policy), *current.Version
	}
	if currentDigest != expectedCurrent {
		return false, fmt.Errorf("current approval is %s; expected %s; review before submitting a new Job",
			currentDigest, expectedCurrent)
	}
	body, err := json.Marshal(struct {
		Policy      string `json:"policy"`
		CAS         int64  `json:"cas"`
		CASRequired bool   `json:"cas_required"`
	}{Policy: desired, CAS: version, CASRequired: true})
	if err != nil {
		return false, fmt.Errorf("encode approval: %w", err)
	}
	if _, err := c.request(ctx, http.MethodPost, approvalPath, body, nil); err != nil {
		return false, fmt.Errorf("write approval with compare-and-set: %w", err)
	}
	verified, err := c.read(ctx)
	if err != nil {
		return false, fmt.Errorf("verify written approval: %w", err)
	}
	if verified == nil || *verified.Policy != desired {
		return false, fmt.Errorf("approval changed before verification; review before retrying")
	}
	return true, nil
}

func (c *approvalClient) read(ctx context.Context) (*storedPolicy, error) {
	var response struct {
		Data storedPolicy `json:"data"`
	}
	status, err := c.request(ctx, http.MethodGet, approvalPath, nil, &response)
	if err != nil {
		return nil, fmt.Errorf("read approval: %w", err)
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if response.Data.Policy == nil || response.Data.Version == nil || *response.Data.Version < 0 {
		return nil, fmt.Errorf("approval response is missing policy or version; policy CAS support is required")
	}
	return &response.Data, nil
}

func (c *approvalClient) request(ctx context.Context, method, path string, body []byte, result any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.address+path, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if method == http.MethodGet && response.StatusCode == http.StatusNotFound {
		return response.StatusCode, nil
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNoContent {
		// Auth errors can echo credentials. Do not print response bodies.
		return response.StatusCode, fmt.Errorf("OpenBao returned HTTP %d; check server audit logs", response.StatusCode)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result); err != nil {
			return response.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return response.StatusCode, nil
}
