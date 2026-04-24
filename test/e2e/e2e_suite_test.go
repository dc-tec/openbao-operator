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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/dc-tec/openbao-operator/test/utils"
)

const (
	defaultProjectImage         = "example.com/openbao-operator:0.0.1"
	defaultConfigInitImage      = "openbao-init:dev"
	defaultBackupExecutorImage  = "openbao-backup:dev"
	defaultUpgradeExecutorImage = "openbao-upgrade:dev"
	defaultHardenedConfigInit   = ""

	operatorInstallModeKustomize = "kustomize"
	operatorInstallModeHelm      = "helm"

	e2eMetricsReaderBindingName = "openbao-operator-metrics-binding"
)

type suiteBootstrap struct {
	Clusters                []string `json:"clusters"`
	Kubeconfigs             []string `json:"kubeconfigs"`
	CertManagerPreinstalled []bool   `json:"certManagerPreinstalled"`
	StorageClass            string   `json:"storageClass,omitempty"`
	OperatorInstallMode     string   `json:"operatorInstallMode,omitempty"`
}

var (
	// namespace where the project is deployed in
	operatorNamespace = "openbao-operator-system"

	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if these components are already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == e2eStringTrue
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

	// hardenedConfigInitImage optionally overrides the init image for Hardened profile
	// scenarios. This supports running Hardened tests with signed helper images while
	// keeping local dev tests on unsigned local helper images.
	hardenedConfigInitImage = defaultHardenedConfigInit

	// hardenedUpgradeExecutorImage optionally overrides the upgrade executor image for
	// hardened-only scenarios. This allows full-suite runs to use a signed executor for
	// hardened coverage while other upgrade scenarios keep using the candidate image.
	hardenedUpgradeExecutorImage string

	// skipCleanup controls whether to clean up resources after the suite finishes.
	// Set E2E_SKIP_CLEANUP=true environment variable to preserve the cluster state for debugging.
	skipCleanup = os.Getenv("E2E_SKIP_CLEANUP") == e2eStringTrue

	// skipImageBuild controls whether to skip building local images during suite setup.
	// This is useful for release workflows that build images once (externally) and only need
	// to load pre-built images into kind.
	skipImageBuild = os.Getenv("E2E_SKIP_IMAGE_BUILD") == e2eStringTrue

	// loadBackupExecutorImage controls whether the backup executor image is loaded into Kind.
	// Fast shards that never create backup jobs can disable this to reduce bootstrap time.
	loadBackupExecutorImage = envBoolDefaultTrue("E2E_LOAD_BACKUP_EXECUTOR_IMAGE")

	// loadUpgradeExecutorImage controls whether the upgrade executor image is loaded into Kind.
	// Fast shards that never create upgrade jobs can disable this to reduce bootstrap time.
	loadUpgradeExecutorImage = envBoolDefaultTrue("E2E_LOAD_UPGRADE_EXECUTOR_IMAGE")

	// preloadUpgradeImages controls whether upgrade-version OpenBao images are preloaded into Kind.
	// This should stay enabled for upgrade-focused shards and can be disabled elsewhere.
	preloadUpgradeImages = envBoolDefaultTrue("E2E_PRELOAD_UPGRADE_IMAGES")

	// preloadHardenedAssets controls whether hardened-only helper images are preloaded into Kind.
	// This should stay enabled for the hardened signed shard and can be disabled elsewhere.
	preloadHardenedAssets = envBoolDefaultTrue("E2E_PRELOAD_HARDENED_ASSETS")

	// installCSIHostPath controls whether the Kind suite installs the CSI hostpath driver.
	// Only storage-expansion coverage needs it; other lanes should keep it disabled to avoid
	// unrelated bootstrap flakes and startup cost.
	installCSIHostPath = envBoolDefaultTrue("E2E_INSTALL_CSI_HOSTPATH")

	// useExistingCluster runs the e2e suite against an already-running cluster (e.g. OpenShift Local / CRC).
	// When enabled, the suite:
	// - does NOT create kind clusters
	// - does NOT build/load local images into kind
	// - uses the current kubectl context (or KUBECONFIG) to install CRDs and deploy the operator
	useExistingCluster = os.Getenv("E2E_USE_EXISTING_CLUSTER") == e2eStringTrue

	// existingClusterName is used only when E2E_USE_EXISTING_CLUSTER=true.
	existingClusterName = envOrDefault("E2E_CLUSTER_NAME", "existing")

	// existingClusterFullCleanup controls whether we uninstall CRDs and cert-manager in existing-cluster mode.
	// Default is false to reduce blast radius on shared clusters.
	existingClusterFullCleanup = os.Getenv("E2E_EXISTING_CLUSTER_FULL_CLEANUP") == e2eStringTrue
)

