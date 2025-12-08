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

func main() {
	if len(os.Args) < 2 {
		setupLog.Error(nil, "missing command", "valid commands", []string{"provisioner", "controller"})
		os.Exit(1)
	}

	// Shift args so flag parsing works inside sub-functions (e.g., --leader-elect)
	command := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	switch command {
	case "provisioner":
		provisioner.Run()
	case "controller":
		controller.Run()
	default:
		setupLog.Error(nil, "unknown command", "command", command, "valid commands", []string{"provisioner", "controller"})
		os.Exit(1)
	}
}
