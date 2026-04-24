// Copyright 2026.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package claimcontract

import (
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestEvaluateSameClusterAdoptionCompatibility(t *testing.T) {
	t.Parallel()

	claim := validRenderedDevelopmentClaimFixture()
	desired := mustDesiredSameClusterCluster(t, claim, mustRenderSameClusterExecutionContract(
		t,
		claim,
		validRenderedDevelopmentCatalogBundleFixture(),
		SameClusterTransitUnsealDefaults{},
	))

	result := EvaluateSameClusterAdoptionCompatibility(desired.DeepCopy(), desired)
	if !result.Compatible || len(result.Issues) != 0 {
		t.Fatalf("EvaluateSameClusterAdoptionCompatibility() = %#v, want compatible", result)
	}

	existing := desired.DeepCopy()
	existing.Spec.Storage.Size = "50Gi"
	existing.Spec.Audit = []openbaov1alpha1.AuditDevice{{Type: "file", Path: "stdout"}}

	result = EvaluateSameClusterAdoptionCompatibility(existing, desired)
	if result.Compatible {
		t.Fatalf("EvaluateSameClusterAdoptionCompatibility() = %#v, want incompatible", result)
	}
	if !hasAdoptionIssue(result, "spec.storage") {
		t.Fatalf("issues = %#v, want spec.storage mismatch", result.Issues)
	}
	if !hasAdoptionIssue(result, "spec.audit") {
		t.Fatalf("issues = %#v, want spec.audit mismatch for intentionally unmanaged field", result.Issues)
	}
}

func hasAdoptionIssue(result AdoptionCompatibilityResult, field string) bool {
	for _, issue := range result.Issues {
		if issue.Field == field {
			return true
		}
	}
	return false
}
