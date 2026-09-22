// Command operator_policy_approval renders the independent OpenBao approval
// policy for a reviewed cluster manifest and this operator source revision.
package main

import (
	"flag"
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	configbuilder "github.com/dc-tec/openbao-operator/internal/adapter/config"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

func main() {
	manifest := flag.String("cluster", "", "Path to one OpenBaoCluster YAML manifest")
	flag.Parse()
	if err := run(*manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
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
	if cluster.Kind != "OpenBaoCluster" || !portauth.OperatorJWTBootstrapEnabled(&cluster) {
		return fmt.Errorf("an OpenBaoCluster with selfInit.enabled and selfInit.oidc.enabled is required")
	}
	_, err = fmt.Fprint(os.Stdout, configbuilder.OperatorPolicyApproval(&cluster))
	return err
}
