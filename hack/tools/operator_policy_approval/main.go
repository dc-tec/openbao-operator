// Command operator_policy_approval renders administrator approval from the
// built-in policies for a cluster manifest or the two release configurations.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
)

func main() {
	manifest := flag.String("cluster", "", "Path to one OpenBaoCluster YAML manifest")
	outputDir := flag.String("output-dir", "", "Write both release approval files instead of reading a manifest")
	flag.Parse()
	if err := run(*manifest, *outputDir, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, outputDir string, output io.Writer) error {
	if outputDir != "" {
		if path != "" {
			return fmt.Errorf("--cluster and --output-dir are mutually exclusive")
		}
		return writeReleasePolicies(outputDir)
	}
	if path == "" {
		return fmt.Errorf("--cluster or --output-dir is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read cluster manifest: %w", err)
	}
	var cluster openbaov1alpha1.OpenBaoCluster
	if err := yaml.UnmarshalStrict(data, &cluster); err != nil {
		return fmt.Errorf("decode cluster manifest: %w", err)
	}
	if cluster.Kind != "OpenBaoCluster" {
		return fmt.Errorf("an OpenBaoCluster manifest is required")
	}
	_, err = fmt.Fprint(output, configbuilder.OperatorPolicyApproval(&cluster))
	return err
}

func writeReleasePolicies(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cluster := &openbaov1alpha1.OpenBaoCluster{}
	for _, name := range []string{"operator-policy-approval.hcl", "operator-policy-approval-with-backup.hcl"} {
		contents := []byte(configbuilder.OperatorPolicyApproval(cluster))
		if err := os.WriteFile(filepath.Join(dir, name), contents, 0o644); err != nil {
			return err
		}
		cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
	}
	return nil
}
