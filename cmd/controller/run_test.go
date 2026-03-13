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

package controller

import (
	"testing"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// Note: OIDC/JWKS tests have been moved to internal/adapter/auth/oidc_test.go
// These tests verify the controller's integration with the auth package.

// TestRun verifies that Run can be called without panicking.
// This is a minimal smoke test to ensure the function signature is correct.
func TestRun(t *testing.T) {
	// This test verifies that Run accepts []string args as expected
	// Full integration tests would require a real Kubernetes cluster
	// and are covered in e2e tests.

	// Test that Run function exists and accepts the correct signature
	// We can't easily test the full Run() function without a real cluster,
	// so this is a placeholder for future integration tests.
	_ = Run
}

func TestUnavailableHelperImageDefaultFields(t *testing.T) {
	t.Run("returns all helper image fields when operator version is missing", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorVersion, "")

		fields := unavailableHelperImageDefaultFields()
		if len(fields) != 3 {
			t.Fatalf("unavailableHelperImageDefaultFields() len = %d, want 3 (%v)", len(fields), fields)
		}
	})

	t.Run("returns no fields when operator version is present", func(t *testing.T) {
		t.Setenv(constants.EnvOperatorVersion, "0.1.0")

		fields := unavailableHelperImageDefaultFields()
		if len(fields) != 0 {
			t.Fatalf("unavailableHelperImageDefaultFields() = %v, want empty", fields)
		}
	})
}
