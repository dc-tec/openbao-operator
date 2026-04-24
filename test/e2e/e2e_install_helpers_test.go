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
	"time"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/test/utils"
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

func patchOperatorServiceClaimsRuntime(ctx context.Context, namespace string) error {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(claimE2EEnableEnv)), "true") {
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

	for _, deploymentName := range []string{"openbao-operator-controller", "openbao-operator-provisioner"} {
		deploy := &appsv1.Deployment{}
		if err := c.Get(ctx, client.ObjectKey{Name: deploymentName, Namespace: namespace}, deploy); err != nil {
			return fmt.Errorf("failed to get %s deployment: %w", deploymentName, err)
		}

		original := deploy.DeepCopy()
		updated := false
		for i := range deploy.Spec.Template.Spec.Containers {
			container := &deploy.Spec.Template.Spec.Containers[i]
			if container.Name != "manager" {
				continue
			}

			updated = upsertEnvVar(container, constants.EnvOperatorEnableServiceClaims, "true") || updated
			if deploymentName != "openbao-operator-controller" {
				continue
			}
			if apiServerCIDR != "" {
				updated = upsertEnvVar(container, constants.EnvOperatorServiceClaimsAPIServerCIDR, apiServerCIDR) || updated
			}

			if endpointIPs := strings.TrimSpace(apiServerEndpointIPs); endpointIPs != "" {
				updated = upsertEnvVar(container, constants.EnvOperatorServiceClaimsAPIServerEndpointIPs, endpointIPs) || updated
			}
			if dnsIPs := strings.TrimSpace(os.Getenv(claimE2EDNSEndpointIPsEnv)); dnsIPs != "" {
				updated = upsertEnvVar(container, constants.EnvOperatorServiceClaimsDNSEndpointIPs, dnsIPs) || updated
			}
		}

		if !updated {
			continue
		}

		if err := c.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
			return fmt.Errorf("failed to patch %s service-claims runtime: %w", deploymentName, err)
		}
	}

	return nil
}

type helmE2EValues struct {
	Image               helmE2EImageValues               `yaml:"image,omitempty"`
	OperatorVersion     string                           `yaml:"operatorVersion,omitempty"`
	Controller          helmE2EControllerValues          `yaml:"controller,omitempty"`
	Provisioner         helmE2EControllerValues          `yaml:"provisioner,omitempty"`
	ServiceAccountToken helmE2EServiceAccountTokenValues `yaml:"serviceAccountToken,omitempty"`
}

type helmE2EImageValues struct {
	Repository string `yaml:"repository,omitempty"`
	Tag        string `yaml:"tag,omitempty"`
	Digest     string `yaml:"digest,omitempty"`
}

type helmE2EControllerValues struct {
	ExtraEnv []helmE2EEnvVar `yaml:"extraEnv,omitempty"`
}

type helmE2EServiceAccountTokenValues struct {
	OpenBaoAudience    string `yaml:"openBaoAudience,omitempty"`
	KubernetesAudience string `yaml:"kubernetesAudience,omitempty"`
}

type helmE2EEnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

func imageTagForOperatorHelper(ref string) (repository, tag string, err error) {
	repository, tag, digest := splitImageReference(ref)
	if strings.TrimSpace(ref) == "" {
		return "", "", fmt.Errorf("helper image reference is empty")
	}
	if digest != "" {
		return "", "", fmt.Errorf("helper image %q uses a digest; the operator runtime currently requires a shared helper tag via OPERATOR_VERSION", ref)
	}
	if repository == "" || tag == "" {
		return "", "", fmt.Errorf("helper image %q must include a repository and tag", ref)
	}
	return repository, tag, nil
}

func helperImageContractForHelmE2E() (map[string]string, string, error) {
	helperRepos := map[string]string{}
	helperImages := []struct {
		envName string
		ref     string
	}{
		{envName: constants.EnvOperatorInitImageRepo, ref: configInitImage},
		{envName: constants.EnvOperatorBackupImageRepo, ref: backupExecutorImage},
		{envName: constants.EnvOperatorUpgradeImageRepo, ref: upgradeExecutorImage},
	}

	var operatorVersion string
	for _, helper := range helperImages {
		repository, tag, err := imageTagForOperatorHelper(helper.ref)
		if err != nil {
			return nil, "", fmt.Errorf("derive helper image contract for claim-capable Helm E2E install: %w", err)
		}
		if operatorVersion == "" {
			operatorVersion = tag
		} else if operatorVersion != tag {
			return nil, "", fmt.Errorf(
				"claim-capable Helm E2E install requires shared helper tag; got %s=%q but %s=%q",
				helperImages[0].envName,
				operatorVersion,
				helper.envName,
				tag,
			)
		}
		helperRepos[helper.envName] = repository
	}

	return helperRepos, operatorVersion, nil
}

