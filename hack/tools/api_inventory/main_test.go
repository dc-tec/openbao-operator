package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestCollectSchemaFieldsIncludesNestedArraysAndSchemaFacts(t *testing.T) {
	schema := apiextensionsv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"items"},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"items": {
				Type: "array",
				Items: &apiextensionsv1.JSONSchemaPropsOrArray{
					Schema: &apiextensionsv1.JSONSchemaProps{
						Type:     "object",
						Required: []string{"name"},
						Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"name": {
								Type:        "string",
								Description: "Deprecated: use id.",
							},
						},
					},
				},
			},
			"enabled": {
				Type:    "boolean",
				Default: &apiextensionsv1.JSON{Raw: []byte("true")},
			},
		},
	}

	fields := collectSchemaFields("spec", schema)
	if len(fields) != 3 {
		t.Fatalf("field count = %d, want 3", len(fields))
	}
	assertSchemaField(t, fields[0], "spec.enabled", "boolean", false, "true")
	assertSchemaField(t, fields[1], "spec.items", "array<object>", true, "-")
	assertSchemaField(t, fields[2], "spec.items[].name", "string", true, "-")
	if !strings.Contains(fields[2].Description, "Deprecated:") {
		t.Fatalf("description = %q, want deprecation marker", fields[2].Description)
	}
}

func TestSchemaValidationCanonicalizesCELRuleContents(t *testing.T) {
	optionalOldSelf := false
	reason := apiextensionsv1.FieldValueForbidden
	rules := apiextensionsv1.ValidationRules{
		{
			Rule:              "self != oldSelf",
			MessageExpression: "'must change from ' + string(oldSelf)",
			FieldPath:         ".value",
			Reason:            &reason,
			OptionalOldSelf:   &optionalOldSelf,
		},
		{
			Rule:    "self > 0",
			Message: "must be positive",
		},
	}

	got := schemaValidation(apiextensionsv1.JSONSchemaProps{XValidations: rules})
	want := `cel=[{"rule":"self != oldSelf","messageExpression":"'must change from ' + string(oldSelf)",` +
		`"fieldPath":".value","reason":"FieldValueForbidden","optionalOldSelf":"false"},` +
		`{"rule":"self \u003e 0","message":"must be positive"}]`
	if got != want {
		t.Fatalf("schemaValidation() = %q, want %q", got, want)
	}

	reordered := apiextensionsv1.ValidationRules{rules[1], rules[0]}
	if got != schemaValidation(apiextensionsv1.JSONSchemaProps{XValidations: reordered}) {
		t.Fatal("schemaValidation() changed when CEL rules were reordered")
	}

	changed := append(apiextensionsv1.ValidationRules(nil), rules...)
	changed[1].Rule = "self >= 0"
	if got == schemaValidation(apiextensionsv1.JSONSchemaProps{XValidations: changed}) {
		t.Fatal("schemaValidation() did not change when CEL rule content changed")
	}
}

func TestResolvePolicyMergesInheritedAndSpecificRules(t *testing.T) {
	resource := resourceInventory{
		Defaults: resourceDefaults{
			Spec: policyDefaults{
				Mutability:        "mutable",
				Migration:         "required-if-changed",
				ModuleInteraction: "none",
				Enforcement:       []string{"crd-schema", "controller"},
			},
		},
		Rules: []inventoryRule{
			{
				Path:           "spec.backup",
				Classification: "beta-stable",
				Owner:          "backup",
			},
			{
				Path:           "spec.backup.target",
				Classification: "likely-move",
				Rationale:      "mixed provider shape",
			},
		},
	}

	policy, matched := resolvePolicy(resource, "spec.backup.target.bucket")
	if policy.Classification != "likely-move" {
		t.Fatalf("classification = %q, want likely-move", policy.Classification)
	}
	if policy.Owner != "backup" {
		t.Fatalf("owner = %q, want inherited backup", policy.Owner)
	}
	if policy.Mutability != "mutable" || policy.Migration != "required-if-changed" {
		t.Fatalf("defaults not inherited: %#v", policy)
	}
	if strings.Join(policy.Enforcement, ",") != "crd-schema,controller" {
		t.Fatalf("enforcement = %v, want inherited layers", policy.Enforcement)
	}
	if policy.RulePath != "spec.backup.target" {
		t.Fatalf("rule path = %q, want spec.backup.target", policy.RulePath)
	}
	if strings.Join(matched, ",") != "spec.backup,spec.backup.target" {
		t.Fatalf("matched rules = %v", matched)
	}
}

