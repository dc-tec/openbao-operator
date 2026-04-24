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
	"sync"
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

var controllerMetricsServiceNames = []string{
	// Manifest installs apply the kustomize name prefix to controller-metrics-service.
	controllerMetricsServiceName,
	// Helm installs name the service from the release fullname plus controller-metrics.
	"openbao-operator-controller-metrics",
}

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
	// Try both install naming conventions for the ClusterRole.
	roleNames := []string{metricsReaderClusterRoleName, "metrics-reader"}

	var lastErr error
	var lastOut string
	selectedRoleName := ""
	for _, roleName := range roleNames {
		cmd := exec.Command("kubectl", "get", "clusterrole", roleName)
		out, err := utils.Run(cmd)
		if err != nil {
			lastErr = err
			lastOut = out
			if strings.Contains(out, "NotFound") || strings.Contains(strings.ToLower(out), "not found") {
				continue
			}
			return err
		}
		selectedRoleName = roleName
		break
	}

	if selectedRoleName == "" {
		if lastErr != nil && lastOut != "" {
			return fmt.Errorf("%w: %s", lastErr, lastOut)
		}
		return lastErr
	}

	createBinding := func() error {
		cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsReaderBindingName,
			fmt.Sprintf("--clusterrole=%s", selectedRoleName),
			fmt.Sprintf("--serviceaccount=%s:%s", operatorNamespace, controllerServiceAccountName),
		)
		out, err := utils.Run(cmd)
		if err == nil {
			return nil
		}
		outLower := strings.ToLower(out)
		if strings.Contains(out, "AlreadyExists") || strings.Contains(outLower, "already exists") {
			return err
		}
		return err
	}

	if err := createBinding(); err == nil {
		return nil
	} else {
		cmd := exec.Command("kubectl", "get", "clusterrolebinding", metricsReaderBindingName, "-o", "jsonpath={.roleRef.name}")
		out, getErr := utils.Run(cmd)
		if getErr != nil {
			return err
		}
		if strings.TrimSpace(out) == selectedRoleName {
			return nil
		}
	}

	cmd := exec.Command("kubectl", "delete", "clusterrolebinding", metricsReaderBindingName)
	if _, err := utils.Run(cmd); err != nil {
		return err
	}
	return createBinding()
}

func findControllerMetricsServiceName(operatorNamespace string) (string, error) {
	var lastErr error
	var lastOut string
	for _, serviceName := range controllerMetricsServiceNames {
		cmd := exec.Command("kubectl", "get", "service", serviceName, "-n", operatorNamespace)
		out, err := utils.Run(cmd)
		if err == nil {
			return serviceName, nil
		}
		lastErr = err
		lastOut = out
		if strings.Contains(out, "NotFound") || strings.Contains(strings.ToLower(out), "not found") {
			continue
		}
		return "", err
	}
	if lastErr != nil && lastOut != "" {
		return "", fmt.Errorf("%w: %s", lastErr, lastOut)
	}
	return "", lastErr
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
	metricsServiceName, err := findControllerMetricsServiceName(operatorNamespace)
	if err != nil {
		return "", err
	}

	deadline := time.Now().Add(timeout)
	var lastOutput string
	var lastErr error
	for time.Now().Before(deadline) {
		metricsOutput, err := waitForControllerMetricSubstringsWithPortForward(operatorNamespace, metricsServiceName, token, deadline, substrings...)
		if metricsOutput != "" {
			lastOutput = metricsOutput
		}
		if err == nil {
			return metricsOutput, nil
		}
		lastErr = err

		sleep := time.Second
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep > 0 {
			time.Sleep(sleep)
		}
	}

	if lastErr != nil {
		return lastOutput, fmt.Errorf("timed out waiting for controller metrics after %s: %w", timeout, lastErr)
	}
	return lastOutput, fmt.Errorf("timed out waiting for controller metrics after %s", timeout)
}

func waitForControllerMetricSubstringsWithPortForward(
	operatorNamespace string,
	metricsServiceName string,
	token string,
	deadline time.Time,
	substrings ...string,
) (string, error) {
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
	serviceRef := fmt.Sprintf("service/%s", metricsServiceName)
	portForwardArg := fmt.Sprintf("%d:8443", localPort)
	portForwardCmd := exec.Command("kubectl", "port-forward", "--namespace", operatorNamespace, serviceRef, portForwardArg)
	portForwardCmd.Stdout = io.Discard
	portForwardStderr := &lockedStringBuffer{}
	portForwardCmd.Stderr = portForwardStderr

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
	if deadline.Before(forwardReadyDeadline) {
		forwardReadyDeadline = deadline
	}
	forwardReady := false
	for time.Now().Before(forwardReadyDeadline) {
		select {
		case waitErr := <-waitCh:
			if waitErr != nil {
				return "", fmt.Errorf("kubectl port-forward exited early: %w%s", waitErr, formattedPortForwardStderr(portForwardStderr))
			}
			return "", fmt.Errorf("kubectl port-forward exited before becoming ready%s", formattedPortForwardStderr(portForwardStderr))
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
		return "", fmt.Errorf("timed out waiting for kubectl port-forward to become ready on localhost:%d%s", localPort, formattedPortForwardStderr(portForwardStderr))
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
	for time.Now().Before(deadline) {
		select {
		case waitErr := <-waitCh:
			if waitErr != nil {
				return lastOutput, fmt.Errorf("kubectl port-forward exited while reading metrics: %w%s", waitErr, formattedPortForwardStderr(portForwardStderr))
			}
			return lastOutput, fmt.Errorf("kubectl port-forward exited while reading metrics%s", formattedPortForwardStderr(portForwardStderr))
		default:
		}

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
		"timed out waiting for metrics endpoint %q to contain expected substrings",
		metricsURL,
	)
}

type lockedStringBuffer struct {
	mu      sync.Mutex
	builder strings.Builder
}

func (b *lockedStringBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.builder.Write(p)
}

func (b *lockedStringBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.builder.String()
}

func formattedPortForwardStderr(stderr *lockedStringBuffer) string {
	if stderr == nil {
		return ""
	}
	output := strings.TrimSpace(stderr.String())
	if output == "" {
		return ""
	}
	return ": " + output
}