func operatorInstallModeForE2E() string {
	if serviceClaimsE2EEnabled() {
		return operatorInstallModeHelm
	}
	return operatorInstallModeKustomize
}

func installOperatorForE2E(ctx context.Context, namespace string) error {
	switch operatorInstallModeForE2E() {
	case operatorInstallModeHelm:
		return installOperatorWithHelm(ctx, namespace)
	default:
		return installOperatorWithKustomize(ctx, namespace)
	}
}

func uninstallOperatorForE2E(ctx context.Context, namespace string, mode string) error {
	switch mode {
	case operatorInstallModeHelm:
		cmd := exec.CommandContext(ctx, "helm", "uninstall", "openbao-operator", "--namespace", namespace) // #nosec G204 -- test harness command
		_, err := utils.Run(cmd)
		if err != nil && !strings.Contains(err.Error(), "release: not found") {
			return err
		}
		return nil
	default:
		cmd := exec.CommandContext(ctx, "make", "undeploy", "ignore-not-found=true", "wait=false") // #nosec G204 -- test harness command
		_, err := utils.Run(cmd)
		return err
	}
}

func installOperatorWithKustomize(ctx context.Context, namespace string) error {
	cmd := exec.Command("make", "install") // #nosec G204 -- test harness command
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("install CRDs: %w", err)
	}
	if err := waitForCRDsEstablished(2 * time.Minute); err != nil {
		return fmt.Errorf("wait for CRDs established: %w", err)
	}

	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage)) // #nosec G204 -- test harness command
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("deploy operator: %w", err)
	}

	if err := patchOperatorServiceClaimsRuntime(ctx, namespace); err != nil {
		return fmt.Errorf("patch claim runtime env: %w", err)
	}

	return nil
}

func installOperatorWithHelm(_ context.Context, namespace string) error {
	valuesFile, err := writeHelmE2EValuesFile()
	if err != nil {
		return fmt.Errorf("write Helm E2E values file: %w", err)
	}
	defer os.Remove(valuesFile)

	if !helmReleaseExists(namespace) {
		cmd := exec.Command("make", "undeploy", "ignore-not-found=true", "wait=false") // #nosec G204 -- test harness command
		_, _ = utils.Run(cmd)
		if err := deleteRenderedHelmResources(namespace, valuesFile); err != nil {
			return fmt.Errorf("delete pre-existing Helm-shaped resources: %w", err)
		}
	}

	if err := applyHelmChartCRDs(); err != nil {
		return fmt.Errorf("apply Helm chart CRDs: %w", err)
	}

	if err := waitForCRDsEstablished(2 * time.Minute); err != nil {
		return fmt.Errorf("wait for CRDs established: %w", err)
	}

	args := []string{
		"upgrade", "--install", "openbao-operator", "charts/openbao-operator",
		"--namespace", namespace,
		"--create-namespace",
		"--wait",
		"--timeout=5m",
		"--values", valuesFile,
	}

	cmd := exec.Command("helm", args...) // #nosec G204 -- test harness command
	if _, err := utils.Run(cmd); err != nil {
		return fmt.Errorf("helm upgrade/install operator: %w", err)
	}

	return nil
}

func applyHelmChartCRDs() error {
	cmd := exec.Command("kubectl", "apply", "-f", "charts/openbao-operator/crds") // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err
}

func helmReleaseExists(namespace string) bool {
	cmd := exec.Command("helm", "status", "openbao-operator", "--namespace", namespace) // #nosec G204 -- test harness command
	_, err := utils.Run(cmd)
	return err == nil
}

