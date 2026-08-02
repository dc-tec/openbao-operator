package main

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestCollectSchemaFieldsIncludesNestedArraysAndSchemaFacts(t *testing.T) {
	listType := "map"
	mapType := "atomic"
	schema := apiextensionsv1.JSONSchemaProps{
		Type:     "object",
		Required: []string{"items"},
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"items": {
				Type:         "array",
				XListType:    &listType,
				XListMapKeys: []string{"name"},
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
			"labels": {
				Type:     "object",
				XMapType: &mapType,
				AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{
					Allows: true,
					Schema: &apiextensionsv1.JSONSchemaProps{Type: "string", MinLength: int64Ptr(1)},
				},
			},
			"port": {
				AnyOf:        []apiextensionsv1.JSONSchemaProps{{Type: "integer"}, {Type: "string"}},
				XIntOrString: true,
			},
		},
	}

	fields := collectSchemaFields("spec", schema)
	if len(fields) != 9 {
		t.Fatalf("field count = %d, want 9", len(fields))
	}
	assertSchemaField(t, fields[0], "spec.enabled", "boolean", false, "true")
	assertSchemaField(t, fields[1], "spec.items", "array<object>", true, "-")
	assertSchemaField(t, fields[2], "spec.items[]", "object", false, "-")
	assertSchemaField(t, fields[3], "spec.items[].name", "string", true, "-")
	assertSchemaField(t, fields[4], "spec.labels", "object", false, "-")
	assertSchemaField(t, fields[5], "spec.labels.*", "string", false, "-")
	assertSchemaField(t, fields[6], "spec.port", "anyOf", false, "-")
	assertSchemaField(t, fields[7], "spec.port.anyOf[0]", "integer", false, "-")
	assertSchemaField(t, fields[8], "spec.port.anyOf[1]", "string", false, "-")
	if !strings.Contains(fields[3].Description, "Deprecated:") {
		t.Fatalf("description = %q, want deprecation marker", fields[3].Description)
	}
	if fields[1].Validation != "x-kubernetes-list-type=map; x-kubernetes-list-map-keys=name" {
		t.Fatalf("list validation = %q", fields[1].Validation)
	}
	if fields[4].Validation != "additionalProperties=true; x-kubernetes-map-type=atomic" {
		t.Fatalf("map validation = %q", fields[4].Validation)
	}
	if fields[5].Validation != "minLength=1" {
		t.Fatalf("map value validation = %q", fields[5].Validation)
	}
	if fields[6].Validation != "x-kubernetes-int-or-string" {
		t.Fatalf("int-or-string validation = %q", fields[6].Validation)
	}
}

