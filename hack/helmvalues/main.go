// Package main verifies that the Helm chart values, schema, and templates stay in sync.
//
// This tool is intentionally conservative:
// - Every `.Values.*` reference used by templates must exist in both `values.yaml` and `values.schema.json`.
// - `values.yaml` must not contain keys disallowed by the schema (respecting JSON schema additionalPropertiess).
//
// This helps prevent Helm chart drift where templates reference keys that are undocumented or invalid.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type options struct {
	chartDir   string
	valuesPath string
	schemaPath string
}

type jsonSchema struct {
	Ref                  string                `json:"$ref"`
	Type                 string                `json:"type"`
	Properties           map[string]*jsonSchema `json:"properties"`
	AdditionalProperties any                   `json:"additionalProperties"`
	Defs                 map[string]*jsonSchema `json:"$defs"`
}

func main() {
	var opts options
	flag.StringVar(&opts.chartDir, "chart-dir", "charts/openbao-operator", "Path to Helm chart root directory")
	flag.StringVar(&opts.valuesPath, "values", "", "Path to values.yaml (defaults to <chart-dir>/values.yaml)")
	flag.StringVar(
		&opts.schemaPath,
		"schema",
		"",
		"Path to values.schema.json (defaults to <chart-dir>/values.schema.json)",
	)
	flag.Parse()

	if opts.valuesPath == "" {
		opts.valuesPath = filepath.Join(opts.chartDir, "values.yaml")
	}
	if opts.schemaPath == "" {
		opts.schemaPath = filepath.Join(opts.chartDir, "values.schema.json")
	}

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(opts options) error {
	valuesAny, err := readYAML(opts.valuesPath)
	if err != nil {
		return fmt.Errorf("read values.yaml: %w", err)
	}

	schemaRoot, err := readSchema(opts.schemaPath)
	if err != nil {
		return fmt.Errorf("read values.schema.json: %w", err)
	}

	templatesDir := filepath.Join(opts.chartDir, "templates")
	valueRefs, err := findValuesReferences(templatesDir)
	if err != nil {
		return fmt.Errorf("scan templates: %w", err)
	}

	var missingInValues []string
	var missingInSchema []string
	for _, ref := range valueRefs {
		path := strings.Split(ref, ".")
		if !yamlPathExists(valuesAny, path) {
			missingInValues = append(missingInValues, ref)
		}
		if !schemaPathExists(schemaRoot, schemaRoot, path) {
			missingInSchema = append(missingInSchema, ref)
		}
	}

	if len(missingInValues) > 0 || len(missingInSchema) > 0 {
		var b strings.Builder
		if len(missingInValues) > 0 {
			sort.Strings(missingInValues)
			fmt.Fprintf(&b, "values.yaml is missing keys referenced by templates:\n")
			for _, k := range missingInValues {
				fmt.Fprintf(&b, "  - %s\n", k)
			}
		}
		if len(missingInSchema) > 0 {
			sort.Strings(missingInSchema)
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			fmt.Fprintf(&b, "values.schema.json is missing keys referenced by templates:\n")
			for _, k := range missingInSchema {
				fmt.Fprintf(&b, "  - %s\n", k)
			}
		}
		return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}

	unknown, err := findValuesKeysDisallowedBySchema(valuesAny, schemaRoot, schemaRoot, nil)
	if err != nil {
		return err
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		var b strings.Builder
		fmt.Fprintf(&b, "values.yaml contains keys disallowed by values.schema.json:\n")
		for _, k := range unknown {
			fmt.Fprintf(&b, "  - %s\n", k)
		}
		return fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}

	return nil
}

func readYAML(path string) (any, error) {
	// #nosec G304 -- local repository path.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var v any
	if err := yaml.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func readSchema(path string) (*jsonSchema, error) {
	// #nosec G304 -- local repository path.
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var root jsonSchema
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

func findValuesReferences(templatesDir string) ([]string, error) {
	re := regexp.MustCompile(`\.Values\.([A-Za-z0-9_][A-Za-z0-9_\-]*(?:\.[A-Za-z0-9_][A-Za-z0-9_\-]*)*)`)
	refs := make(map[string]struct{})

	err := filepath.WalkDir(templatesDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		// Only scan typical Helm template file types.
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" && ext != ".tpl" && ext != ".txt" {
			return nil
		}

		// #nosec G304 -- local repository path.
		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		matches := re.FindAllStringSubmatch(string(data), -1)
		for _, m := range matches {
			if len(m) != 2 {
				continue
			}
			refs[m[1]] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := make([]string, 0, len(refs))
	for r := range refs {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

func yamlPathExists(v any, path []string) bool {
	_, ok := yamlGet(v, path)
	return ok
}

func yamlGet(v any, path []string) (any, bool) {
	cur := v
	for _, seg := range path {
		m, ok := asStringMap(cur)
		if !ok {
			return nil, false
		}
		next, exists := m[seg]
		if !exists {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func asStringMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, vv := range m {
			ks, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[ks] = vv
		}
		return out, true
	default:
		return nil, false
	}
}

func schemaPathExists(root, node *jsonSchema, path []string) bool {
	cur := node
	for _, seg := range path {
		cur = resolveRef(root, cur)
		if cur == nil || cur.Properties == nil {
			return false
		}
		next := cur.Properties[seg]
		if next == nil {
			return false
		}
		cur = next
	}
	return true
}

func resolveRef(root, node *jsonSchema) *jsonSchema {
	if node == nil || node.Ref == "" {
		return node
	}
	if !strings.HasPrefix(node.Ref, "#/") {
		return node
	}

	// Minimal JSON pointer resolver for refs used in this repo.
	// Today we only rely on refs into root $defs.
	// Example: "#/$defs/resources"
	ref := strings.TrimPrefix(node.Ref, "#/")
	parts := strings.Split(ref, "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], "~1", "/")
		parts[i] = strings.ReplaceAll(parts[i], "~0", "~")
	}
	if len(parts) == 2 && parts[0] == "$defs" {
		if root != nil && root.Defs != nil {
			return root.Defs[parts[1]]
		}
		return nil
	}

	// Fallback: unsupported ref shape; treat the ref as opaque.
	return node
}

func findValuesKeysDisallowedBySchema(
	values any,
	schemaRoot, schemaNode *jsonSchema,
	prefix []string,
) ([]string, error) {
	schemaNode = resolveRef(schemaRoot, schemaNode)
	m, ok := asStringMap(values)
	if !ok {
		// Only validate objects/maps. Arrays/scalars are validated by the schema via Helm.
		return nil, nil
	}

	props := map[string]*jsonSchema(nil)
	additional := any(nil)
	if schemaNode != nil {
		props = schemaNode.Properties
		additional = schemaNode.AdditionalProperties
	}

	var unknown []string
	for k, v := range m {
		child := (*jsonSchema)(nil)
		if props != nil {
			child = props[k]
		}

		if child == nil {
			allowed, additionalSchema := additionalPropertiesAllows(additional)
			if !allowed {
				unknown = append(unknown, strings.Join(append(prefix, k), "."))
				continue
			}
			if additionalSchema == nil {
				continue
			}
			// If additionalProperties is a schema object, validate nested object keys recursively.
			nestedUnknown, err := findValuesKeysDisallowedBySchema(v, schemaRoot, additionalSchema, append(prefix, k))
			if err != nil {
				return nil, err
			}
			unknown = append(unknown, nestedUnknown...)
			continue
		}

		nestedUnknown, err := findValuesKeysDisallowedBySchema(v, schemaRoot, child, append(prefix, k))
		if err != nil {
			return nil, err
		}
		unknown = append(unknown, nestedUnknown...)
	}

	return unknown, nil
}

func additionalPropertiesAllows(v any) (bool, *jsonSchema) {
	if v == nil {
		// JSON schema default is additionalProperties=true when omitted.
		return true, nil
	}
	switch vv := v.(type) {
	case bool:
		return vv, nil
	case map[string]any:
		// When unmarshalling into `any`, nested schema objects appear as map[string]any.
		// Re-marshal into jsonSchema so we can recurse.
		data, err := json.Marshal(vv)
		if err != nil {
			return true, nil
		}
		var s jsonSchema
		if err := json.Unmarshal(data, &s); err != nil {
			return true, nil
		}
		return true, &s
	default:
		return true, nil
	}
}
