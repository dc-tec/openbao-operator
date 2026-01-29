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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/test/utils"
)

const (
	defaultProjectImage         = "example.com/openbao-operator:0.0.1"
	defaultConfigInitImage      = "openbao-init:dev"
	defaultBackupExecutorImage  = "openbao-backup:dev"
	defaultUpgradeExecutorImage = "openbao-upgrade:dev"
)

type suiteBootstrap struct {
	Clusters                []string `json:"clusters"`
	Kubeconfigs             []string `json:"kubeconfigs"`
	CertManagerPreinstalled []bool   `json:"certManagerPreinstalled"`
	StorageClass            string   `json:"storageClass,omitempty"`
}

var (
	// namespace where the project is deployed in
	operatorNamespace = "openbao-operator-system"

	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if these components are already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// suiteBootstrapState stores cross-cluster bootstrap state for cleanup (node 1 only).
	suiteBootstrapState *suiteBootstrap
	// Note: Gateway API CRDs are NOT installed by default in BeforeSuite.
	// Individual tests that require Gateway API should use InstallGatewayAPI() and UninstallGatewayAPI()
	// helper functions to manage Gateway API CRDs on a per-test basis.

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = defaultProjectImage

	// configInitImage is the image used by OpenBao pods as init container.
	// It must be resolvable inside the kind cluster; in E2E we build it locally
	// and load it into kind.
	configInitImage = defaultConfigInitImage

	// backupExecutorImage is the image used by backup Jobs.
	// It must be resolvable inside the kind cluster; in E2E we build it locally
	// and load it into kind.
	backupExecutorImage = defaultBackupExecutorImage

	// upgradeExecutorImage is the image used by upgrade Jobs.
	// It must be resolvable inside the kind cluster; in E2E we build it locally
	// and load it into kind.
	upgradeExecutorImage = defaultUpgradeExecutorImage

	// skipCleanup controls whether to clean up resources after the suite finishes.
	// Set E2E_SKIP_CLEANUP=true environment variable to preserve the cluster state for debugging.
	skipCleanup = os.Getenv("E2E_SKIP_CLEANUP") == "true"

	// skipImageBuild controls whether to skip building local images during suite setup.
	// This is useful for release workflows that build images once (externally) and only need
	// to load pre-built images into kind.
	skipImageBuild = os.Getenv("E2E_SKIP_IMAGE_BUILD") == "true"

	// useExistingCluster runs the e2e suite against an already-running cluster (e.g. OpenShift Local / CRC).
	// When enabled, the suite:
	// - does NOT create kind clusters
	// - does NOT build/load local images into kind
	// - uses the current kubectl context (or KUBECONFIG) to install CRDs and deploy the operator
	useExistingCluster = os.Getenv("E2E_USE_EXISTING_CLUSTER") == "true"

	// existingClusterName is used only when E2E_USE_EXISTING_CLUSTER=true.
	existingClusterName = envOrDefault("E2E_CLUSTER_NAME", "existing")

	// existingClusterFullCleanup controls whether we uninstall CRDs and cert-manager in existing-cluster mode.
	// Default is false to reduce blast radius on shared clusters.
	existingClusterFullCleanup = os.Getenv("E2E_EXISTING_CLUSTER_FULL_CLEANUP") == "true"
)

func kindClusterName(base string, index int) string {
	if index < 1 {
		index = 1
	}
	return fmt.Sprintf("%s-%d", base, index)
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

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(false)))
	projectImage = envOrDefault("E2E_OPERATOR_IMAGE", projectImage)
	configInitImage = envOrDefault("E2E_CONFIG_INIT_IMAGE", configInitImage)
	backupExecutorImage = envOrDefault("E2E_BACKUP_EXECUTOR_IMAGE", backupExecutorImage)
	upgradeExecutorImage = envOrDefault("E2E_UPGRADE_EXECUTOR_IMAGE", upgradeExecutorImage)
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting openbao-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

func envOrDefault(key, defaultValue string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return defaultValue
	}
	return value
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

