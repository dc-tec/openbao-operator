// Command bao-policy-approval applies one reviewed operator approval from a GitOps Job.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const absentPolicy = "absent"

type options struct {
	address         string
	caFile          string
	serverName      string
	authMount       string
	role            string
	jwtFile         string
	policyFile      string
	digest          string
	expectedCurrent string
	timeout         time.Duration
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil && !errors.Is(err, flag.ErrHelp) {
		_, _ = fmt.Fprintf(os.Stderr, "policy approval failed: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	var opts options
	flags := flag.NewFlagSet("bao-policy-approval", flag.ContinueOnError)
	flags.StringVar(&opts.address, "address", "", "OpenBao HTTPS address")
	flags.StringVar(&opts.caFile, "ca-file", "", "Public CA PEM file (empty uses system roots)")
	flags.StringVar(&opts.serverName, "tls-server-name", "", "TLS server name (empty uses the address hostname)")
	flags.StringVar(&opts.authMount, "auth-mount", "jwt-operator", "JWT auth mount")
	flags.StringVar(&opts.role, "role", "", "Dedicated approver JWT role")
	flags.StringVar(&opts.jwtFile, "jwt-file", "", "Projected ServiceAccount token file")
	flags.StringVar(&opts.policyFile, "policy-file", "", "Reviewed approval HCL file")
	flags.StringVar(&opts.digest, "sha256", "", "Expected SHA-256 of the exact approval file")
	flags.StringVar(&opts.expectedCurrent, "expected-current-sha256", "",
		"Expected current approval SHA-256, or absent for creation")
	flags.DurationVar(&opts.timeout, "timeout", 2*time.Minute, "Deadline for the complete operation")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if err := opts.validate(); err != nil {
		return err
	}
	policy, err := os.ReadFile(opts.policyFile)
	if err != nil {
		return fmt.Errorf("read approval file: %w", err)
	}
	if strings.TrimSpace(string(policy)) == "" || digestOf(string(policy)) != opts.digest {
		return fmt.Errorf("approval file is empty or does not match --sha256")
	}
	jwt, err := os.ReadFile(opts.jwtFile)
	if err != nil {
		return fmt.Errorf("read projected token: %w", err)
	}
	if strings.TrimSpace(string(jwt)) == "" {
		return fmt.Errorf("projected token is empty")
	}
	client, err := newApprovalClient(opts)
	if err != nil {
		return err
	}
	defer client.http.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	if err := client.checkVersion(ctx); err != nil {
		return err
	}
	if err := client.login(ctx, opts.authMount, opts.role, strings.TrimSpace(string(jwt))); err != nil {
		return err
	}
	changed, err := client.apply(ctx, string(policy), opts.expectedCurrent)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "approval verified: sha256=%s changed=%t\n", opts.digest, changed)
	return err
}

func (o options) validate() error {
	u, err := url.Parse(o.address)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return fmt.Errorf("--address must be an HTTPS origin without credentials, path, query, or fragment")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+(/[a-zA-Z0-9_-]+)*$`).MatchString(o.authMount) {
		return fmt.Errorf("--auth-mount must be a mount path without URL delimiters or traversal")
	}
	if o.role == "" || o.jwtFile == "" || o.policyFile == "" || o.timeout <= 0 {
		return fmt.Errorf("--role, --jwt-file, --policy-file, and a positive --timeout are required")
	}
	if !validDigest(o.digest) || (o.expectedCurrent != absentPolicy && !validDigest(o.expectedCurrent)) {
		return fmt.Errorf("--sha256 and --expected-current-sha256 require lowercase SHA-256 hex; " +
			"the latter also accepts absent")
	}
	return nil
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func digestOf(policy string) string {
	digest := sha256.Sum256([]byte(policy))
	return hex.EncodeToString(digest[:])
}
