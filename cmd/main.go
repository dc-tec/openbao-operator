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
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openbao/operator/cmd/controller"
	"github.com/openbao/operator/cmd/provisioner"
)

var (
	setupLog = ctrl.Log.WithName("setup")
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("missing command (valid commands: provisioner, controller)")
	}

	// Shift args so flag parsing works inside sub-functions (e.g., --leader-elect)
	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "provisioner":
		provisioner.Run(args)
	case "controller":
		controller.Run(args)
	default:
		return fmt.Errorf("unknown command %q (valid commands: provisioner, controller)", command)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		setupLog.Error(err, "command failed")
		os.Exit(1)
	}
}
