/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/dc-tec/openbao-operator/cmd/controller"
	"github.com/dc-tec/openbao-operator/cmd/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/entrypoint"
)

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return &entrypoint.UsageError{Err: fmt.Errorf("missing command (valid commands: provisioner, controller)")}
	}

	switch command := args[0]; command {
	case "provisioner":
		return provisioner.Run(ctx, args[1:])
	case "controller":
		return controller.Run(ctx, args[1:])
	case "-h", "--help":
		fmt.Fprintln(os.Stderr, "Usage: manager <controller|provisioner> [flags]")
		return flag.ErrHelp
	default:
		return &entrypoint.UsageError{
			Err: fmt.Errorf("unknown command %q (valid commands: provisioner, controller)", command),
		}
	}
}

func exitCode(err error) int {
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	var usageError *entrypoint.UsageError
	if errors.As(err, &usageError) {
		return 2
	}
	return 1
}

func main() {
	err := run(ctrl.SetupSignalHandler(), os.Args[1:])
	code := exitCode(err)
	if code != 0 {
		// Argument and kubeconfig failures can occur before logging is configured.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(code)
	}
}
