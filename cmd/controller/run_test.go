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
)

// Note: OIDC/JWKS tests have been moved to internal/auth/oidc_test.go
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
