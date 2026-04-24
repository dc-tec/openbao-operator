package openbaoclusterclaim

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestServiceOfferingMutatorHandle(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name             string
		claimsEnabled    bool
		operation        admissionv1.Operation
		username         string
		claim            *openbaov1alpha1.OpenBaoClusterClaim
		oldClaim         *openbaov1alpha1.OpenBaoClusterClaim
		objects          []runtime.Object
		wantAllowed      bool
		wantPatchPath    string
		wantPatchedValue string
		wantMessage      string
	}

	for _, tt := range []testCase{
		{
			name:          "create offering only pins current revision",
			claimsEnabled: true,
			operation:     admissionv1.Create,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v1"),
				validServiceProfile("standard-ha-v1"),
			},
			wantAllowed:      true,
			wantPatchPath:    "/spec/serviceProfileRef/name",
			wantPatchedValue: "standard-ha-v1",
		},
		{
			name:          "direct pinned profile stays untouched",
			claimsEnabled: true,
			operation:     admissionv1.Create,
			claim:         validClaimForMutation(),
			objects:       nil,
			wantAllowed:   true,
		},
		{
			name:          "claims disabled denies offering selector",
			claimsEnabled: false,
			operation:     admissionv1.Create,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v1"),
				validServiceProfile("standard-ha-v1"),
			},
			wantAllowed: false,
			wantMessage: "requires claim handling to be enabled",
		},
		{
			name:          "update unchanged offering does not live follow promotion",
			claimsEnabled: true,
			operation:     admissionv1.Update,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}
				claim.Labels = map[string]string{"change": "metadata-only"}
				return claim
			}(),
			oldClaim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v2"),
				validServiceProfile("standard-ha-v2"),
			},
			wantAllowed: true,
		},
		{
			name:          "update offering change repins current revision",
			claimsEnabled: true,
			operation:     admissionv1.Update,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-secure"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}
				return claim
			}(),
			oldClaim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-secure", "standard-secure-v3"),
				validServiceProfile("standard-secure-v3"),
			},
			wantAllowed:      true,
			wantPatchPath:    "/spec/serviceProfileRef/name",
			wantPatchedValue: "standard-secure-v3",
		},
		{
			name:          "unchanged offering rejects manual profile drift",
			claimsEnabled: true,
			operation:     admissionv1.Update,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
				return claim
			}(),
			oldClaim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v2"),
				validServiceProfile("standard-ha-v2"),
			},
			wantAllowed: false,
			wantMessage: "may only change when the offering selection changes",
		},
		{
			name:          "controller may repin unchanged offering to current revision",
			claimsEnabled: true,
			operation:     admissionv1.Update,
			username:      "system:serviceaccount:openbao-operator-system:openbao-operator-controller",
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v2"}
				return claim
			}(),
			oldClaim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "standard-ha-v1"}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v2"),
				validServiceProfile("standard-ha-v2"),
			},
			wantAllowed: true,
		},
		{
			name:          "selector mismatch is denied on create",
			claimsEnabled: true,
			operation:     admissionv1.Create,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: "different-v1"}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v1"),
				validServiceProfile("standard-ha-v1"),
			},
			wantAllowed: false,
			wantMessage: "selectors disagree",
		},
		{
			name:          "missing offering revision is denied",
			claimsEnabled: true,
			operation:     admissionv1.Create,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v9"),
			},
			wantAllowed: false,
			wantMessage: "points at missing OpenBaoServiceProfile",
		},
		{
			name:          "multi-cluster only still allows offering selector",
			claimsEnabled: true,
			operation:     admissionv1.Create,
			claim: func() *openbaov1alpha1.OpenBaoClusterClaim {
				claim := validClaimForMutation()
				claim.Spec.ServiceOfferingRef = &openbaov1alpha1.LocalReference{Name: "standard-ha"}
				claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{}
				return claim
			}(),
			objects: []runtime.Object{
				validServiceOffering("standard-ha", "standard-ha-v1"),
				validServiceProfile("standard-ha-v1"),
			},
			wantAllowed:      true,
			wantPatchPath:    "/spec/serviceProfileRef/name",
			wantPatchedValue: "standard-ha-v1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := newMutationScheme(t)
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if len(tt.objects) > 0 {
				builder = builder.WithRuntimeObjects(tt.objects...)
			}
			mutator := NewServiceOfferingMutator(
				builder.Build(),
				scheme,
				tt.claimsEnabled,
				"openbao-operator-system",
				"openbao-operator-controller",
			)

			resp := mutator.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tt.operation,
					UserInfo:  authnv1.UserInfo{Username: tt.username},
					Object:    runtime.RawExtension{Raw: mustMarshalMutationObject(t, tt.claim)},
					OldObject: runtime.RawExtension{Raw: mustMarshalMutationObject(t, tt.oldClaim)},
				},
			})

			if resp.Allowed != tt.wantAllowed {
				t.Fatalf("response allowed = %v, want %v (message=%q)", resp.Allowed, tt.wantAllowed, responseMessage(resp))
			}
			if tt.wantMessage != "" && !contains(responseMessage(resp), tt.wantMessage) {
				t.Fatalf("response message = %q, want substring %q", responseMessage(resp), tt.wantMessage)
			}
			if tt.wantPatchPath == "" {
				if len(resp.Patches) != 0 {
					t.Fatalf("expected no patches, got %#v", resp.Patches)
				}
				return
			}
			if len(resp.Patches) == 0 {
				t.Fatalf("expected patches, got none")
			}
			if resp.Patches[len(resp.Patches)-1].Path != tt.wantPatchPath {
				t.Fatalf("last patch path = %q, want %q; patches=%#v", resp.Patches[len(resp.Patches)-1].Path, tt.wantPatchPath, resp.Patches)
			}
			if got := resp.Patches[len(resp.Patches)-1].Value; got != tt.wantPatchedValue {
				t.Fatalf("last patch value = %#v, want %#v; patches=%#v", got, tt.wantPatchedValue, resp.Patches)
			}
		})
	}
}

