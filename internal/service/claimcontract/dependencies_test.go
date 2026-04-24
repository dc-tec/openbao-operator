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

func TestAppliedRenderedDependencies(t *testing.T) {
	t.Parallel()

	rendered := validRenderedDependencyContractFixture()
	status := AppliedRenderedDependencies(rendered)
	if status == nil {
		t.Fatal("AppliedRenderedDependencies() = nil, want dependency status")
	}
	if status.EntrypointRef == nil || status.EntrypointRef.UID != "entrypoint-uid" {
		t.Fatalf("unexpected entrypoint ref: %#v", status.EntrypointRef)
	}
	if status.IngressPolicyRef == nil || status.IngressPolicyRef.UID != "ingress-policy-uid" {
		t.Fatalf("unexpected ingress policy ref: %#v", status.IngressPolicyRef)
	}
	if status.BackupTargetRef == nil || status.BackupTargetRef.UID != "backup-target-uid" {
		t.Fatalf("unexpected backup target ref: %#v", status.BackupTargetRef)
	}
	if status.BackupBackendRef == nil || status.BackupBackendRef.UID != "backup-backend-uid" {
		t.Fatalf("unexpected backup backend ref: %#v", status.BackupBackendRef)
	}
	if status.BackupAuthProfileRef == nil || status.BackupAuthProfileRef.UID != "backup-auth-uid" {
		t.Fatalf("unexpected backup auth ref: %#v", status.BackupAuthProfileRef)
	}
	if status.TransferProfileRef == nil || status.TransferProfileRef.UID != "transfer-profile-uid" {
		t.Fatalf("unexpected transfer profile ref: %#v", status.TransferProfileRef)
	}
	if status.BootstrapProjectionIdentity == nil || status.BootstrapProjectionIdentity.IdentityHash == "" {
		t.Fatalf("unexpected bootstrap projection identity: %#v", status.BootstrapProjectionIdentity)
	}
	if status.Identity == nil || status.Identity.IdentityHash == "" {
		t.Fatalf("unexpected dependency identity: %#v", status.Identity)
	}
}

func TestValidateRenderedDependencyContinuity(t *testing.T) {
	t.Parallel()

	rendered := validRenderedDependencyContractFixture()
	applied := openbaov1alpha1.OpenBaoClusterClaimAppliedStatus{
		RenderedDependencies: AppliedRenderedDependencies(rendered),
	}
	if result := ValidateRenderedDependencyContinuity(applied, rendered); !result.Valid {
		t.Fatalf("ValidateRenderedDependencyContinuity() = %#v, want valid", result)
	}

	drifted := validRenderedDependencyContractFixture()
	drifted.Exposure.Ingress.PolicyRef = &RenderedBoundReference{Name: "nginx-backend-tls-v1", UID: "different-ingress-policy-uid"}
	result := ValidateRenderedDependencyContinuity(applied, drifted)
	if result.Valid {
		t.Fatalf("ValidateRenderedDependencyContinuity() = %#v, want invalid", result)
	}
	if result.Reason != openbaov1alpha1.ReasonInvalid {
		t.Fatalf("ValidateRenderedDependencyContinuity() reason = %q, want %q", result.Reason, openbaov1alpha1.ReasonInvalid)
	}

	renamed := validRenderedDependencyContractFixture()
	renamed.Exposure.Entrypoint.Ref = &RenderedBoundReference{Name: "internal-gateway-v2", UID: "different-entrypoint-uid"}
	if result = ValidateRenderedDependencyContinuity(applied, renamed); !result.Valid {
		t.Fatalf("ValidateRenderedDependencyContinuity() = %#v, want valid when dependency name changes", result)
	}

	driftedBootstrap := validRenderedDependencyContractFixture()
	driftedBootstrap.Bootstrap.Policies.Bundles[0].ContentFromRef = openbaov1alpha1.TypedObjectReference{
		Kind: "Secret",
		Name: "bootstrap-policy-v2",
	}
	result = ValidateRenderedDependencyContinuity(applied, driftedBootstrap)
	if result.Valid {
		t.Fatalf("ValidateRenderedDependencyContinuity() = %#v, want invalid bootstrap projection continuity", result)
	}
	if result.Reason != openbaov1alpha1.ReasonInvalid {
		t.Fatalf("ValidateRenderedDependencyContinuity() reason = %q, want %q", result.Reason, openbaov1alpha1.ReasonInvalid)
	}
}

func validRenderedDependencyContractFixture() *RenderedExecutionContract {
	return &RenderedExecutionContract{
		Exposure: RenderedExposure{
			Entrypoint: &RenderedExposureEntrypoint{
				Ref: &RenderedBoundReference{Name: "internal-gateway-v1", UID: "entrypoint-uid"},
			},
			Ingress: &RenderedExposureIngress{
				PolicyRef: &RenderedBoundReference{Name: "nginx-backend-tls-v1", UID: "ingress-policy-uid"},
			},
		},
		Backup: RenderedBackup{
			TargetRef:          &RenderedBoundReference{Name: "primary-object-backup-v1", UID: "backup-target-uid"},
			BackendRef:         &RenderedBoundReference{Name: "s3-primary-v1", UID: "backup-backend-uid"},
			AuthProfileRef:     &RenderedBoundReference{Name: "aws-irsa-backup-v1", UID: "backup-auth-uid"},
			TransferProfileRef: &RenderedBoundReference{Name: "multipart-standard-v1", UID: "transfer-profile-uid"},
		},
		Bootstrap: RenderedBootstrap{
			Auth: &RenderedBootstrapAuthSpec{
				Methods: []RenderedBootstrapAuthMethodSpec{{
					Type: "jwt",
					Path: "jwt-operator",
					ConfigFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "bootstrap-jwt-config-v1",
					},
				}},
			},
			Policies: &RenderedBootstrapPoliciesSpec{
				Bundles: []RenderedBootstrapPolicyBundleSpec{{
					Name: "bootstrap-policy",
					ContentFromRef: openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "bootstrap-policy-v1",
					},
				}},
			},
			Audit: &RenderedBootstrapAuditSpec{
				Devices: []RenderedBootstrapAuditDeviceSpec{{
					Type: "http",
					Path: "sys/audit/http",
					SinkFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "bootstrap-audit-sink-v1",
					},
				}},
			},
		},
	}
}