func TestNewSchemaFieldRecordsRootValidation(t *testing.T) {
	schema := apiextensionsv1.JSONSchemaProps{
		Type: "object",
		XValidations: apiextensionsv1.ValidationRules{{
			Rule:    "has(self.value)",
			Message: "value is required",
		}},
	}

	field := newSchemaField("spec", schema, true, true)
	if !field.SchemaRoot || field.Path != "spec" || !field.Required {
		t.Fatalf("root field = %#v", field)
	}
	if !strings.Contains(field.Validation, `"rule":"has(self.value)"`) {
		t.Fatalf("root validation = %q", field.Validation)
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
				Omission:       "uses the configured default target",
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
	if policy.Omission != "" {
		t.Fatalf("nested policy omission = %q, want exact-path semantics only", policy.Omission)
	}
	exact, _ := resolvePolicy(resource, "spec.backup.target")
	if exact.Omission != "uses the configured default target" {
		t.Fatalf("exact policy omission = %q", exact.Omission)
	}
	if strings.Join(matched, ",") != "spec.backup,spec.backup.target" {
		t.Fatalf("matched rules = %v", matched)
	}
}

func TestResolveResourceRejectsInvalidStableValues(t *testing.T) {
	resource := resourceInventory{
		Kind: "OpenBaoCluster",
		Defaults: resourceDefaults{
			Status: policyDefaults{
				Mutability:        "operator-owned",
				Migration:         "none",
				ModuleInteraction: "none",
				Enforcement:       []string{"status-writer"},
			},
		},
		Rules: []inventoryRule{{
			Path:           "status.conditions",
			Classification: "stable-automation",
			Owner:          "status-controller",
			StableValues:   []string{"Ready", "", "Ready"},
		}},
	}
	fields := []schemaField{{Path: "status.conditions", Type: "array<object>"}}

	_, errs := resolveResource(resource, fields)
	if !containsError(errs, "stableValues must not contain empty values") {
		t.Fatalf("errors = %v, want empty stable value failure", errs)
	}
	if !containsError(errs, `duplicate stable value "Ready"`) {
		t.Fatalf("errors = %v, want duplicate stable value failure", errs)
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
	if report.DecisionStatus != "approved" {
		t.Fatalf("decision status = %q, want approved", report.DecisionStatus)
	}
	if err := verifySnapshot(report.Snapshot, report); err != nil {
		t.Fatalf("verifySnapshot() error = %v", err)
	}
}

func TestRepositoryInventoryTracksProducedConditionTypes(t *testing.T) {
	report := repositoryInventoryReport(t)

	assertStableValues(t, report, "OpenBaoCluster", "status.conditions", []string{
		string(openbaov1alpha1.ConditionACMECacheReady),
		string(openbaov1alpha1.ConditionACMEIntegrationReady),
		string(openbaov1alpha1.ConditionAPIServerNetworkReady),
		string(openbaov1alpha1.ConditionAuditFileStorageReady),
		string(openbaov1alpha1.ConditionAvailable),
		string(openbaov1alpha1.ConditionBackingUp),
		string(openbaov1alpha1.ConditionBackupConfigurationReady),
		string(openbaov1alpha1.ConditionCloudUnsealIdentityReady),
		string(openbaov1alpha1.ConditionDegraded),
		string(openbaov1alpha1.ConditionEtcdEncryptionWarning),
		string(openbaov1alpha1.ConditionGatewayIntegrationReady),
		string(openbaov1alpha1.ConditionIngressIntegrationReady),
		string(openbaov1alpha1.ConditionNodeSecurityCapabilityMismatch),
		string(openbaov1alpha1.ConditionOpenBaoInitialized),
		string(openbaov1alpha1.ConditionOpenBaoLeader),
		string(openbaov1alpha1.ConditionOpenBaoSealed),
		string(openbaov1alpha1.ConditionProductionReady),
		string(openbaov1alpha1.ConditionRaftMembershipReady),
		string(openbaov1alpha1.ConditionReadReplicaStorageConfigured),
		string(openbaov1alpha1.ConditionReadReplicasAutopilotHealthy),
		string(openbaov1alpha1.ConditionReadReplicasReady),
		string(openbaov1alpha1.ConditionReadServingAvailable),
		string(openbaov1alpha1.ConditionSecurityRisk),
		string(openbaov1alpha1.ConditionStorageConfigured),
		string(openbaov1alpha1.ConditionTLSReady),
		string(openbaov1alpha1.ConditionUpgrading),
		string(openbaov1alpha1.ConditionUserAccessBootstrap),
	})
	assertStableValues(t, report, "OpenBaoRestore", "status.conditions", []string{
		constants.ConditionTypeOperationLockOverride,
		constants.RestoreConditionType,
		constants.RestoreConfigurationConditionType,
	})
	assertStableValues(t, report, "OpenBaoTenant", "status.conditions", []string{
		constants.TenantProvisionedConditionType,
	})
}

func repositoryInventoryReport(t *testing.T) inventoryReport {
	t.Helper()
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
	return report
}

func assertStableValues(t *testing.T, report inventoryReport, kind, path string, want []string) {
	t.Helper()
	sort.Strings(want)
	for _, resource := range report.Resources {
		if resource.Kind != kind {
			continue
		}
		for _, field := range resource.Fields {
			if field.Field.Path == path {
				if strings.Join(field.Policy.StableValues, ",") != strings.Join(want, ",") {
					t.Fatalf("%s %s stable values = %v, want %v", kind, path, field.Policy.StableValues, want)
				}
				return
			}
		}
	}
	t.Fatalf("missing %s %s", kind, path)
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
OpenBaoCluster: 3 schema nodes
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
							Omission:          "controller derives the image from version",
							StableValues:      []string{"Ready", "Unavailable"},
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
	if !strings.Contains(out.String(),
		"omission: controller derives the image from version; stable values: Ready,Unavailable") {
		t.Fatalf("snapshot decisions missing: %s", out.String())
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

func int64Ptr(value int64) *int64 {
	return &value
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
