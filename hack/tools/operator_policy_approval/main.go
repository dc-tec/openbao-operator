// Command operator_policy_approval renders the independent OpenBao approval
// policy for a reviewed cluster manifest and this operator source revision.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
)

func main() {
	manifest := flag.String("cluster", "", "Path to one OpenBaoCluster YAML manifest")
	bundlesDir := flag.String("bundles-dir", "", "Write the immutable chart bundles to this directory")
	check := flag.Bool("check", false, "Verify existing bundles without writing files")
	flag.Parse()
	var err error
	if *bundlesDir != "" && *manifest == "" {
		err = writeBundles(*bundlesDir, *check)
	} else if *bundlesDir == "" && *manifest != "" && !*check {
		err = run(*manifest)
	} else {
		err = fmt.Errorf("choose either --cluster or --bundles-dir (optionally with --check)")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeBundles(directory string, check bool) error {
	for _, strategy := range []struct {
		name  string
		value openbaov1alpha1.UpdateStrategyType
	}{{"rolling-update", "RollingUpdate"}, {"blue-green", "BlueGreen"}} {
		for _, backup := range []bool{false, true} {
			cluster := &openbaov1alpha1.OpenBaoCluster{}
			cluster.Spec.Upgrade = &openbaov1alpha1.UpgradeConfig{Strategy: strategy.value}
			name := strategy.name
			if backup {
				cluster.Spec.Backup = &openbaov1alpha1.BackupSchedule{}
				name += "-backup"
			}
			path := filepath.Join(directory, configbuilder.OperatorPolicyBundleRevision, name+".hcl")
			desired := configbuilder.OperatorPolicyApproval(cluster)
			stored, err := os.ReadFile(path)
			if err == nil {
				if string(stored) != desired {
					return fmt.Errorf("bundle %s is immutable; advance OperatorPolicyBundleRevision and retain old bundles", path)
				}
				continue
			}
			if !errors.Is(err, os.ErrNotExist) || check {
				return fmt.Errorf("read bundle: %w", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create bundle directory: %w", err)
			}
			if err := os.WriteFile(path, []byte(desired), 0o644); err != nil {
				return fmt.Errorf("write bundle: %w", err)
			}
		}
	}
	return nil
}

func run(path string) error {
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
	_, err = fmt.Fprint(os.Stdout, configbuilder.OperatorPolicyApproval(&cluster))
	return err
}
