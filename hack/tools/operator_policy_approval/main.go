// Command operator_policy_approval renders the independent OpenBao approval
// policy for a reviewed cluster manifest and this operator source revision.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"sigs.k8s.io/yaml"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
)

func main() {
	manifest := flag.String("cluster", "", "Path to one OpenBaoCluster YAML manifest")
	flag.Parse()
	if err := run(*manifest, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string, output io.Writer) error {
	if path == "" {
		return fmt.Errorf("--cluster is required")
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
