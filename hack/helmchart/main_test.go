package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	syaml "sigs.k8s.io/yaml"
)

func TestTransformPolicyToHelm_RewritesProvisionerServiceAccountVariable(t *testing.T) {
	input := `apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-restrict-provisioner-rbac
spec:
  variables:
    - name: provisioner_serviceaccount_name
      expression: >-
        'openbao-operator-provisioner'
`

	got := transformPolicyToHelm(input)
	want := `'{{ include "openbao-operator.provisionerServiceAccountName" . }}'`
	if !strings.Contains(got, want) {
		t.Fatalf("transformed policy missing provisioner helper expression %q:\n%s", want, got)
	}
	if strings.Contains(got, `'openbao-operator-provisioner'`) {
		t.Fatalf("transformed policy still contains stale provisioner ServiceAccount name:\n%s", got)
	}
}

func TestTransformRBACToHelm_DeduplicatesCommonLabels(t *testing.T) {
	input := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openbao-operator-controller
  labels:
    app.kubernetes.io/name: openbao-operator
    app.kubernetes.io/component: controller
    app.kubernetes.io/managed-by: kustomize
rules:
  - apiGroups:
      - openbao.org
    resources:
      - openbaoclusters
    verbs:
      - get
`

	got := transformRBACToHelm(input, "controller", false)
	if !strings.Contains(got, `{{- include "openbao-operator.labels" . | nindent 4 }}`) {
		t.Fatalf("transformed RBAC missing Helm labels helper:\n%s", got)
	}
	if strings.Contains(got, "app.kubernetes.io/name: openbao-operator") {
		t.Fatalf("transformed RBAC still contains duplicate app name label:\n%s", got)
	}
	if strings.Contains(got, "app.kubernetes.io/managed-by: kustomize") {
		t.Fatalf("transformed RBAC still contains duplicate managed-by label:\n%s", got)
	}
	if !strings.Contains(got, "app.kubernetes.io/component: controller") {
		t.Fatalf("transformed RBAC dropped component label:\n%s", got)
	}
}

func TestSyncAggregatedRBAC_IncludesHelperImageDelegationRole(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	writeRole := func(filename, name string, verbs ...string) {
		t.Helper()

		var builder strings.Builder
		builder.WriteString("apiVersion: rbac.authorization.k8s.io/v1\n")
		builder.WriteString("kind: ClusterRole\n")
		builder.WriteString("metadata:\n")
		builder.WriteString("  name: " + name + "\n")
		builder.WriteString("rules:\n")
		builder.WriteString("  - apiGroups:\n")
		builder.WriteString("      - openbao.org\n")
		builder.WriteString("    resources:\n")
		builder.WriteString("      - openbaoclusters\n")
		builder.WriteString("    verbs:\n")
		for _, verb := range verbs {
			builder.WriteString("      - " + verb + "\n")
		}

		if err := os.WriteFile(filepath.Join(inputDir, filename), []byte(builder.String()), 0o600); err != nil {
			t.Fatalf("write %s: %v", filename, err)
		}
	}

	writeRole("openbaocluster_admin_role.yaml", "openbaocluster-admin-role", "*")
	writeRole("openbaocluster_editor_role.yaml", "openbaocluster-editor-role", "create", "update")
	writeRole("openbaocluster_helper_image_role.yaml", "openbaocluster-helper-image-role", "get", "usehelperimages")
	writeRole("openbaocluster_viewer_role.yaml", "openbaocluster-viewer-role", "get", "list")
	writeRole("openbaotenant_editor_role.yaml", "openbaotenant-editor-role", "create", "update")

	if err := syncAggregatedRBAC(options{rbacInputDir: inputDir, rbacOutputDir: outputDir}); err != nil {
		t.Fatalf("syncAggregatedRBAC() failed: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(outputDir, "aggregated-clusterroles.yaml"))
	if err != nil {
		t.Fatalf("read generated aggregated roles: %v", err)
	}
	output := string(got)
	for _, want := range []string{
		`{{ include "openbao-operator.fullname" . }}-openbaocluster-helper-image`,
		"usehelperimages",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated aggregated RBAC missing %q:\n%s", want, output)
		}
	}
}

func TestSyncCRDsFiltersToCoreChartAndPrunesStaleOutputs(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()

	for _, crd := range []string{
		"openbao.org_openbaobackupauthprofiles.yaml",
		"openbao.org_openbaobackupbackends.yaml",
		"openbao.org_openbaobackupprofiles.yaml",
		"openbao.org_openbaobackuptargets.yaml",
		"openbao.org_openbaobootstrapprofiles.yaml",
		"openbao.org_openbaoclusters.yaml",
		"openbao.org_openbaoclusterclaims.yaml",
		"openbao.org_openbaoentrypoints.yaml",
		"openbao.org_openbaoexposureclasses.yaml",
		"openbao.org_openbaoingresspolicies.yaml",
		"openbao.org_openbaonetworkprofiles.yaml",
		"openbao.org_openbaoobservabilityprofiles.yaml",
		"openbao.org_openbaorestores.yaml",
		"openbao.org_openbaoruntimeprofiles.yaml",
		"openbao.org_openbaoserviceofferings.yaml",
		"openbao.org_openbaoserviceofferingrollouts.yaml",
		"openbao.org_openbaoserviceprofiles.yaml",
		"openbao.org_openbaostorageprofiles.yaml",
		"openbao.org_openbaotenants.yaml",
		"openbao.org_openbaotransferprofiles.yaml",
		"openbao.org_openbaounsealprofiles.yaml",
		"openbao.org_openbaoupgradepolicies.yaml",
		"openbao.org_unrelateds.yaml",
	} {
		plural := strings.TrimSuffix(strings.TrimPrefix(crd, "openbao.org_"), ".yaml")
		writeYAML(t, filepath.Join(inputDir, crd), sampleCRD(plural+".openbao.org"))
	}
	writeYAML(t, filepath.Join(outputDir, "openbao.org_unrelateds.yaml"), "stale")

	err := syncCRDs(options{crdInputDir: inputDir, crdOutputDir: outputDir})
	if err != nil {
		t.Fatalf("syncCRDs() error = %v", err)
	}

	for _, name := range []string{
		"openbao.org_openbaobackupauthprofiles.yaml",
		"openbao.org_openbaobackupbackends.yaml",
		"openbao.org_openbaobackupprofiles.yaml",
		"openbao.org_openbaobackuptargets.yaml",
		"openbao.org_openbaobootstrapprofiles.yaml",
		"openbao.org_openbaoclusters.yaml",
		"openbao.org_openbaoclusterclaims.yaml",
		"openbao.org_openbaoentrypoints.yaml",
		"openbao.org_openbaoexposureclasses.yaml",
		"openbao.org_openbaoingresspolicies.yaml",
		"openbao.org_openbaonetworkprofiles.yaml",
		"openbao.org_openbaoobservabilityprofiles.yaml",
		"openbao.org_openbaorestores.yaml",
		"openbao.org_openbaoruntimeprofiles.yaml",
		"openbao.org_openbaoserviceofferings.yaml",
		"openbao.org_openbaoserviceofferingrollouts.yaml",
		"openbao.org_openbaoserviceprofiles.yaml",
		"openbao.org_openbaostorageprofiles.yaml",
		"openbao.org_openbaotenants.yaml",
		"openbao.org_openbaotransferprofiles.yaml",
		"openbao.org_openbaounsealprofiles.yaml",
		"openbao.org_openbaoupgradepolicies.yaml",
	} {
		path := filepath.Join(outputDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read synced CRD %q: %v", name, err)
		}
		if string(data) == "stale" {
			t.Fatalf("synced CRD %q was not refreshed", name)
		}
		if !strings.Contains(string(data), "helm.sh/resource-policy: keep") {
			t.Fatalf("synced CRD %q missing keep annotation", name)
		}
	}

	for _, name := range []string{
		"openbao.org_unrelateds.yaml",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); !os.IsNotExist(err) {
			t.Fatalf("excluded CRD %q was not pruned", name)
		}
	}
}

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func sampleCRD(name string) string {
	return "" +
		"apiVersion: apiextensions.k8s.io/v1\n" +
		"kind: CustomResourceDefinition\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  annotations:\n" +
		"    controller-gen.kubebuilder.io/version: v0.19.0\n" +
		"spec:\n" +
		"  group: openbao.org\n"
}

func TestCoreChartCRDFilesIncludeStartupRequiredSurfaces(t *testing.T) {
	for _, name := range []string{
		"openbao.org_openbaoclusterclaims.yaml",
		"openbao.org_openbaoserviceofferings.yaml",
		"openbao.org_openbaoingresspolicies.yaml",
	} {
		if _, ok := coreChartCRDFiles[name]; !ok {
			t.Fatalf("coreChartCRDFiles missing required startup surface %q", name)
		}
	}
}

func TestAddNamespacePodSecurityLabelRBACMode_ConditionsNamespaceMutationVerbs(t *testing.T) {
	input := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: openbao-operator-provisioner
rules:
  - apiGroups:
      - ""
    resources:
      - namespaces
    verbs:
      - create
      - get
      - update
      - patch
`

	got, err := addNamespacePodSecurityLabelRBACMode(input)
	if err != nil {
		t.Fatalf("addNamespacePodSecurityLabelRBACMode() failed: %v", err)
	}
	if !strings.Contains(got, `{{ if eq .Values.tenancy.namespacePodSecurityLabels.mode "enforce" }}`) {
		t.Fatalf("transformed RBAC missing namespace Pod Security label mode conditional:\n%s", got)
	}
	if !strings.Contains(got, "      - create\n      - get\n{{ if") {
		t.Fatalf("transformed RBAC should leave create/get outside the conditional:\n%s", got)
	}
}

