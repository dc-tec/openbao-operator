//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/test/utils"
)

const operatorControllerWebhookServiceName = "openbao-operator-controller-webhook"

func waitForNamespaceDeletionIfTerminating(ctx context.Context, namespace string, timeout time.Duration, pollInterval time.Duration) error {
	if strings.TrimSpace(namespace) == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}

	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create namespace readiness client: %w", err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		current := &corev1.Namespace{}
		err := c.Get(ctx, client.ObjectKey{Name: namespace}, current)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get namespace %q: %w", namespace, err)
		}
		if current.DeletionTimestamp == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for namespace %q deletion: %w", namespace, ctx.Err())
		case <-timer.C:
			return fmt.Errorf("namespace %q is still terminating after %s", namespace, timeout)
		case <-ticker.C:
		}
	}
}

func resolveE2EAPIServerCIDR(ctx context.Context) (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("E2E_API_SERVER_CIDR")); explicit != "" {
		return explicit, nil
	}

	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return "", err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return "", fmt.Errorf("failed to create API server discovery client: %w", err)
	}

	service := &corev1.Service{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "kubernetes"}, service); err != nil {
		return "", fmt.Errorf("failed to get kubernetes service: %w", err)
	}

	ip := net.ParseIP(strings.TrimSpace(service.Spec.ClusterIP))
	if ip == nil {
		return "", fmt.Errorf("invalid kubernetes service ClusterIP %q", service.Spec.ClusterIP)
	}
	if ip.To4() != nil {
		return ip.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}

func resolveE2EAPIServerEndpointIPs(ctx context.Context) (string, error) {
	if explicit := strings.TrimSpace(os.Getenv(claimE2EAPIServerEndpointIPsEnv)); explicit != "" {
		return explicit, nil
	}

	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return "", err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return "", fmt.Errorf("failed to create API endpoint discovery client: %w", err)
	}

	//nolint:staticcheck // The suite still targets the legacy kubernetes Endpoints object for API server discovery.
	endpoints := &corev1.Endpoints{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "kubernetes"}, endpoints); err != nil {
		return "", fmt.Errorf("failed to get kubernetes endpoints: %w", err)
	}

	seen := map[string]struct{}{}
	var ips []string
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			ip := strings.TrimSpace(address.IP)
			if net.ParseIP(ip) == nil {
				continue
			}
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("kubernetes endpoints did not contain any valid addresses")
	}
	slices.Sort(ips)
	return strings.Join(ips, ","), nil
}

func upsertEnvVar(container *corev1.Container, name string, value string) bool {
	if container == nil || strings.TrimSpace(name) == "" {
		return false
	}
	for i := range container.Env {
		if container.Env[i].Name != name {
			continue
		}
		if container.Env[i].Value == value {
			return false
		}
		container.Env[i].Value = value
		return true
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
	return true
}

func withEnv(key string, value string, fn func()) {
	previousValue, hadPrevious := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		panic(err)
	}
	defer func() {
		if hadPrevious {
			_ = os.Setenv(key, previousValue)
			return
		}
		_ = os.Unsetenv(key)
	}()
	fn()
}

func envOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
}

func envBoolDefaultTrue(key string) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return true
	}
	return !strings.EqualFold(value, "false")
}

func waitForDeploymentsAvailable(namespace string, timeout time.Duration) error {
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}

	seconds := int(timeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	cmd := exec.Command("kubectl",
		"wait",
		"--for=condition=Available",
		"deployment",
		"-l", "app.kubernetes.io/name=openbao-operator",
		"-n", namespace,
		"--timeout", fmt.Sprintf("%ds", seconds),
	) // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err
}

func waitForCRDsEstablished(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}

	seconds := int(timeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	cmd := exec.Command("kubectl",
		"wait",
		"--for=condition=Established",
		"crd/openbaoclusters.openbao.org",
		"crd/openbaotenants.openbao.org",
		"crd/openbaorestores.openbao.org",
		"--timeout", fmt.Sprintf("%ds", seconds),
	) // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err
}

func waitForCoreDNSAvailable(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}

	seconds := int(timeout.Seconds())
	if seconds < 1 {
		seconds = 1
	}

	cmd := exec.Command("kubectl",
		"wait",
		"--for=condition=Available",
		"deployment/coredns",
		"-n", "kube-system",
		"--timeout", fmt.Sprintf("%ds", seconds),
	) // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err
}

func waitForServiceEndpoints(namespace string, serviceName string, timeout time.Duration) error {
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("service name is required")
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}

	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return err
	}
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create service-endpoint readiness client: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for {
		//nolint:staticcheck // The readiness gate observes the service Endpoints published by the cluster.
		endpoints := &corev1.Endpoints{}
		err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: serviceName}, endpoints)
		if err == nil {
			for _, subset := range endpoints.Subsets {
				if len(subset.Addresses) > 0 {
					// Give the apiserver proxy/endpoints controller a short settle window before the
					// suite issues the first admission-backed create request.
					time.Sleep(2 * time.Second)
					return nil
				}
			}
		} else if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get endpoints %s/%s: %w", namespace, serviceName, err)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("service %s/%s did not publish ready endpoints within %s", namespace, serviceName, timeout)
		}
		time.Sleep(time.Second)
	}
}

func waitForClaimAdmissionWebhookReady(namespace string, timeout time.Duration) error {
	if strings.TrimSpace(namespace) == "" {
		return fmt.Errorf("namespace is required")
	}
	if timeout <= 0 {
		return fmt.Errorf("timeout must be > 0")
	}

	probeName := "webhook-probe-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	manifest := fmt.Sprintf(`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaim
metadata:
  namespace: %s
  name: %s
spec:
  tenantRef:
    name: probe-tenant
  serviceProfileRef:
    name: probe-profile
`, namespace, probeName)

	deadline := time.Now().Add(timeout)
	for {
		cmd := exec.Command("kubectl", "create", "--dry-run=server", "-f", "-") // #nosec G204 -- test harness command
		cmd.Stdin = strings.NewReader(manifest)
		_, err := utils.Run(cmd)
		if err == nil {
			return nil
		}

		errText := err.Error()
		if !strings.Contains(errText, "failed calling webhook") &&
			!strings.Contains(errText, "Bad Gateway") &&
			!strings.Contains(errText, "connection refused") &&
			!strings.Contains(errText, "i/o timeout") {
			return fmt.Errorf("claim admission readiness probe failed unexpectedly: %w", err)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("claim admission webhook did not become reachable within %s: %w", timeout, err)
		}
		time.Sleep(time.Second)
	}
}
