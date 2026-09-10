// edge_chart prepares a temporary chart copy for a verified edge candidate.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

type image struct {
	name   string
	prefix string
	key    string
	ref    string
	digest string
}

type config struct {
	chartVersion string
	version      string
	sha          string
	images       []image
}

func main() {
	cfg := config{
		chartVersion: os.Getenv("CHART_VERSION"),
		version:      os.Getenv("VERSION"),
		sha:          os.Getenv("SHA"),
		images: []image{
			{name: "openbao-operator", prefix: "MANAGER"},
			{name: "openbao-init", prefix: "CONFIG_INIT", key: "init"},
			{name: "openbao-backup", prefix: "BACKUP_EXECUTOR", key: "backup"},
			{name: "openbao-upgrade", prefix: "UPGRADE_EXECUTOR", key: "upgrade"},
		},
	}
	for i := range cfg.images {
		cfg.images[i].ref = os.Getenv(cfg.images[i].prefix + "_IMAGE")
		cfg.images[i].digest = os.Getenv(cfg.images[i].prefix + "_DIGEST")
	}
	if err := prepare(os.Getenv("CHART_DIR"), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare(dir string, cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("CHART_DIR must identify a temporary chart copy")
	}
	chart, err := readYAML(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return err
	}
	values, err := readYAML(filepath.Join(dir, "values.yaml"))
	if err != nil {
		return err
	}
	chart["version"] = cfg.chartVersion
	chart["appVersion"] = cfg.version
	annotations := mapping(chart, "annotations")
	// Release changelog and image metadata do not describe this candidate.
	for key := range annotations {
		if strings.HasPrefix(key, "artifacthub.io/") {
			delete(annotations, key)
		}
	}
	annotations["artifacthub.io/prerelease"] = "true"
	annotations["org.opencontainers.image.revision"] = cfg.sha
	annotations["org.opencontainers.image.source"] = "https://github.com/dc-tec/openbao-operator"

	manager := cfg.images[0]
	imageValues := mapping(values, "image")
	imageValues["repository"] = manager.ref
	imageValues["tag"] = cfg.version
	imageValues["digest"] = manager.digest
	values["operatorVersion"] = cfg.version
	helperValues := mapping(values, "helperImages")
	for _, helper := range cfg.images[1:] {
		helperValues[helper.key] = helper.ref + "@" + helper.digest
	}
	if err := writeYAML(filepath.Join(dir, "Chart.yaml"), chart); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, "values.yaml"), values); err != nil {
		return err
	}
	readme := fmt.Sprintf(`# OpenBao Operator edge chart

Evaluation build from commit %s. Chart version: %s.

Install this chart from oci://ghcr.io/dc-tec/charts-edge/openbao-operator using the exact version.
The controller, Provisioner, and default helper images are pinned to the verified candidate digests.
This repository is not registered in Artifact Hub.

See https://dc-tec.github.io/openbao-operator/next/docs/get-started/install/ for installation and CRD upgrade steps.
`, cfg.sha, cfg.chartVersion)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("write edge chart readme: %w", err)
	}
	return nil
}

func (cfg config) validate() error {
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(cfg.sha) {
		return fmt.Errorf("SHA must be a full commit SHA")
	}
	if cfg.version != "edge-"+cfg.sha[:12] {
		return fmt.Errorf("VERSION must identify the edge commit")
	}
	pattern := `^[0-9]+\.[0-9]+\.[0-9]+-edge\.[1-9][0-9]*\.[1-9][0-9]*\.g` + cfg.sha[:12] + `$`
	if !regexp.MustCompile(pattern).MatchString(cfg.chartVersion) {
		return fmt.Errorf("CHART_VERSION must contain the edge run ID, attempt, and commit")
	}
	if len(cfg.images) != 4 {
		return fmt.Errorf("all four candidate images are required")
	}
	for _, image := range cfg.images {
		if !regexp.MustCompile(`^ghcr\.io/[a-z0-9-]+/` + image.name + `$`).MatchString(image.ref) {
			return fmt.Errorf("invalid candidate repository for %s", image.name)
		}
		if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(image.digest) {
			return fmt.Errorf("invalid candidate digest for %s", image.name)
		}
	}
	return nil
}

func mapping(parent map[string]any, key string) map[string]any {
	value, ok := parent[key].(map[string]any)
	if !ok {
		value = map[string]any{}
		parent[key] = value
	}
	return value
}

func readYAML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var value map[string]any
	if err := yaml.UnmarshalStrict(data, &value); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return value, nil
}

func writeYAML(path string, value map[string]any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