func TestAddNamespacePodSecurityLabelPolicyMode_AddsExternalDenyRule(t *testing.T) {
	input := `apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-restrict-provisioner-namespace-mutations
spec:
  validations:
    # Rule 1: Only enforce Pod Security Standards labels (restricted), and do not change anything else.
    - expression: >-
        !variables.is_provisioner
      message: "The Provisioner may only enforce Pod Security Standards labels (restricted) on Namespaces."
`

	got, err := addNamespacePodSecurityLabelPolicyMode(input)
	if err != nil {
		t.Fatalf("addNamespacePodSecurityLabelPolicyMode() failed: %v", err)
	}
	for _, want := range []string{
		`{{ if eq .Values.tenancy.namespacePodSecurityLabels.mode "external" }}`,
		"The Provisioner may not mutate Namespaces when tenant namespace Pod Security labels are externally managed.",
		`{{ else }}`,
		"The Provisioner may only enforce Pod Security Standards labels (restricted) on Namespaces.",
		`{{ end }}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("transformed policy missing %q:\n%s", want, got)
		}
	}
}

func TestHelmTemplateRetainsProvisionerNamespaceCreateAndWebhookIngress(t *testing.T) {
	renderedBytes := renderChart(t)
	rendered := string(renderedBytes)

	var provisionerRole struct {
		Rules []renderedRBACRule `json:"rules"`
	}
	if !findRenderedYAMLObject(t, renderedBytes, "ClusterRole", "test-openbao-operator-provisioner", &provisionerRole) {
		t.Fatalf("rendered chart missing provisioner ClusterRole:\n%s", rendered)
	}
	if !hasResourceVerbs(provisionerRole.Rules, "namespaces", "create", "get", "update", "patch") {
		t.Fatalf("rendered chart missing provisioner namespace create/update surface:\n%s", rendered)
	}

	if !strings.Contains(rendered, "kind: NetworkPolicy") ||
		!strings.Contains(rendered, "name: test-openbao-operator-allow-webhook") ||
		!strings.Contains(rendered, "- port: 9443") {
		t.Fatalf("rendered chart missing webhook ingress allow rule:\n%s", rendered)
	}

	if !strings.Contains(rendered, "system:controller:namespace-controller") ||
		!strings.Contains(rendered, "system:serviceaccount:kube-system:namespace-controller") {
		t.Fatalf("rendered chart missing namespace-controller managed-resource deletion allowance:\n%s", rendered)
	}

	if !strings.Contains(rendered, "name: test-openbao-operator-provisioner") ||
		!strings.Contains(rendered, "startupProbe:") ||
		!strings.Contains(rendered, "failureThreshold: 30") ||
		!strings.Contains(rendered, "timeoutSeconds: 5") ||
		!strings.Contains(rendered, "memory: 256Mi") ||
		!strings.Contains(rendered, "cpu: 500m") {
		t.Fatalf("rendered chart missing provisioner startup or resource hardening:\n%s", rendered)
	}
}

func TestHelmTemplateIncludesRequiredCRDs(t *testing.T) {
	rendered := string(renderChart(t))

	for _, name := range []string{
		"openbaoclusterclaims.openbao.org",
		"openbaoserviceofferings.openbao.org",
		"openbaoingresspolicies.openbao.org",
		"openbaostorageprofiles.openbao.org",
		"openbaounsealprofiles.openbao.org",
		"openbaoruntimeprofiles.openbao.org",
		"openbaoobservabilityprofiles.openbao.org",
		"openbaonetworkprofiles.openbao.org",
		"openbaoupgradepolicies.openbao.org",
	} {
		if !strings.Contains(rendered, "name: "+name) {
			t.Fatalf("rendered chart missing CRD %q", name)
		}
	}
}

func TestHelmTemplateRendersStrictYAML(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "default"},
		{name: "multi", args: []string{"--set", "tenancy.mode=multi"}},
		{
			name: "external-namespace-pod-security-labels",
			args: []string{"--set", "tenancy.namespacePodSecurityLabels.mode=external"},
		},
		{name: "single", args: []string{"--set", "tenancy.mode=single", "--set", "tenancy.targetNamespace=openbao-system"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderChart(t, tt.args...)
			assertStrictYAML(t, rendered)
		})
	}
}

func renderChart(t *testing.T, extraArgs ...string) []byte {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm binary not available")
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current file path")
	}
	chartDir := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", "charts", "openbao-operator"))
	args := []string{
		"template",
		"test",
		chartDir,
		"--namespace",
		"openbao",
		"--include-crds",
	}
	args = append(args, extraArgs...)
	cmd := exec.Command("helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, string(output))
	}
	return output
}

func findRenderedYAMLObject(t *testing.T, rendered []byte, kind, name string, out interface{}) bool {
	t.Helper()

	for _, doc := range bytes.Split(rendered, []byte("\n---")) {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		var meta struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		}
		if err := syaml.Unmarshal(doc, &meta); err != nil {
			t.Fatalf("decode rendered document metadata: %v\n%s", err, string(doc))
		}
		if meta.Kind != kind || meta.Metadata.Name != name {
			continue
		}
		if err := syaml.Unmarshal(doc, out); err != nil {
			t.Fatalf("decode rendered %s/%s: %v\n%s", kind, name, err, string(doc))
		}
		return true
	}
	return false
}

type renderedRBACRule struct {
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
}

func hasResourceVerbs(rules []renderedRBACRule, resource string, verbs ...string) bool {
	for _, rule := range rules {
		if !containsString(rule.Resources, resource) {
			continue
		}
		for _, verb := range verbs {
			if !containsString(rule.Verbs, verb) {
				return false
			}
		}
		return true
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertStrictYAML(t *testing.T, rendered []byte) {
	t.Helper()

	for i, doc := range bytes.Split(rendered, []byte("\n---")) {
		doc = bytes.TrimSpace(doc)
		if len(doc) == 0 {
			continue
		}
		var out map[string]interface{}
		if err := syaml.UnmarshalStrict(doc, &out); err != nil {
			t.Fatalf("rendered document %d failed strict YAML decoding: %v\n%s", i+1, err, string(doc))
		}
	}
}