func deleteRenderedHelmResources(namespace, valuesFile string) error {
	renderedFile, err := os.CreateTemp("", "openbao-operator-e2e-rendered-*.yaml")
	if err != nil {
		return err
	}
	renderedPath := renderedFile.Name()
	if err := renderedFile.Close(); err != nil {
		return err
	}
	defer os.Remove(renderedPath)

	templateArgs := []string{
		"template", "openbao-operator", "charts/openbao-operator",
		"--namespace", namespace,
		"--values", valuesFile,
	}
	templateCmd := exec.Command("helm", templateArgs...) // #nosec G204 -- test harness command
	rendered, err := utils.Run(templateCmd)
	if err != nil {
		return err
	}
	if err := os.WriteFile(renderedPath, []byte(rendered), 0o600); err != nil {
		return err
	}

	deleteCmd := exec.Command("kubectl", "delete", "--ignore-not-found", "--wait=false", "-f", renderedPath) // #nosec G204 -- test harness command
	_, err = utils.Run(deleteCmd)
	return err
}

func writeHelmE2EValuesFile() (string, error) {
	values := helmE2EValues{}

	repository, tag, digest := splitImageReference(projectImage)
	values.Image.Repository = repository
	values.Image.Tag = tag
	values.Image.Digest = digest

	if version := strings.TrimSpace(os.Getenv(constants.EnvOperatorVersion)); version != "" {
		values.OperatorVersion = version
	}

	if serviceClaimsE2EEnabled() {
		helperRepos, helperTag, err := helperImageContractForHelmE2E()
		if err != nil {
			return "", err
		}
		values.OperatorVersion = helperTag
		for _, envName := range []string{
			constants.EnvOperatorInitImageRepo,
			constants.EnvOperatorBackupImageRepo,
			constants.EnvOperatorUpgradeImageRepo,
		} {
			values.Controller.ExtraEnv = append(values.Controller.ExtraEnv,
				helmE2EEnvVar{Name: envName, Value: helperRepos[envName]},
			)
			values.Provisioner.ExtraEnv = append(values.Provisioner.ExtraEnv,
				helmE2EEnvVar{Name: envName, Value: helperRepos[envName]},
			)
		}
	}

	if audience := strings.TrimSpace(os.Getenv("OPENBAO_JWT_AUDIENCE")); audience != "" {
		values.ServiceAccountToken.OpenBaoAudience = audience
	}
	if audience := strings.TrimSpace(os.Getenv("OPENBAO_KUBE_API_AUDIENCE")); audience != "" {
		values.ServiceAccountToken.KubernetesAudience = audience
	}

	if serviceClaimsE2EEnabled() {
		values.Controller.ExtraEnv = append(values.Controller.ExtraEnv,
			helmE2EEnvVar{Name: constants.EnvOperatorEnableServiceClaims, Value: "true"},
		)
		values.Provisioner.ExtraEnv = append(values.Provisioner.ExtraEnv,
			helmE2EEnvVar{Name: constants.EnvOperatorEnableServiceClaims, Value: "true"},
		)
		if apiServerCIDR != "" {
			values.Controller.ExtraEnv = append(values.Controller.ExtraEnv,
				helmE2EEnvVar{Name: constants.EnvOperatorServiceClaimsAPIServerCIDR, Value: apiServerCIDR},
			)
		}
		if endpointIPs := strings.TrimSpace(apiServerEndpointIPs); endpointIPs != "" {
			values.Controller.ExtraEnv = append(values.Controller.ExtraEnv,
				helmE2EEnvVar{Name: constants.EnvOperatorServiceClaimsAPIServerEndpointIPs, Value: endpointIPs},
			)
		}
		if dnsIPs := strings.TrimSpace(os.Getenv(claimE2EDNSEndpointIPsEnv)); dnsIPs != "" {
			values.Controller.ExtraEnv = append(values.Controller.ExtraEnv,
				helmE2EEnvVar{Name: constants.EnvOperatorServiceClaimsDNSEndpointIPs, Value: dnsIPs},
			)
		}
	}

	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}

	file, err := os.CreateTemp("", "openbao-operator-e2e-values-*.yaml")
	if err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}

	return file.Name(), nil
}

func splitImageReference(ref string) (repository, tag, digest string) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return "", "", ""
	}
	if parts := strings.SplitN(trimmed, "@", 2); len(parts) == 2 {
		return parts[0], "", parts[1]
	}

	lastSlash := strings.LastIndex(trimmed, "/")
	lastColon := strings.LastIndex(trimmed, ":")
	if lastColon > lastSlash {
		return trimmed[:lastColon], trimmed[lastColon+1:], ""
	}

	return trimmed, "", ""
}