func TestValidateResolvedFieldRequiresExplicitTopLevelRule(t *testing.T) {
	field := schemaField{Path: "spec.version"}
	policy := effectivePolicy{
		Classification:    "beta-stable",
		Owner:             "workload",
		Mutability:        "mutable",
		Migration:         "required-if-changed",
		ModuleInteraction: "none",
		Enforcement:       []string{"crd-schema", "controller"},
	}

	errs := validateResolvedField("OpenBaoCluster", field, policy, nil)
	if !containsError(errs, "top-level field requires an explicit rule") {
		t.Fatalf("errors = %v, want explicit top-level rule failure", errs)
	}
}

func TestValidateResolvedFieldRequiresExplicitDeprecation(t *testing.T) {
	field := schemaField{
		Path:        "spec.tls.acme.domain",
		Description: "Deprecated: use domains.",
	}
	policy := effectivePolicy{
		Classification:    "beta-stable",
		Owner:             "transport-security",
		Mutability:        "mutable",
		Migration:         "required-if-changed",
		ModuleInteraction: "none",
		Enforcement:       []string{"crd-schema", "controller"},
	}

	errs := validateResolvedField("OpenBaoCluster", field, policy, map[string]inventoryRule{
		"spec.tls": {Path: "spec.tls", Classification: "beta-stable"},
	})
	if !containsError(errs, "schema deprecation requires an explicit deprecated rule") {
		t.Fatalf("errors = %v, want explicit deprecation failure", errs)
	}
}

func TestRepositoryInventoryIsComplete(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("change to repository root: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	report, err := buildReport(defaultInventoryPath)
	if err != nil {
		t.Fatalf("buildReport() error = %v", err)
	}
	if len(report.Resources) != 3 {
		t.Fatalf("resource count = %d, want 3", len(report.Resources))
	}
	if report.DecisionStatus != "proposed" {
		t.Fatalf("decision status = %q, want proposed", report.DecisionStatus)
	}
	if err := verifySnapshot(report.Snapshot, report); err != nil {
		t.Fatalf("verifySnapshot() error = %v", err)
	}
}

func TestWriteSummary(t *testing.T) {
	report := inventoryReport{
		Release:        "0.5.0",
		Baseline:       "0.4.2",
		DecisionStatus: "proposed",
		Resources: []resourceReport{
			{
				Kind:   "OpenBaoCluster",
				Fields: make([]resolvedField, 3),
				Counts: map[string]int{"beta-stable": 2, "likely-move": 1},
			},
		},
	}

	var out bytes.Buffer
	if err := writeSummary(&out, report); err != nil {
		t.Fatalf("writeSummary() error = %v", err)
	}
	want := `API stability inventory: pass (release 0.5.0, baseline 0.4.2, decisions proposed)
OpenBaoCluster: 3 schema fields
  beta-stable          2
  likely-move          1
`
	if out.String() != want {
		t.Fatalf("summary =\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRenderSnapshotIncludesResolvedSchemaAndPolicy(t *testing.T) {
	report := inventoryReport{
		Resources: []resourceReport{
			{
				Kind: "OpenBaoCluster",
				Fields: []resolvedField{
					{
						Resource: "OpenBaoCluster",
						Field: schemaField{
							Path:        "spec.version",
							Type:        "string",
							Required:    true,
							Default:     "-",
							Validation:  "minLength=1",
							Description: "Version selects OpenBao. More detail.",
						},
						Policy: effectivePolicy{
							Classification:    "beta-stable",
							Owner:             "workload",
							Mutability:        "mutable",
							Enforcement:       []string{"crd-schema", "controller"},
							ModuleInteraction: "none",
							Migration:         "required-if-changed",
							RulePath:          "spec.version",
						},
					},
				},
			},
		},
	}

	var out bytes.Buffer
	if err := renderSnapshot(&out, report); err != nil {
		t.Fatalf("renderSnapshot() error = %v", err)
	}
	if !strings.Contains(out.String(),
		"OpenBaoCluster\tspec.version\tstring\ttrue\t-\tminLength=1\tbeta-stable\tworkload") {
		t.Fatalf("snapshot =\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Version selects OpenBao.") {
		t.Fatalf("snapshot purpose missing: %s", out.String())
	}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		if strings.HasSuffix(line, "\t") || strings.HasSuffix(line, " ") {
			t.Fatalf("snapshot line has trailing whitespace: %q", line)
		}
	}
}

func assertSchemaField(t *testing.T, field schemaField, path, fieldType string, required bool, defaultValue string) {
	t.Helper()
	if field.Path != path || field.Type != fieldType || field.Required != required || field.Default != defaultValue {
		t.Fatalf(
			"field = %#v, want path=%q type=%q required=%t default=%q",
			field,
			path,
			fieldType,
			required,
			defaultValue,
		)
	}
}

func containsError(errs []string, want string) bool {
	for _, err := range errs {
		if strings.Contains(err, want) {
			return true
		}
	}
	return false
}