var _ = SynchronizedBeforeSuite(func() []byte {
	// THIS BLOCK RUNS ONCE (on node 1)
	var (
		cmd *exec.Cmd
		err error
	)

	suiteConfig, _ := GinkgoConfiguration()
	parallelTotal := suiteConfig.ParallelTotal
	if parallelTotal < 1 {
		parallelTotal = 1
	}

	if useExistingCluster {
		kubeconfigPath := strings.TrimSpace(os.Getenv("KUBECONFIG"))
		if kubeconfigPath == "" {
			By("KUBECONFIG not set; exporting current kubectl context to a temporary kubeconfig")
			cmd = exec.Command("kubectl", "config", "view", "--raw") // #nosec G204 -- test harness command
			out, err := utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to export kubeconfig from current kubectl context")
			kubeconfigPath = filepath.Join(os.TempDir(), "openbao-operator-e2e-existing.kubeconfig")
			ExpectWithOffset(1, os.WriteFile(kubeconfigPath, []byte(out), 0o600)).To(Succeed(), "Failed to write exported kubeconfig")
		}

		if strings.TrimSpace(projectImage) == "" || projectImage == defaultProjectImage {
			Fail("E2E_USE_EXISTING_CLUSTER requires E2E_OPERATOR_IMAGE to be set to a pullable image reference")
		}

		bootstrap := suiteBootstrap{
			Clusters:                []string{existingClusterName},
			Kubeconfigs:             []string{kubeconfigPath},
			CertManagerPreinstalled: []bool{},
		}

		withEnv("KUBECONFIG", kubeconfigPath, func() {
			certManagerPreinstalled := false
			if !skipCertManagerInstall {
				By("checking if cert manager is installed already")
				certManagerPreinstalled = utils.IsCertManagerCRDsInstalled()
				if !certManagerPreinstalled {
					_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
					ExpectWithOffset(1, utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
				}
			}
			bootstrap.CertManagerPreinstalled = append(bootstrap.CertManagerPreinstalled, certManagerPreinstalled)

			By("creating operator namespace")
			cmd = exec.Command("kubectl", "create", "ns", operatorNamespace) // #nosec G204 -- test harness command
			_, err = utils.Run(cmd)
			if err != nil {
				Expect(err.Error()).To(ContainSubstring("AlreadyExists"))
			}

			By("labeling the operator namespace to enforce the restricted security policy (best-effort)")
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", operatorNamespace,
				"pod-security.kubernetes.io/enforce=restricted") // #nosec G204 -- test harness command
			_, _ = utils.Run(cmd)

			By("installing CRDs")
			cmd = exec.Command("make", "install")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs")
			ExpectWithOffset(1, waitForCRDsEstablished(2*time.Minute)).To(Succeed(), "CRDs did not become Established in time")

			By("deploying the controller-manager and provisioner")
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy the operator")

			By("waiting for operator deployments to become Available")
			ExpectWithOffset(1, waitForDeploymentsAvailable(operatorNamespace, 5*time.Minute)).
				To(Succeed(), "Operator deployments did not become Available in time")
		})

		suiteBootstrapState = &bootstrap

		data, err := json.Marshal(bootstrap)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to marshal suite bootstrap state")
		return data
	}

	baseCluster := strings.TrimSpace(os.Getenv("KIND_CLUSTER"))
	if baseCluster == "" {
		baseCluster = "kind"
	}

	kindBinary := strings.TrimSpace(os.Getenv("KIND"))
	if kindBinary == "" {
		kindBinary = "kind"
	}

	if skipImageBuild {
		By("skipping build of the manager(Operator) image")
	} else {
		By("building the manager(Operator) image")
		cmd = exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")
	}

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Images are loaded into Kind clusters below.

	if skipImageBuild {
		By("skipping build of the config-init image")
	} else {
		By("building the config-init image")
		cmd = exec.Command("make", "docker-build-init", fmt.Sprintf("IMG=%s", configInitImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the config-init image")
	}

	// Images are loaded per parallel cluster below.

	if skipImageBuild {
		By("skipping build of the backup executor image")
	} else {
		By("building the backup executor image")
		cmd = exec.Command("make", "docker-build-backup", fmt.Sprintf("IMG=%s", backupExecutorImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the backup executor image")
	}

	// Images are loaded per parallel cluster below.

	if skipImageBuild {
		By("skipping build of the upgrade executor image")
	} else {
		By("building the upgrade executor image")
		cmd = exec.Command("make", "docker-build-upgrade", fmt.Sprintf("IMG=%s", upgradeExecutorImage))
		_, err = utils.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the upgrade executor image")
	}

	// Single cluster setup for all parallel nodes
	clusterName := baseCluster
	kubeconfigPath := filepath.Join(os.TempDir(), fmt.Sprintf("openbao-operator-e2e-%s.kubeconfig", clusterName))

	bootstrap := suiteBootstrap{
		Clusters:                []string{clusterName},
		Kubeconfigs:             []string{kubeconfigPath},
		CertManagerPreinstalled: []bool{},
		StorageClass:            "",
	}

	By(fmt.Sprintf("exporting kubeconfig for Kind cluster %q", clusterName))
	cmd = exec.Command(kindBinary, "export", "kubeconfig", "--name", clusterName, "--kubeconfig", kubeconfigPath)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to export kubeconfig for Kind cluster %q", clusterName))

	withEnv("KUBECONFIG", kubeconfigPath, func() {
		withEnv("KIND_CLUSTER", clusterName, func() {
			By(fmt.Sprintf("waiting for CoreDNS to become Available (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForCoreDNSAvailable(2*time.Minute)).To(Succeed(), "CoreDNS did not become Available in time")

			By(fmt.Sprintf("installing CSI hostpath driver for storage expansion tests (cluster=%s)", clusterName))
			ExpectWithOffset(1, utils.InstallCSIHostPathDriver()).To(Succeed(), "Failed to install CSI hostpath driver")
			By("installing the expandable E2E StorageClass (openbao-e2e-hostpath)")
			cmd = exec.Command("kubectl", "apply", "-f", "test/manifests/csi-hostpath/v1.9.0/storageclass-openbao-e2e-hostpath.yaml") // #nosec G204 -- test harness
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to apply E2E StorageClass")
			bootstrap.StorageClass = "openbao-e2e-hostpath"
			ExpectWithOffset(1, os.Setenv("E2E_STORAGE_CLASS", bootstrap.StorageClass)).To(Succeed())
			By("setting openbao-e2e-hostpath as the default StorageClass (best effort)")
			// Prefer the hostpath CSI driver for all PVCs in Kind E2E to cover volume expansion.
			cmd = exec.Command("kubectl", "patch", "storageclass", bootstrap.StorageClass, "--type=merge",
				`-p={"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true","storageclass.beta.kubernetes.io/is-default-class":"true"}}}`) // #nosec G204 -- test harness
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to set default StorageClass for E2E")
			// Kind typically ships a "standard" default StorageClass. Best-effort flip it off to avoid ambiguity.
			cmd = exec.Command("kubectl", "patch", "storageclass", "standard", "--type=merge",
				`-p={"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false","storageclass.beta.kubernetes.io/is-default-class":"false"}}}`) // #nosec G204 -- test harness
			_, _ = utils.Run(cmd)

			By(fmt.Sprintf("loading the manager(Operator) image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(projectImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the manager(Operator) image into Kind (cluster=%s)", clusterName))

			By(fmt.Sprintf("loading the config-init image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(configInitImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the config-init image into Kind (cluster=%s)", clusterName))

			By(fmt.Sprintf("loading the backup executor image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(backupExecutorImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the backup executor image into Kind (cluster=%s)", clusterName))

			By(fmt.Sprintf("loading the upgrade executor image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(upgradeExecutorImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the upgrade executor image into Kind (cluster=%s)", clusterName))

			By(fmt.Sprintf("pre-loading OpenBao images on Kind to reduce flakiness (cluster=%s)", clusterName))
			openBaoImages := []string{
				openBaoImage,
				fmt.Sprintf("openbao/openbao:%s", defaultUpgradeFromVersion),
				fmt.Sprintf("openbao/openbao:%s", defaultUpgradeToVersion),
			}
			seen := make(map[string]struct{}, len(openBaoImages))
			for _, img := range openBaoImages {
				img = strings.TrimSpace(img)
				if img == "" {
					continue
				}
				if _, ok := seen[img]; ok {
					continue
				}
				seen[img] = struct{}{}
				_, _ = fmt.Fprintf(GinkgoWriter, "Loading OpenBao image %q into kind (cluster=%s)\n", img, clusterName)
				// Ensure the image exists locally before kind load (fresh CI runners won't have it).
				if _, err := utils.Run(exec.Command("docker", "image", "inspect", img)); err != nil { // #nosec G204 -- test harness command
					_, _ = fmt.Fprintf(GinkgoWriter, "Pulling OpenBao image %q...\n", img)
					_, err = utils.Run(exec.Command("docker", "pull", img)) // #nosec G204 -- test harness command
					ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to pull OpenBao image %q (cluster=%s)", img, clusterName))
				}
				err = utils.LoadImageToKindClusterWithName(img)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load OpenBao image %q into Kind (cluster=%s)", img, clusterName))
			}

			certManagerPreinstalled := false
			if !skipCertManagerInstall {
				By(fmt.Sprintf("checking if cert manager is installed already (cluster=%s)", clusterName))
				certManagerPreinstalled = utils.IsCertManagerCRDsInstalled()
				if !certManagerPreinstalled {
					_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager (cluster=%s)...\n", clusterName)
					ExpectWithOffset(1, utils.InstallCertManager()).To(Succeed(), fmt.Sprintf("Failed to install CertManager (cluster=%s)", clusterName))
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation (cluster=%s)...\n", clusterName)
				}
			}
			bootstrap.CertManagerPreinstalled = append(bootstrap.CertManagerPreinstalled, certManagerPreinstalled)

			By(fmt.Sprintf("creating operator namespace (cluster=%s)", clusterName))
			cmd = exec.Command("kubectl", "create", "ns", operatorNamespace)
			_, err = utils.Run(cmd)
			if err != nil {
				// Namespace may already exist if tests are re-run without cleanup.
				Expect(err.Error()).To(ContainSubstring("AlreadyExists"))
			}

			By(fmt.Sprintf("labeling the operator namespace to enforce the restricted security policy (cluster=%s)", clusterName))
			cmd = exec.Command("kubectl", "label", "--overwrite", "ns", operatorNamespace,
				"pod-security.kubernetes.io/enforce=restricted")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to label operator namespace with restricted policy (cluster=%s)", clusterName))

			By(fmt.Sprintf("installing CRDs (cluster=%s)", clusterName))
			cmd = exec.Command("make", "install")
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to install CRDs (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForCRDsEstablished(2*time.Minute)).To(Succeed(), fmt.Sprintf("CRDs did not become Established in time (cluster=%s)", clusterName))

			By(fmt.Sprintf("deploying the controller-manager and provisioner (cluster=%s)", clusterName))
			cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
			_, err = utils.Run(cmd)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to deploy the operator (cluster=%s)", clusterName))

			By(fmt.Sprintf("waiting for operator deployments to become Available (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForDeploymentsAvailable(operatorNamespace, 5*time.Minute)).
				To(Succeed(), fmt.Sprintf("Operator deployments did not become Available in time (cluster=%s)", clusterName))
		})
	})

	suiteBootstrapState = &bootstrap

	data, err := json.Marshal(bootstrap)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to marshal suite bootstrap state")
	return data
}, func(data []byte) {
	// THIS BLOCK RUNS ON ALL NODES (after node 1 finishes)
	bootstrap := &suiteBootstrap{}
	ExpectWithOffset(1, json.Unmarshal(data, bootstrap)).To(Succeed(), "Failed to unmarshal suite bootstrap state")
	ExpectWithOffset(1, len(bootstrap.Clusters)).To(Equal(1))
	ExpectWithOffset(1, len(bootstrap.Kubeconfigs)).To(Equal(1))

	// All processes share the same cluster
	clusterName := bootstrap.Clusters[0]
	kubeconfigPath := bootstrap.Kubeconfigs[0]

	ExpectWithOffset(1, os.Setenv("KIND_CLUSTER", clusterName)).To(Succeed())
	ExpectWithOffset(1, os.Setenv("KUBECONFIG", kubeconfigPath)).To(Succeed())
	if strings.TrimSpace(bootstrap.StorageClass) != "" {
		ExpectWithOffset(1, os.Setenv("E2E_STORAGE_CLASS", bootstrap.StorageClass)).To(Succeed())
	}

	proc := GinkgoParallelProcess()
	_, _ = fmt.Fprintf(GinkgoWriter, "E2E parallel process=%d shared_cluster=%s kubeconfig=%s\n", proc, clusterName, kubeconfigPath)
})

var _ = SynchronizedAfterSuite(func() {
	// THIS BLOCK RUNS ON ALL NODES (after all specs complete)
	// Per-node cleanup can go here if needed.
	// Currently, all cleanup is shared and goes in the node 1 block below.
}, func() {
	// THIS BLOCK RUNS ONCE (on node 1, after all nodes complete the first block)
	// This ensures cleanup only happens once after all tests finish.

	if skipCleanup {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping cleanup as E2E_SKIP_CLEANUP is set to true\n")
		return
	}

	ExpectWithOffset(1, suiteBootstrapState).NotTo(BeNil(), "suite bootstrap state not initialized")

	// Single cluster cleanup
	if len(suiteBootstrapState.Clusters) > 0 {
		clusterName := suiteBootstrapState.Clusters[0]
		kubeconfigPath := suiteBootstrapState.Kubeconfigs[0]

		withEnv("KIND_CLUSTER", clusterName, func() {
			withEnv("KUBECONFIG", kubeconfigPath, func() {
				By(fmt.Sprintf("cleaning up the curl pod for metrics (cluster=%s)", clusterName))
				cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", operatorNamespace, "--ignore-not-found")
				_, _ = utils.Run(cmd)

				By(fmt.Sprintf("cleaning up OpenBao custom resources before undeploying (cluster=%s)", clusterName))
				// Use a shorter timeout for cleanup - if resources are stuck, we'll remove finalizers quickly
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := cleanupOpenBaoCustomResources(ctx); err != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: cleanupOpenBaoCustomResources failed (cluster=%s; will continue with undeploy): %v\n", clusterName, err)
				}
				cancel()

				By(fmt.Sprintf("undeploying the operator (cluster=%s)", clusterName))
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
				cmd = exec.CommandContext(ctx, "make", "undeploy", "ignore-not-found=true", "wait=false")
				_, _ = utils.Run(cmd)
				cancel()

				if !useExistingCluster || existingClusterFullCleanup {
					By(fmt.Sprintf("uninstalling CRDs (cluster=%s)", clusterName))
					ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
					cmd = exec.CommandContext(ctx, "make", "uninstall", "ignore-not-found=true", "wait=false")
					_, _ = utils.Run(cmd)
					cancel()
				} else {
					_, _ = fmt.Fprintf(GinkgoWriter, "E2E_USE_EXISTING_CLUSTER: skipping CRD uninstall (set E2E_EXISTING_CLUSTER_FULL_CLEANUP=true to enable)\n")
				}

				By(fmt.Sprintf("removing operator namespace (cluster=%s)", clusterName))
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
				cmd = exec.CommandContext(ctx, "kubectl", "delete", "ns", operatorNamespace, "--ignore-not-found", "--wait=false")
				_, _ = utils.Run(cmd)
				cancel()

				// Teardown CertManager after the suite if not skipped and if it was not already installed.
				// In existing-cluster mode, keep cert-manager by default.
				if !skipCertManagerInstall && !suiteBootstrapState.CertManagerPreinstalled[0] {
					if !useExistingCluster || existingClusterFullCleanup {
						_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager (cluster=%s)...\n", clusterName)
						utils.UninstallCertManager()
					} else {
						_, _ = fmt.Fprintf(GinkgoWriter, "E2E_USE_EXISTING_CLUSTER: skipping CertManager uninstall (set E2E_EXISTING_CLUSTER_FULL_CLEANUP=true to enable)\n")
					}
				}
			})
		})
	}

	// Gateway API CRDs are managed per-test, not in AfterSuite.
	// Individual tests that install Gateway API should clean up using UninstallGatewayAPI.
})

func cleanupOpenBaoCustomResources(ctx context.Context) error {
	cfg, scheme, err := buildSuiteClientConfig()
	if err != nil {
		return err
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create cleanup client: %w", err)
	}

	if err := deleteAllOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}

	// Wait briefly for resources to be deleted (most should delete quickly)
	if err := waitForOpenBaoCustomResourcesDeleted(ctx, c, 10*time.Second, 1*time.Second); err == nil {
		return nil
	}

	// If resources are stuck (usually finalizers), remove finalizers and try again.
	// This is faster than waiting for the full timeout
	if err := removeFinalizersFromOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}
	if err := deleteAllOpenBaoCustomResources(ctx, c); err != nil {
		return err
	}

	// Wait again with remaining context time (but cap at 10 seconds)
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		remainingTimeout := time.Until(deadline)
		if remainingTimeout > 10*time.Second {
			remainingTimeout = 10 * time.Second
		}
		if remainingTimeout > 0 {
			if err := waitForOpenBaoCustomResourcesDeleted(ctx, c, remainingTimeout, 1*time.Second); err != nil {
				// Log but don't fail - undeploy will clean up remaining resources
				_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Some resources may still exist after cleanup: %v\n", err)
			}
		}
	}

	return nil
}

func buildSuiteClientConfig() (*rest.Config, *runtime.Scheme, error) {
	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kube config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		return nil, nil, fmt.Errorf("failed to add openbao scheme: %w", err)
	}

	return cfg, scheme, nil
}

func deleteAllOpenBaoCustomResources(ctx context.Context, c client.Client) error {
	var clusters openbaov1alpha1.OpenBaoClusterList
	if err := c.List(ctx, &clusters); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusters: %w", err)
	}
	for i := range clusters.Items {
		cluster := clusters.Items[i]
		if err := c.Delete(ctx, &cluster); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	}

	var tenants openbaov1alpha1.OpenBaoTenantList
	if err := c.List(ctx, &tenants); err != nil {
		return fmt.Errorf("failed to list OpenBaoTenants: %w", err)
	}
	for i := range tenants.Items {
		tenant := tenants.Items[i]
		if err := c.Delete(ctx, &tenant); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete OpenBaoTenant %s/%s: %w", tenant.Namespace, tenant.Name, err)
		}
	}

	var namespaces corev1.NamespaceList
	if err := c.List(ctx, &namespaces); err != nil {
		return fmt.Errorf("failed to list namespaces: %w", err)
	}
	for i := range namespaces.Items {
		ns := namespaces.Items[i]
		if !strings.HasPrefix(ns.Name, "e2e-") {
			continue
		}
		if err := c.Delete(ctx, &ns); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete namespace %q: %w", ns.Name, err)
		}
	}

	return nil
}

