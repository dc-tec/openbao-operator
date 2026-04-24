package openbaoclusterclaim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	// MutatingWebhookPath is the request path used for OpenBaoClusterClaim service-offering mutation.
	MutatingWebhookPath = "/mutate-openbao-org-v1alpha1-openbaoclusterclaim"
)

// ServiceOfferingMutator resolves stable service-offering aliases into pinned immutable
// service-profile revisions before claims are persisted.
type ServiceOfferingMutator struct {
	client                       client.Reader
	decoder                      admission.Decoder
	claimsEnabled                bool
	operatorNamespace            string
	controllerServiceAccountName string
}

// NewServiceOfferingMutator constructs the narrow claim mutator that resolves
// serviceOfferingRef into a pinned serviceProfileRef.
func NewServiceOfferingMutator(
	reader client.Reader,
	scheme *runtime.Scheme,
	claimsEnabled bool,
	operatorNamespace string,
	controllerServiceAccountName string,
) *ServiceOfferingMutator {
	return &ServiceOfferingMutator{
		client:                       reader,
		decoder:                      admission.NewDecoder(scheme),
		claimsEnabled:                claimsEnabled,
		operatorNamespace:            strings.TrimSpace(operatorNamespace),
		controllerServiceAccountName: strings.TrimSpace(controllerServiceAccountName),
	}
}

// Handle mutates claim create and pre-materialization update requests so the stored
// claim spec always carries a pinned immutable service-profile revision.
func (m *ServiceOfferingMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if m == nil || m.client == nil || m.decoder == nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("service-offering mutator is not configured"))
	}

	claim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if err := m.decoder.Decode(req, claim); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	original := claim.DeepCopy()
	oldClaim := &openbaov1alpha1.OpenBaoClusterClaim{}
	if req.Operation == admissionv1.Update && len(req.OldObject.Raw) > 0 {
		if err := m.decoder.DecodeRaw(req.OldObject, oldClaim); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	newOfferingName := localReferenceName(claim.Spec.ServiceOfferingRef)
	newProfileName := strings.TrimSpace(claim.Spec.ServiceProfileRef.Name)
	oldOfferingName := localReferenceName(oldClaim.Spec.ServiceOfferingRef)
	oldProfileName := strings.TrimSpace(oldClaim.Spec.ServiceProfileRef.Name)

	if newOfferingName == "" {
		if newProfileName == "" {
			return admission.Denied("OpenBaoClusterClaim requires either spec.serviceOfferingRef or spec.serviceProfileRef.")
		}
		return admission.Allowed("OpenBaoClusterClaim uses an explicit pinned service-profile revision.")
	}

	if !m.claimsEnabled {
		return admission.Denied("OpenBaoClusterClaim.spec.serviceOfferingRef requires claim handling to be enabled.")
	}

	if req.Operation == admissionv1.Update && oldOfferingName == newOfferingName {
		if oldProfileName == "" {
			return m.pinClaimToOffering(ctx, req, claim, original, newOfferingName)
		}
		if newProfileName == "" {
			claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: oldProfileName}
			return m.patchClaim(req, original, claim, "Restored the pinned service-profile revision for the selected service offering.")
		}
		if newProfileName != oldProfileName {
			if m.isControllerRequest(req) {
				return m.pinClaimToOffering(ctx, req, claim, original, newOfferingName)
			}
			return admission.Denied("OpenBaoClusterClaim.spec.serviceProfileRef is pinned by spec.serviceOfferingRef and may only change when the offering selection changes.")
		}
		return admission.Allowed("OpenBaoClusterClaim retains its pinned service-profile revision for the selected service offering.")
	}

	return m.pinClaimToOffering(ctx, req, claim, original, newOfferingName)
}

func (m *ServiceOfferingMutator) pinClaimToOffering(
	ctx context.Context,
	req admission.Request,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	original *openbaov1alpha1.OpenBaoClusterClaim,
	offeringName string,
) admission.Response {
	revisionName, err := m.resolveOfferingRevision(ctx, offeringName)
	if err != nil {
		return admission.Denied(err.Error())
	}
	currentProfileName := strings.TrimSpace(claim.Spec.ServiceProfileRef.Name)
	if currentProfileName != "" && currentProfileName != revisionName {
		return admission.Denied("OpenBaoClusterClaim selectors disagree: spec.serviceOfferingRef does not resolve to the provided spec.serviceProfileRef.")
	}
	claim.Spec.ServiceProfileRef = openbaov1alpha1.LocalReference{Name: revisionName}
	return m.patchClaim(req, original, claim, "Resolved the stable service offering to its current pinned service-profile revision.")
}

func (m *ServiceOfferingMutator) resolveOfferingRevision(ctx context.Context, offeringName string) (string, error) {
	offering := &openbaov1alpha1.OpenBaoServiceOffering{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: offeringName}, offering); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("referenced OpenBaoServiceOffering %q does not exist", offeringName)
		}
		return "", fmt.Errorf("failed to load OpenBaoServiceOffering %q", offeringName)
	}

	revisionName := strings.TrimSpace(offering.Spec.CurrentRevisionRef.Name)
	if revisionName == "" {
		return "", fmt.Errorf("OpenBaoServiceOffering %q does not define spec.currentRevisionRef.name", offeringName)
	}

	serviceProfile := &openbaov1alpha1.OpenBaoServiceProfile{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: revisionName}, serviceProfile); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("OpenBaoServiceOffering %q points at missing OpenBaoServiceProfile %q", offeringName, revisionName)
		}
		return "", fmt.Errorf("failed to load OpenBaoServiceProfile %q for OpenBaoServiceOffering %q", revisionName, offeringName)
	}

	return revisionName, nil
}

func (m *ServiceOfferingMutator) patchClaim(
	req admission.Request,
	original *openbaov1alpha1.OpenBaoClusterClaim,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	message string,
) admission.Response {
	if original == nil || claim == nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("claim mutation requires both original and mutated objects"))
	}

	originalBytes, err := json.Marshal(original)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	currentBytes, err := json.Marshal(claim)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	if string(originalBytes) == string(currentBytes) {
		return admission.Allowed(message)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, currentBytes).WithWarnings()
}

func localReferenceName(ref *openbaov1alpha1.LocalReference) string {
	if ref == nil {
		return ""
	}
	return strings.TrimSpace(ref.Name)
}

func (m *ServiceOfferingMutator) isControllerRequest(req admission.Request) bool {
	if m == nil || m.operatorNamespace == "" || m.controllerServiceAccountName == "" {
		return false
	}
	return req.UserInfo.Username == fmt.Sprintf(
		"system:serviceaccount:%s:%s",
		m.operatorNamespace,
		m.controllerServiceAccountName,
	)
}
