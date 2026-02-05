//go:build e2e
// +build e2e

package framework

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
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

func randomHex(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
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

func escapeForSingleQuotes(s string) string {
	// Safely embed arbitrary text inside a single-quoted POSIX shell string.
	// Example: abc'def -> 'abc'"'"'def'
	return strings.ReplaceAll(s, "'", `'"'"'`)
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

	suffix, err := randomHex(4)
	if err != nil {
		return "", err
	}
	podName := fmt.Sprintf("curl-metrics-%s", suffix)

	// Keep each attempt bounded so the overall shell loop duration roughly tracks the provided timeout.
	const (
		curlConnectTimeoutSeconds = 2
		curlMaxTimeSeconds        = 3
		sleepSeconds              = 1
	)
	perAttemptBudget := time.Duration(curlMaxTimeSeconds+sleepSeconds) * time.Second
	attempts := int(timeout / perAttemptBudget)
	if attempts < 1 {
		attempts = 1
	}

	var checks []string
	for _, s := range substrings {
		needle := escapeForSingleQuotes(s)
		checks = append(checks, fmt.Sprintf(`echo "$out" | grep -Fq '%s'`, needle))
	}
	condition := strings.Join(checks, " && ")

	script := fmt.Sprintf(
		`attempts=%d; i=0; while [ "$i" -lt "$attempts" ]; do `+
			`out="$(curl -sfk --connect-timeout %d --max-time %d -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics 2>/dev/null || true)"; `+
			`%s && { echo "$out"; exit 0; }; `+
			`i=$((i+1)); sleep %d; `+
			`done; echo "$out"; exit 1`,
		attempts,
		curlConnectTimeoutSeconds,
		curlMaxTimeSeconds,
		token,
		controllerMetricsServiceName,
		operatorNamespace,
		condition,
		sleepSeconds,
	)

	cmd := exec.Command("kubectl", "run", podName, "--restart=Never",
		"--namespace", operatorNamespace,
		"--image=curlimages/curl:latest",
		"--overrides",
		fmt.Sprintf(`{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl:latest",
					"imagePullPolicy": "IfNotPresent",
					"command": ["/bin/sh", "-c"],
					"args": [%q],
					"securityContext": {
						"readOnlyRootFilesystem": true,
						"allowPrivilegeEscalation": false,
						"capabilities": {
							"drop": ["ALL"]
						},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {
							"type": "RuntimeDefault"
						}
					}
				}],
				"serviceAccountName": "%s"
			}
		}`, script, controllerServiceAccountName),
	)

	_, runErr := utils.Run(cmd)
	defer func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "pod", podName, "-n", operatorNamespace, "--ignore-not-found"))
	}()
	if runErr != nil {
		return "", runErr
	}

	deadline := time.Now().Add(timeout + 30*time.Second)
	for time.Now().Before(deadline) {
		cmd = exec.Command("kubectl", "get", "pod", podName,
			"-o", "jsonpath={.status.phase}",
			"-n", operatorNamespace,
		)
		phase, err := utils.Run(cmd)
		if err == nil {
			phase = strings.TrimSpace(phase)
			if phase == "Succeeded" {
				logs, logsErr := utils.Run(exec.Command("kubectl", "logs", podName, "-n", operatorNamespace))
				if logsErr != nil {
					return "", logsErr
				}
				return logs, nil
			}
			if phase == "Failed" {
				logs, _ := utils.Run(exec.Command("kubectl", "logs", podName, "-n", operatorNamespace))
				return logs, fmt.Errorf("metrics curl pod %q failed", podName)
			}
		}

		time.Sleep(2 * time.Second)
	}

	logs, _ := utils.Run(exec.Command("kubectl", "logs", podName, "-n", operatorNamespace))
	return logs, fmt.Errorf("timed out waiting for metrics curl pod %q to complete", podName)
}