func waitForOpenBaoCustomResourcesDeleted(ctx context.Context, c client.Client, timeout time.Duration, pollInterval time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if pollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		var clusters openbaov1alpha1.OpenBaoClusterList
		if err := c.List(ctx, &clusters); err != nil {
			return fmt.Errorf("failed to list OpenBaoClusters: %w", err)
		}
		var tenants openbaov1alpha1.OpenBaoTenantList
		if err := c.List(ctx, &tenants); err != nil {
			return fmt.Errorf("failed to list OpenBaoTenants: %w", err)
		}

		if len(clusters.Items) == 0 && len(tenants.Items) == 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled while waiting for OpenBao custom resources to be deleted: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for OpenBao custom resources to be deleted (clusters=%d tenants=%d)", len(clusters.Items), len(tenants.Items))
		case <-ticker.C:
		}
	}
}

func removeFinalizersFromOpenBaoCustomResources(ctx context.Context, c client.Client) error {
	var clusters openbaov1alpha1.OpenBaoClusterList
	if err := c.List(ctx, &clusters); err != nil {
		return fmt.Errorf("failed to list OpenBaoClusters for finalizer removal: %w", err)
	}
	for i := range clusters.Items {
		cluster := clusters.Items[i]
		if len(cluster.Finalizers) == 0 {
			continue
		}
		original := cluster.DeepCopy()
		cluster.Finalizers = nil
		if err := c.Patch(ctx, &cluster, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
	}

	var tenants openbaov1alpha1.OpenBaoTenantList
	if err := c.List(ctx, &tenants); err != nil {
		return fmt.Errorf("failed to list OpenBaoTenants for finalizer removal: %w", err)
	}
	for i := range tenants.Items {
		tenant := tenants.Items[i]
		if len(tenant.Finalizers) == 0 {
			continue
		}
		original := tenant.DeepCopy()
		tenant.Finalizers = nil
		if err := c.Patch(ctx, &tenant, client.MergeFrom(original)); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from OpenBaoTenant %s/%s: %w", tenant.Namespace, tenant.Name, err)
		}
	}

	return nil
}