func patchOperatorKubeAPITokenAudience(ctx context.Context, namespace string) error {
	audience := strings.TrimSpace(os.Getenv("OPENBAO_KUBE_API_AUDIENCE"))
	if audience == "" {
		return nil
	}

	cfg, err := ctrlconfig.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kube config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add apps scheme: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	patchDeployment := func(name string) error {
		deploy := &appsv1.Deployment{}
		if err := c.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, deploy); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		orig := deploy.DeepCopy()
		updated := false

		for i := range deploy.Spec.Template.Spec.Volumes {
			vol := &deploy.Spec.Template.Spec.Volumes[i]
			if vol.Name != "kube-api-access" || vol.Projected == nil {
				continue
			}
			for j := range vol.Projected.Sources {
				src := &vol.Projected.Sources[j]
				if src.ServiceAccountToken == nil {
					continue
				}
				if src.ServiceAccountToken.Audience != audience {
					src.ServiceAccountToken.Audience = audience
					updated = true
				}
				break
			}
			break
		}

		if !updated {
			return nil
		}

		if err := c.Patch(ctx, deploy, client.MergeFrom(orig)); err != nil {
			return err
		}

		return nil
	}

	if err := patchDeployment("openbao-operator-controller"); err != nil {
		return fmt.Errorf("patch controller audience: %w", err)
	}
	if err := patchDeployment("openbao-operator-provisioner"); err != nil {
		return fmt.Errorf("patch provisioner audience: %w", err)
	}

	return nil
}

func applyControllerEnvOverrides(namespace string) error {
	overrides := make([]string, 0, 2)
	for _, key := range []string{"OPENBAO_REQUEUE_STANDARD", "OPENBAO_JWT_AUTH_STRATEGY"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			overrides = append(overrides, fmt.Sprintf("%s=%s", key, value))
		}
	}
	if len(overrides) == 0 {
		return nil
	}

	cmd := exec.Command("kubectl", "get", "deployment",
		"-l", "app.kubernetes.io/name=openbao-operator,app.kubernetes.io/component=controller",
		"-n", namespace,
		"-o", "jsonpath={.items[0].metadata.name}") // #nosec G204 -- test harness command
	out, err := utils.Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to find controller deployment by label: %w", err)
	}
	controllerDeploymentName := strings.TrimSpace(out)
	if controllerDeploymentName == "" {
		return fmt.Errorf("controller deployment not found in namespace %q", namespace)
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "Injecting controller env overrides: %s\n", strings.Join(overrides, ", "))
	args := append([]string{"set", "env", fmt.Sprintf("deployment/%s", controllerDeploymentName), "-n", namespace}, overrides...)
	cmd = exec.Command("kubectl", args...) // #nosec G204 -- test harness command
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to set controller env overrides: %w", err)
	}

	cmd = exec.Command("kubectl", "rollout", "status", fmt.Sprintf("deployment/%s", controllerDeploymentName),
		"-n", namespace, "--timeout=2m") // #nosec G204 -- test harness command
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("failed to wait for controller rollout: %w", err)
	}
	return nil
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
	hardenedConfigInitImage = envOrDefault("E2E_HARDENED_CONFIG_INIT_IMAGE", hardenedConfigInitImage)
	hardenedUpgradeExecutorImage = envOrDefault("E2E_HARDENED_UPGRADE_EXECUTOR_IMAGE", hardenedUpgradeExecutorImage)
	if hardenedConfigInitImage == "" {
		hardenedConfigInitImage = configInitImage
	}
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting openbao-operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

func deleteE2EMetricsReaderBinding() error {
	cmd := exec.Command("kubectl", "delete", "clusterrolebinding", e2eMetricsReaderBindingName, "--ignore-not-found") // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err
}