func validClaimForMutation() *openbaov1alpha1.OpenBaoClusterClaim {
	return &openbaov1alpha1.OpenBaoClusterClaim{
		ObjectMeta: metav1.ObjectMeta{Namespace: "tenant-a", Name: "payments-bao"},
		Spec: openbaov1alpha1.OpenBaoClusterClaimSpec{
			TenantRef:         openbaov1alpha1.LocalReference{Name: "payments"},
			ServiceProfileRef: openbaov1alpha1.LocalReference{Name: "standard-ha-v1"},
		},
	}
}

func validServiceOffering(name, revision string) *openbaov1alpha1.OpenBaoServiceOffering {
	return &openbaov1alpha1.OpenBaoServiceOffering{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openbaov1alpha1.OpenBaoServiceOfferingSpec{
			CurrentRevisionRef: openbaov1alpha1.LocalReference{Name: revision},
		},
	}
}

func validServiceProfile(name string) *openbaov1alpha1.OpenBaoServiceProfile {
	return &openbaov1alpha1.OpenBaoServiceProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: openbaov1alpha1.OpenBaoServiceProfileSpec{
			Cluster: openbaov1alpha1.OpenBaoServiceProfileClusterSpec{
				Version:         "2.4.4",
				Voters:          3,
				SecurityProfile: openbaov1alpha1.ProfileDevelopment,
			},
			Storage:   openbaov1alpha1.OpenBaoServiceProfileStorageSpec{PrimarySize: "10Gi"},
			Bootstrap: openbaov1alpha1.OpenBaoServiceProfileBootstrapSpec{Mode: openbaov1alpha1.OpenBaoBootstrapModeSelfInit},
			Exposure:  openbaov1alpha1.OpenBaoServiceProfileExposureSpec{ClassRef: openbaov1alpha1.LocalReference{Name: "internal"}},
			Backup:    openbaov1alpha1.OpenBaoServiceProfileBackupSpec{ProfileRef: openbaov1alpha1.LocalReference{Name: "backup"}},
		},
	}
}

func newMutationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	return scheme
}

func mustMarshalMutationObject(t *testing.T, obj any) []byte {
	t.Helper()
	if obj == nil {
		return nil
	}
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("json.Marshal(%T) error = %v", obj, err)
	}
	return data
}

func responseMessage(resp admission.Response) string {
	if resp.Result == nil {
		return ""
	}
	return resp.Result.Message
}

func contains(message, substring string) bool {
	return len(substring) == 0 || strings.Contains(message, substring)
}
