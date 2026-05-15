package main

import (
	"bytes"
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
	if !strings.Contains(got, "      - get\n{{ if") {
		t.Fatalf("transformed RBAC should leave get outside the conditional:\n%s", got)
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