var _ = SynchronizedBeforeSuite(func() []byte {
	// THIS BLOCK RUNS ONCE (on node 1)
	var (
		cmd *exec.Cmd
		err error
	)

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
			OperatorInstallMode:     operatorInstallModeForE2E(),
		}

		withEnv("KUBECONFIG", kubeconfigPath, func() {
			By("resolving the Kubernetes API service CIDR for E2E claim networking")
			resolvedCIDR, err := resolveE2EAPIServerCIDR(context.Background())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to resolve Kubernetes API service CIDR")
			apiServerCIDR = resolvedCIDR
			_, _ = fmt.Fprintf(GinkgoWriter, "Using E2E API server CIDR %q\n", apiServerCIDR)
			if serviceClaimsE2EEnabled() {
				resolvedEndpointIPs, err := resolveE2EAPIServerEndpointIPs(context.Background())
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to resolve Kubernetes API endpoint IPs")
				apiServerEndpointIPs = resolvedEndpointIPs
				_, _ = fmt.Fprintf(GinkgoWriter, "Using E2E API server endpoint IPs %q\n", apiServerEndpointIPs)
			}

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

			By("removing stale E2E metrics RBAC binding")
			ExpectWithOffset(1, deleteE2EMetricsReaderBinding()).To(Succeed(), "Failed to delete stale E2E metrics RBAC binding")

			By("waiting for the operator namespace to become reusable")
			ExpectWithOffset(1, waitForNamespaceDeletionIfTerminating(context.Background(), operatorNamespace, 60*time.Second, time.Second)).
				To(Succeed(), "Operator namespace is stuck terminating")

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

			By(fmt.Sprintf("installing the operator via %s (existing cluster)", bootstrap.OperatorInstallMode))
			ExpectWithOffset(1, installOperatorForE2E(context.Background(), operatorNamespace)).
				To(Succeed(), "Failed to install the operator")

			By("applying kube-apiserver token audience override when configured")
			ExpectWithOffset(1, patchOperatorKubeAPITokenAudience(context.Background(), operatorNamespace)).
				To(Succeed(), "Failed to patch kube-api token audience on operator Deployments")

			By("waiting for operator deployments to become Available")
			ExpectWithOffset(1, waitForDeploymentsAvailable(operatorNamespace, 5*time.Minute)).
				To(Succeed(), "Operator deployments did not become Available in time")
			if serviceClaimsE2EEnabled() {
				By("waiting for the controller admission webhook to publish ready endpoints")
				ExpectWithOffset(1, waitForServiceEndpoints(operatorNamespace, operatorControllerWebhookServiceName, 2*time.Minute)).
					To(Succeed(), "Controller admission webhook did not publish ready endpoints in time")
				By("waiting for claim admission dry-run requests to succeed")
				ExpectWithOffset(1, waitForClaimAdmissionWebhookReady(operatorNamespace, 2*time.Minute)).
					To(Succeed(), "Claim admission webhook did not become reachable in time")
			}

			By("applying controller env overrides when configured")
			ExpectWithOffset(1, applyControllerEnvOverrides(operatorNamespace)).
				To(Succeed(), "Failed to apply controller env overrides")
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
		OperatorInstallMode:     operatorInstallModeForE2E(),
	}

	By(fmt.Sprintf("exporting kubeconfig for Kind cluster %q", clusterName))
	cmd = exec.Command(kindBinary, "export", "kubeconfig", "--name", clusterName, "--kubeconfig", kubeconfigPath)
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to export kubeconfig for Kind cluster %q", clusterName))

	withEnv("KUBECONFIG", kubeconfigPath, func() {
		withEnv("KIND_CLUSTER", clusterName, func() {
			By(fmt.Sprintf("resolving the Kubernetes API service CIDR for E2E claim networking (cluster=%s)", clusterName))
			resolvedCIDR, err := resolveE2EAPIServerCIDR(context.Background())
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to resolve Kubernetes API service CIDR (cluster=%s)", clusterName))
			apiServerCIDR = resolvedCIDR
			_, _ = fmt.Fprintf(GinkgoWriter, "Using E2E API server CIDR %q (cluster=%s)\n", apiServerCIDR, clusterName)
			if serviceClaimsE2EEnabled() {
				resolvedEndpointIPs, err := resolveE2EAPIServerEndpointIPs(context.Background())
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to resolve Kubernetes API endpoint IPs (cluster=%s)", clusterName))
				apiServerEndpointIPs = resolvedEndpointIPs
				_, _ = fmt.Fprintf(GinkgoWriter, "Using E2E API server endpoint IPs %q (cluster=%s)\n", apiServerEndpointIPs, clusterName)
			}

			By(fmt.Sprintf("waiting for CoreDNS to become Available (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForCoreDNSAvailable(2*time.Minute)).To(Succeed(), "CoreDNS did not become Available in time")

			if installCSIHostPath {
				By(fmt.Sprintf("installing CSI hostpath driver for storage expansion tests (cluster=%s)", clusterName))
				ExpectWithOffset(1, utils.InstallCSIHostPathDriver()).To(Succeed(), "Failed to install CSI hostpath driver")
				By("installing the expandable E2E StorageClass (openbao-e2e-hostpath)")
				cmd = exec.Command("kubectl", "apply", "-f", "test/manifests/csi-hostpath/v1.9.0/storageclass-openbao-e2e-hostpath.yaml") // #nosec G204 -- test harness
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to apply E2E StorageClass")
				bootstrap.StorageClass = "openbao-e2e-hostpath"
				ExpectWithOffset(1, os.Setenv("E2E_STORAGE_CLASS", bootstrap.StorageClass)).To(Succeed())
				By("setting openbao-e2e-hostpath as the default StorageClass (best effort)")
				// Prefer the hostpath CSI driver for storage-expansion coverage.
				cmd = exec.Command("kubectl", "patch", "storageclass", bootstrap.StorageClass, "--type=merge",
					`-p={"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true","storageclass.beta.kubernetes.io/is-default-class":"true"}}}`) // #nosec G204 -- test harness
				_, err = utils.Run(cmd)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to set default StorageClass for E2E")
				// Kind typically ships a "standard" default StorageClass. Best-effort flip it off to avoid ambiguity.
				cmd = exec.Command("kubectl", "patch", "storageclass", "standard", "--type=merge",
					`-p={"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"false","storageclass.beta.kubernetes.io/is-default-class":"false"}}}`) // #nosec G204 -- test harness
				_, _ = utils.Run(cmd)
			} else {
				By(fmt.Sprintf("skipping CSI hostpath driver install (cluster=%s)", clusterName))
			}

			By(fmt.Sprintf("loading the manager(Operator) image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(projectImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the manager(Operator) image into Kind (cluster=%s)", clusterName))

			By(fmt.Sprintf("loading the config-init image on Kind (cluster=%s)", clusterName))
			err = utils.LoadImageToKindClusterWithName(configInitImage)
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the config-init image into Kind (cluster=%s)", clusterName))

			if loadBackupExecutorImage {
				By(fmt.Sprintf("loading the backup executor image on Kind (cluster=%s)", clusterName))
				err = utils.LoadImageToKindClusterWithName(backupExecutorImage)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the backup executor image into Kind (cluster=%s)", clusterName))
			} else {
				By(fmt.Sprintf("skipping backup executor image load (cluster=%s)", clusterName))
			}

			if loadUpgradeExecutorImage {
				By(fmt.Sprintf("loading the upgrade executor image on Kind (cluster=%s)", clusterName))
				err = utils.LoadImageToKindClusterWithName(upgradeExecutorImage)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the upgrade executor image into Kind (cluster=%s)", clusterName))
			} else {
				By(fmt.Sprintf("skipping upgrade executor image load (cluster=%s)", clusterName))
			}

			if preloadHardenedAssets && strings.TrimSpace(hardenedUpgradeExecutorImage) != "" && hardenedUpgradeExecutorImage != upgradeExecutorImage {
				By(fmt.Sprintf("loading the hardened upgrade executor image on Kind (cluster=%s)", clusterName))
				if _, err := utils.Run(exec.Command("docker", "image", "inspect", hardenedUpgradeExecutorImage)); err != nil { // #nosec G204 -- test harness command
					_, _ = fmt.Fprintf(GinkgoWriter, "Pulling hardened upgrade executor image %q...\n", hardenedUpgradeExecutorImage)
					_, err = utils.Run(exec.Command("docker", "pull", hardenedUpgradeExecutorImage)) // #nosec G204 -- test harness command
					ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to pull hardened upgrade executor image %q (cluster=%s)", hardenedUpgradeExecutorImage, clusterName))
				}
				err = utils.LoadImageToKindClusterWithName(hardenedUpgradeExecutorImage)
				ExpectWithOffset(1, err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to load the hardened upgrade executor image into Kind (cluster=%s)", clusterName))
			}

			By(fmt.Sprintf("pre-loading OpenBao images on Kind to reduce flakiness (cluster=%s)", clusterName))
			openBaoImages := []string{openBaoImage}
			if preloadUpgradeImages {
				openBaoImages = append(
					openBaoImages,
					fmt.Sprintf("openbao/openbao:%s", defaultUpgradeFromVersion),
					fmt.Sprintf("openbao/openbao:%s", defaultUpgradeToVersion),
				)
			}
			if preloadHardenedAssets {
				openBaoImages = append(openBaoImages, hardenedConfigInitImage)
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

			By(fmt.Sprintf("removing stale E2E metrics RBAC binding (cluster=%s)", clusterName))
			ExpectWithOffset(1, deleteE2EMetricsReaderBinding()).To(Succeed(), fmt.Sprintf("Failed to delete stale E2E metrics RBAC binding (cluster=%s)", clusterName))

			By(fmt.Sprintf("waiting for the operator namespace to become reusable (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForNamespaceDeletionIfTerminating(context.Background(), operatorNamespace, 60*time.Second, time.Second)).
				To(Succeed(), fmt.Sprintf("Operator namespace is stuck terminating (cluster=%s)", clusterName))

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

			By(fmt.Sprintf("installing the operator via %s (cluster=%s)", bootstrap.OperatorInstallMode, clusterName))
			ExpectWithOffset(1, installOperatorForE2E(context.Background(), operatorNamespace)).
				To(Succeed(), fmt.Sprintf("Failed to install the operator (cluster=%s)", clusterName))

			By(fmt.Sprintf("applying kube-apiserver token audience override when configured (cluster=%s)", clusterName))
			ExpectWithOffset(1, patchOperatorKubeAPITokenAudience(context.Background(), operatorNamespace)).
				To(Succeed(), fmt.Sprintf("Failed to patch kube-api token audience on operator Deployments (cluster=%s)", clusterName))

			By(fmt.Sprintf("waiting for operator deployments to become Available (cluster=%s)", clusterName))
			ExpectWithOffset(1, waitForDeploymentsAvailable(operatorNamespace, 5*time.Minute)).
				To(Succeed(), fmt.Sprintf("Operator deployments did not become Available in time (cluster=%s)", clusterName))
			if serviceClaimsE2EEnabled() {
				By(fmt.Sprintf("waiting for the controller admission webhook to publish ready endpoints (cluster=%s)", clusterName))
				ExpectWithOffset(1, waitForServiceEndpoints(operatorNamespace, operatorControllerWebhookServiceName, 2*time.Minute)).
					To(Succeed(), fmt.Sprintf("Controller admission webhook did not publish ready endpoints in time (cluster=%s)", clusterName))
				By(fmt.Sprintf("waiting for claim admission dry-run requests to succeed (cluster=%s)", clusterName))
				ExpectWithOffset(1, waitForClaimAdmissionWebhookReady(operatorNamespace, 2*time.Minute)).
					To(Succeed(), fmt.Sprintf("Claim admission webhook did not become reachable in time (cluster=%s)", clusterName))
			}

			By(fmt.Sprintf("applying controller env overrides when configured (cluster=%s)", clusterName))
			ExpectWithOffset(1, applyControllerEnvOverrides(operatorNamespace)).
				To(Succeed(), fmt.Sprintf("Failed to apply controller env overrides (cluster=%s)", clusterName))
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
	ExpectWithOffset(1, bootstrap.Clusters).To(HaveLen(1))
	ExpectWithOffset(1, bootstrap.Kubeconfigs).To(HaveLen(1))

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

	if suiteBootstrapState == nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping shared cleanup because suite bootstrap state was not initialized\n")
		return
	}

	// Single cluster cleanup
	if len(suiteBootstrapState.Clusters) > 0 {
		clusterName := suiteBootstrapState.Clusters[0]
		kubeconfigPath := suiteBootstrapState.Kubeconfigs[0]

		withEnv("KIND_CLUSTER", clusterName, func() {
			withEnv("KUBECONFIG", kubeconfigPath, func() {
				By(fmt.Sprintf("cleaning up the E2E metrics RBAC binding (cluster=%s)", clusterName))
				_ = deleteE2EMetricsReaderBinding()

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

				By(fmt.Sprintf("removing the operator via %s (cluster=%s)", suiteBootstrapState.OperatorInstallMode, clusterName))
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
				if err := uninstallOperatorForE2E(ctx, operatorNamespace, suiteBootstrapState.OperatorInstallMode); err != nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: operator uninstall failed (cluster=%s): %v\n", clusterName, err)
				}
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

				if !useExistingCluster || existingClusterFullCleanup {
					By(fmt.Sprintf("removing operator namespace (cluster=%s)", clusterName))
					ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
					cmd = exec.CommandContext(ctx, "kubectl", "delete", "ns", operatorNamespace, "--ignore-not-found", "--wait=false")
					_, _ = utils.Run(cmd)
					cancel()
				} else {
					_, _ = fmt.Fprintf(
						GinkgoWriter,
						"E2E_USE_EXISTING_CLUSTER: keeping operator namespace %q to avoid shared-cluster termination races (set E2E_EXISTING_CLUSTER_FULL_CLEANUP=true to enable deletion)\n",
						operatorNamespace,
					)
				}

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
