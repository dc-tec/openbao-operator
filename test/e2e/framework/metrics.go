//go:build e2e
// +build e2e

package framework

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/dc-tec/openbao-operator/test/utils"
)

const (
	controllerMetricsServiceName = "openbao-operator-controller-metrics-service"
	controllerServiceAccountName = "openbao-operator-controller"
	// metricsReaderClusterRoleName is the cluster role that grants GET /metrics.
	//
	// Note: In manifest installs, kustomize namePrefix results in "openbao-operator-metrics-reader".
	// In Helm installs, the ClusterRole is named "metrics-reader".
	metricsReaderClusterRoleName = "openbao-operator-metrics-reader"
	metricsReaderBindingName     = "openbao-operator-metrics-binding"
)

func findFreeLocalPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = listener.Close()
	}()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address type %T", listener.Addr())
	}
	return addr.Port, nil
}

func ensureMetricsReaderClusterRoleBinding(operatorNamespace string) error {
	// Create is easiest; tolerate AlreadyExists to make this idempotent across tests.
	// Try both install naming conventions for the ClusterRole.
	roleNames := []string{metricsReaderClusterRoleName, "metrics-reader"}

	var lastErr error
	var lastOut string
	for _, roleName := range roleNames {
		cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsReaderBindingName,
			fmt.Sprintf("--clusterrole=%s", roleName),
			fmt.Sprintf("--serviceaccount=%s:%s", operatorNamespace, controllerServiceAccountName),
		)
		out, err := utils.Run(cmd)
		if err == nil {
			return nil
		}
		outLower := strings.ToLower(out)
		if strings.Contains(out, "AlreadyExists") || strings.Contains(outLower, "already exists") {
			return nil
		}

		lastErr = err
		lastOut = out

		// If the role name is wrong for this install method, try the next candidate.
		if strings.Contains(out, "NotFound") || strings.Contains(strings.ToLower(out), "not found") {
			continue
		}
		return err
	}

	if lastErr != nil && lastOut != "" {
		return fmt.Errorf("%w: %s", lastErr, lastOut)
	}
	return lastErr
}

func createServiceAccountToken(operatorNamespace string) (string, error) {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "create", "token", controllerServiceAccountName,
			"-n", operatorNamespace,
			"--duration=1h",
		)
		output, err := cmd.CombinedOutput()
		if err == nil {
			token := strings.TrimSpace(string(output))
			if token != "" {
				return token, nil
			}
			lastErr = fmt.Errorf("token is empty in response")
		} else {
			lastErr = fmt.Errorf("kubectl create token failed: %w, output: %s", err, string(output))
		}
		time.Sleep(2 * time.Second)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("timed out creating service account token")
	}
	return "", lastErr
}

// WaitForControllerMetricSubstring fetches the controller /metrics endpoint until the provided
// substring is observed or the timeout elapses. It returns the last fetched metrics output.
func WaitForControllerMetricSubstring(operatorNamespace, substring string, timeout time.Duration) (string, error) {
	return WaitForControllerMetricSubstrings(operatorNamespace, timeout, substring)
}

// WaitForControllerMetricSubstrings fetches the controller /metrics endpoint until all provided
// substrings are observed (anywhere in the output) or the timeout elapses. It returns the last
// fetched metrics output.
func WaitForControllerMetricSubstrings(operatorNamespace string, timeout time.Duration, substrings ...string) (string, error) {
	if operatorNamespace == "" {
		return "", fmt.Errorf("operator namespace is required")
	}
	if len(substrings) == 0 {
		return "", fmt.Errorf("at least one substring is required")
	}
	for i, s := range substrings {
		if strings.TrimSpace(s) == "" {
			return "", fmt.Errorf("substring %d is empty", i)
		}
	}
	if timeout <= 0 {
		return "", fmt.Errorf("timeout must be positive")
	}

	if err := ensureMetricsReaderClusterRoleBinding(operatorNamespace); err != nil {
		return "", err
	}

	token, err := createServiceAccountToken(operatorNamespace)
	if err != nil {
		return "", err
	}

	localPort, err := findFreeLocalPort()
	if err != nil {
		return "", err
	}

	// Keep each attempt bounded so the overall shell loop duration roughly tracks the provided timeout.
	const (
		connectTimeoutSeconds = 2
		maxTimeSeconds        = 3
		sleepSeconds          = 1
	)
	perAttemptBudget := time.Duration(maxTimeSeconds+sleepSeconds) * time.Second
	attempts := int(timeout / perAttemptBudget)
	if attempts < 1 {
		attempts = 1
	}

	serviceRef := fmt.Sprintf("service/%s", controllerMetricsServiceName)
	portForwardArg := fmt.Sprintf("%d:8443", localPort)
	portForwardCmd := exec.Command("kubectl", "port-forward", "--namespace", operatorNamespace, serviceRef, portForwardArg)
	portForwardCmd.Stdout = io.Discard
	portForwardCmd.Stderr = io.Discard

	if err := portForwardCmd.Start(); err != nil {
		return "", err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- portForwardCmd.Wait()
	}()
	defer func() {
		if portForwardCmd.Process != nil {
			_ = portForwardCmd.Process.Kill()
		}
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
		}
	}()

	forwardReadyDeadline := time.Now().Add(20 * time.Second)
	forwardReady := false
	for time.Now().Before(forwardReadyDeadline) {
		select {
		case waitErr := <-waitCh:
			if waitErr != nil {
				return "", fmt.Errorf("kubectl port-forward exited early: %w", waitErr)
			}
			return "", fmt.Errorf("kubectl port-forward exited before becoming ready")
		default:
		}

		conn, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", localPort), 500*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			forwardReady = true
			break
		}

		time.Sleep(200 * time.Millisecond)
	}
	if !forwardReady {
		return "", fmt.Errorf("timed out waiting for kubectl port-forward to become ready on localhost:%d", localPort)
	}

	metricsURL := fmt.Sprintf("https://127.0.0.1:%d/metrics", localPort)
	httpClient := &http.Client{
		Timeout: time.Duration(connectTimeoutSeconds+maxTimeSeconds) * time.Second,
		Transport: &http.Transport{
			//nolint:gosec // E2E port-forward targets localhost while cert SANs are service DNS names.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var lastOutput string
	for i := 0; i < attempts; i++ {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, metricsURL, nil)
		if reqErr != nil {
			return "", reqErr
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, reqErr := httpClient.Do(req)
		if reqErr == nil {
			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr == nil {
				lastOutput = string(body)
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					matched := true
					for _, s := range substrings {
						if !strings.Contains(lastOutput, s) {
							matched = false
							break
						}
					}
					if matched {
						return lastOutput, nil
					}
				}
			}
		}

		time.Sleep(time.Duration(sleepSeconds) * time.Second)
	}

	return lastOutput, fmt.Errorf(
		"timed out waiting for metrics endpoint %q to contain expected substrings after %d attempts",
		metricsURL,
		attempts,
	)
}
