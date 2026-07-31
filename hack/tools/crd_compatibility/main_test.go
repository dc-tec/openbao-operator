package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestCompareSnapshotsClassifiesChanges(t *testing.T) {
	t.Parallel()

	oldChanged := testSchemaNode("spec.changed", "string")
	oldChanged.Default = `"old"`
	oldChanged.Enum = []string{`"one"`, `"two"`}
	oldChanged.Constraints = map[string]string{"minLength": "1", "pattern": "old"}
	oldRelaxed := testSchemaNode("spec.relaxed", "string")
	oldRelaxed.Required = true
	oldRelaxed.Constraints = map[string]string{"maxLength": "10", "nullable": trueValue}
	oldNodes := []schemaNode{
		testSchemaNode("spec.removed", "string"),
		oldChanged,
		oldRelaxed,
		testSchemaNode("spec.nowNullable", "string"),
	}

	newChanged := testSchemaNode("spec.changed", "integer")
	newChanged.Required = true
	newChanged.Default = `"new"`
	newChanged.Enum = []string{`"two"`, `"three"`}
	newChanged.Constraints = map[string]string{"minLength": "2", "pattern": "new"}
	newChanged.CEL = []celRule{{Rule: "self > 0"}}
	newRelaxed := testSchemaNode("spec.relaxed", "string")
	newRelaxed.Constraints = map[string]string{"maxLength": "20"}
	newRequired := testSchemaNode("spec.required", "string")
	newRequired.Required = true
	providerName := testSchemaNode("spec.provider.name", "string")
	providerName.Required = true
	nowNullable := testSchemaNode("spec.nowNullable", "string")
	nowNullable.Constraints = map[string]string{"nullable": trueValue}
	newNodes := []schemaNode{
		newChanged,
		newRelaxed,
		testSchemaNode("spec.added", "string"),
		newRequired,
		testSchemaNode("spec.provider", "object"),
		providerName,
		nowNullable,
	}

	changes := compareSnapshots(oldNodes, newNodes)
	assertChange(t, changes, "breaking", "field-removed", "spec.removed")
	assertChange(t, changes, "breaking", "field-added-required", "spec.required")
	assertChange(t, changes, "breaking", "type-changed", "spec.changed")
	assertChange(t, changes, "breaking", "requiredness-added", "spec.changed")
	assertChange(t, changes, "breaking", "enum-values-removed", "spec.changed")
	assertChange(t, changes, "review", "default-changed", "spec.changed")
	assertChange(t, changes, "review", "validation-tightened", "spec.changed")
	assertChange(t, changes, "review", "validation-changed", "spec.changed")
	assertChange(t, changes, "compatible", "enum-values-added", "spec.changed")
	assertChange(t, changes, "compatible", "requiredness-removed", "spec.relaxed")
	assertChange(t, changes, "compatible", "validation-relaxed", "spec.relaxed")
	assertChange(t, changes, "review", "validation-tightened", "spec.relaxed")
	assertChange(t, changes, "compatible", "validation-relaxed", "spec.nowNullable")
	assertChange(t, changes, "compatible", "field-added-optional", "spec.added")
	assertChange(
		t,
		changes,
		impactCompatible,
		"field-added-under-optional-parent",
		"spec.provider.name",
	)
}

func TestWriteReportMakesReportOnlyModeExplicit(t *testing.T) {
	t.Parallel()

	result := report{
		Baseline: "0.4.2",
		Target:   "0.5.0",
		Mode:     "report",
		Changes: []change{{
			Impact:         "breaking",
			Classification: "field-removed",
			Kind:           "Widget",
			Version:        "v1",
			Path:           "spec.oldField",
			Detail:         "field is absent from the current schema",
		}},
	}

	var output bytes.Buffer
	if err := writeReport(&output, result, "markdown"); err != nil {
		t.Fatalf("write report: %v", err)
	}
	for _, want := range []string{
		"CRD compatibility: 0.4.2 -> 0.5.0 (report-only)",
		"Summary: 1 breaking, 0 review, 0 compatible",
		"| breaking | field-removed | Widget/v1 | `spec.oldField` |",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("report does not contain %q:\n%s", want, output.String())
		}
	}
	if hasBlockingChanges(nil) {
		t.Fatal("an empty report must not block enforcement")
	}
	if !hasBlockingChanges(result.Changes) {
		t.Fatal("a breaking report must block enforcement")
	}
}

func TestMigrationFixturesCoverRequiredManifestChanges(t *testing.T) {
	t.Parallel()

	before := loadMigrationFixture(t, "0.4.2-openbaocluster.yaml")
	after := loadMigrationFixture(t, "0.5.0-openbaocluster.yaml")

	assertNestedString(t, before, "bao.example.com", "spec", "tls", "acme", "domain")
	assertNestedStringSlice(t, after, []string{"bao.example.com"}, "spec", "tls", "acme", "domains")
	assertNestedString(t, before, "2026-08-01T12:00:00Z", "spec", "maintenance", "restartAt")
	assertNestedString(t, after, "2026-08-01T12:00:00Z", "spec", "runtime", "restartAt")
	assertNestedPresence(t, before, true, "spec", "unseal", "awskms")
	assertNestedPresence(t, after, false, "spec", "unseal", "awskms")
	assertNestedPresence(t, before, true, "spec", "backup", "target", "gcs")
	assertNestedPresence(t, after, false, "spec", "backup", "target", "gcs")
}

func assertChange(t *testing.T, changes []change, impact, classification, path string) {
	t.Helper()
	for _, item := range changes {
		if item.Impact == impact && item.Classification == classification && item.Path == path {
			return
		}
	}
	t.Errorf("missing %s/%s change for %s; got %#v", impact, classification, path, changes)
}

func loadMigrationFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "api-migration", name))
	if err != nil {
		t.Fatalf("read migration fixture %s: %v", name, err)
	}
	data, err = utilyaml.ToJSON(data)
	if err != nil {
		t.Fatalf("convert migration fixture %s to JSON: %v", name, err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("decode migration fixture %s: %v", name, err)
	}
	return object
}

func assertNestedString(t *testing.T, object map[string]any, want string, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || got != want {
		t.Errorf("field %s = %q, found %t, err %v; want %q", strings.Join(fields, "."), got, found, err, want)
	}
}

func assertNestedStringSlice(t *testing.T, object map[string]any, want []string, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedStringSlice(object, fields...)
	if err != nil || !found || strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("field %s = %v, found %t, err %v; want %v", strings.Join(fields, "."), got, found, err, want)
	}
}

func assertNestedPresence(t *testing.T, object map[string]any, want bool, fields ...string) {
	t.Helper()
	_, found, err := unstructured.NestedFieldNoCopy(object, fields...)
	if err != nil || found != want {
		t.Errorf("field %s presence = %t, err %v; want %t", strings.Join(fields, "."), found, err, want)
	}
}

func testSchemaNode(path, schemaType string) schemaNode {
	return schemaNode{
		CRD:     "widgets.example.com",
		Kind:    "Widget",
		Version: "v1",
		Path:    path,
		Type:    schemaType,
	}
}
