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
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openbao/operator/test/utils"
)

var (
	// namespace where the project is deployed in
	operatorNamespace = "openbao-operator-system"

	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if these components are already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false
	// Note: Gateway API CRDs are NOT installed by default in BeforeSuite.
	// Individual tests that require Gateway API should use InstallGatewayAPI() and UninstallGatewayAPI()
	// helper functions to manage Gateway API CRDs on a per-test basis.

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/openbao-operator:v0.0.1"

	// configInitImage is the image used by OpenBao pods as init container.
	// It must be resolvable inside the kind cluster; in E2E we build it locally
	// and load it into kind.
	configInitImage = "openbao-config-init:dev"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting openbao-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("building the config-init image")
	cmd = exec.Command("make", "docker-build-init", fmt.Sprintf("IMG=%s", configInitImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the config-init image")

	By("loading the config-init image on Kind")
	err = utils.LoadImageToKindClusterWithName(configInitImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the config-init image into Kind")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}

	// Gateway API CRDs are NOT installed by default in BeforeSuite.
	// Individual tests that require Gateway API should install/uninstall them
	// using the helper functions (InstallGatewayAPI/UninstallGatewayAPI).
	// This allows tests like "reports Degraded when Gateway API CRDs are missing"
	// to actually verify the degraded condition.

	By("creating operator namespace")
	cmd = exec.Command("kubectl", "create", "ns", operatorNamespace)
	_, err = utils.Run(cmd)
	if err != nil {
		// Namespace may already exist if tests are re-run without cleanup.
		Expect(err.Error()).To(ContainSubstring("AlreadyExists"))
	}

	By("labeling the operator namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", operatorNamespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label operator namespace with restricted policy")

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying the controller-manager and provisioner")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy the operator")
})

var _ = AfterSuite(func() {
	By("cleaning up the curl pod for metrics")
	cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", operatorNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("undeploying the operator")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, "make", "undeploy", "ignore-not-found=true", "wait=false")
	_, _ = utils.Run(cmd)

	// Clean up infra-bao (shared across tests) from operator namespace
	By("cleaning up infra-bao resources")
	_ = exec.Command("kubectl", "delete", "pod", "infra-bao", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "svc", "infra-bao", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "secret", "infra-bao-tls-server", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "secret", "infra-bao-tls-ca", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "secret", "infra-bao-unseal-key", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "secret", "infra-bao-root-token", "-n", operatorNamespace, "--ignore-not-found").Run()
	_ = exec.Command("kubectl", "delete", "configmap", "infra-bao-config", "-n", operatorNamespace, "--ignore-not-found").Run()

	By("uninstalling CRDs")
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, "make", "uninstall")
	_, _ = utils.Run(cmd)

	By("removing operator namespace")
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd = exec.CommandContext(ctx, "kubectl", "delete", "ns", operatorNamespace, "--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)

	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}

	// Gateway API CRDs are managed per-test, not in AfterSuite.
	// Individual tests that install Gateway API should clean up using UninstallGatewayAPI.
})

// InstallGatewayAPI installs the Gateway API CRDs (standard and experimental).
// This is exported so individual tests can install Gateway API when needed.
// Returns an error if installation fails.
func InstallGatewayAPI() error {
	return installGatewayAPI()
}

// installGatewayAPI installs the Gateway API CRDs (standard and experimental).
func installGatewayAPI() error {
	const (
		gatewayAPIStandardURL     = "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml"
		gatewayAPIExperimentalURL = "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml"
	)

	// Install standard CRDs first
	cmd := exec.Command("kubectl", "apply", "-f", gatewayAPIStandardURL)
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to install Gateway API standard CRDs: %w", err)
	}

	// Install experimental CRDs using 'create' instead of 'apply' because the metadata
	// annotations are too long for 'apply' to handle (exceeds Kubernetes annotation size limits).
	cmd = exec.Command("kubectl", "create", "-f", gatewayAPIExperimentalURL)
	output, err := utils.Run(cmd)
	if err != nil {
		// If CRDs already exist, that's okay - we can continue
		// kubectl create returns an error with "AlreadyExists" in the output when resources exist
		if !strings.Contains(output, "AlreadyExists") && !strings.Contains(err.Error(), "AlreadyExists") {
			return fmt.Errorf("failed to install Gateway API experimental CRDs: %w", err)
		}
		// If it's an AlreadyExists error, that's fine - CRDs are already installed
	}

	// Wait for Gateway API CRDs to be established
	cmd = exec.Command("kubectl", "wait", "--for", "condition=Established",
		"crd/gateways.gateway.networking.k8s.io",
		"crd/httproutes.gateway.networking.k8s.io",
		"crd/tlsroutes.gateway.networking.k8s.io",
		"--timeout", "5m")
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to wait for Gateway API CRDs: %w", err)
	}

	return nil
}

// isGatewayAPICRDsInstalled checks if Gateway API CRDs are installed
// by verifying the existence of key CRDs.
func isGatewayAPICRDsInstalled() bool {
	gatewayAPICRDs := []string{
		"gateways.gateway.networking.k8s.io",
		"httproutes.gateway.networking.k8s.io",
		"tlsroutes.gateway.networking.k8s.io",
	}

	cmd := exec.Command("kubectl", "get", "crds")
	output, err := utils.Run(cmd)
	if err != nil {
		return false
	}

	crdList := utils.GetNonEmptyLines(output)
	for _, crd := range gatewayAPICRDs {
		found := false
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// UninstallGatewayAPI removes the Gateway API CRDs from the cluster.
// This is exported so individual tests can clean up Gateway API after use.
// Returns an error if uninstallation fails.
func UninstallGatewayAPI() error {
	const (
		gatewayAPIStandardURL     = "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/standard-install.yaml"
		gatewayAPIExperimentalURL = "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.4.1/experimental-install.yaml"
	)

	// Delete experimental CRDs first (they may depend on standard CRDs)
	cmd := exec.Command("kubectl", "delete", "-f", gatewayAPIExperimentalURL, "--ignore-not-found")
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to uninstall Gateway API experimental CRDs: %w", err)
	}

	// Delete standard CRDs
	cmd = exec.Command("kubectl", "delete", "-f", gatewayAPIStandardURL, "--ignore-not-found")
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to uninstall Gateway API standard CRDs: %w", err)
	}

	// Wait for CRDs to be fully removed (optional, but helps ensure clean state)
	// We use a short timeout since we're using --ignore-not-found
	cmd = exec.Command("kubectl", "wait", "--for=delete",
		"crd/gateways.gateway.networking.k8s.io",
		"crd/httproutes.gateway.networking.k8s.io",
		"crd/tlsroutes.gateway.networking.k8s.io",
		"--timeout", "30s")
	_, _ = utils.Run(cmd) // Ignore errors - CRDs may already be deleted

	return nil
}
