package admission

import (
	"context"
	"fmt"
	"strings"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Dependency identifies a required ValidatingAdmissionPolicy and its Binding.
type Dependency struct {
	// Name is a stable identifier for logs and status messages.
	Name string
	// PolicyName is the base metadata.name of the ValidatingAdmissionPolicy in manifests.
	PolicyName string
	// BindingName is the base metadata.name of the ValidatingAdmissionPolicyBinding in manifests.
	BindingName string
}

// DependencyStatus contains the evaluation result for a single Dependency.
type DependencyStatus struct {
	Dependency Dependency
	Ready      bool
	Issues     []string
}

// Status summarizes admission dependency readiness.
type Status struct {
	CheckedAt     time.Time
	Dependencies  []DependencyStatus
	OverallReady  bool
	SentinelReady bool
}

const (
	dependencySentinelMutations    = "sentinel-mutations"
	dependencyProvisionerDelegate  = "provisioner-delegate"
	dependencyManagedResourceLocks = "managed-resource-locks"
)

// DefaultDependencies returns the admission dependencies treated as release-critical.
func DefaultDependencies() []Dependency {
	return []Dependency{
		{
			Name:        dependencySentinelMutations,
			PolicyName:  "openbao-restrict-sentinel-mutations",
			BindingName: "openbao-restrict-sentinel-mutations",
		},
		{
			Name:        "validate-openbaocluster",
			PolicyName:  "validate-openbaocluster",
			BindingName: "validate-openbaocluster",
		},
		{
			Name:        "lock-controller-statefulset-mutations",
			PolicyName:  "lock-controller-statefulset-mutations",
			BindingName: "lock-controller-statefulset-mutations",
		},
		{
			Name:        dependencyProvisionerDelegate,
			PolicyName:  "openbao-restrict-provisioner-delegate",
			BindingName: "openbao-restrict-provisioner-delegate-binding",
		},
		{
			Name:        dependencyManagedResourceLocks,
			PolicyName:  "lock-managed-resource-mutations",
			BindingName: "lock-managed-resource-mutations",
		},
	}
}

// CheckDependencies validates that the required ValidatingAdmissionPolicy and
// ValidatingAdmissionPolicyBinding objects exist and are configured to enforce denies.
//
// namePrefixes are tried in order when resolving object names (for example,
// "openbao-operator-" and "").
func CheckDependencies(ctx context.Context, c client.Reader, deps []Dependency, namePrefixes []string) (Status, error) {
	if ctx == nil {
		return Status{}, fmt.Errorf("context is required")
	}
	if c == nil {
		return Status{}, fmt.Errorf("kubernetes client reader is required")
	}
	if len(deps) == 0 {
		return Status{}, fmt.Errorf("at least one dependency is required")
	}
	if len(namePrefixes) == 0 {
		return Status{}, fmt.Errorf("at least one name prefix is required")
	}

	status := Status{
		CheckedAt: time.Now(),
	}

	var overallReady = true
	var sentinelReady = true

	for _, dep := range deps {
		depStatus := checkDependency(ctx, c, dep, namePrefixes)
		status.Dependencies = append(status.Dependencies, depStatus)
		if !depStatus.Ready {
			overallReady = false
			if dep.Name == dependencySentinelMutations {
				sentinelReady = false
			}
		}
	}

	status.OverallReady = overallReady
	status.SentinelReady = sentinelReady

	return status, nil
}

// SummaryMessage returns a single-line summary for logs, Events, or Conditions.
// It is safe to log (contains no sensitive data).
func (s Status) SummaryMessage() string {
	if s.OverallReady {
		return "Required admission policies are installed and correctly bound"
	}

	parts := make([]string, 0, len(s.Dependencies))
	for _, dep := range s.Dependencies {
		if dep.Ready {
			continue
		}
		if len(dep.Issues) == 0 {
			parts = append(parts, fmt.Sprintf("%s: not ready", dep.Dependency.Name))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", dep.Dependency.Name, strings.Join(dep.Issues, "; ")))
	}
	if len(parts) == 0 {
		return "Admission policies are not ready"
	}
	return "Admission policies are not ready: " + strings.Join(parts, " | ")
}

func checkDependency(ctx context.Context, c client.Reader, dep Dependency, namePrefixes []string) DependencyStatus {
	depStatus := DependencyStatus{
		Dependency: dep,
		Ready:      true,
	}

	policyNameCandidates := buildNameCandidates(dep.PolicyName, namePrefixes)
	bindingNameCandidates := buildNameCandidates(dep.BindingName, namePrefixes)

	binding, bindingName, err := getFirstFoundBinding(ctx, c, bindingNameCandidates)
	if err != nil {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("failed to read ValidatingAdmissionPolicyBinding: %v", err))
		return depStatus
	}
	if binding == nil {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("missing ValidatingAdmissionPolicyBinding (%s)", strings.Join(bindingNameCandidates, " or ")))
		return depStatus
	}

	policyName := binding.Spec.PolicyName
	if policyName == "" {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("binding %q has empty spec.policyName", bindingName))
		return depStatus
	}

	if !containsString(policyNameCandidates, policyName) {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("binding %q references unexpected policy %q", bindingName, policyName))
	}

	policy := &admissionregistrationv1.ValidatingAdmissionPolicy{}
	if err := c.Get(ctx, client.ObjectKey{Name: policyName}, policy); err != nil {
		if apierrors.IsNotFound(err) {
			depStatus.Ready = false
			depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("binding %q references missing policy %q", bindingName, policyName))
			return depStatus
		}
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("failed to read ValidatingAdmissionPolicy %q: %v", policyName, err))
		return depStatus
	}

	if policy.Spec.FailurePolicy == nil || *policy.Spec.FailurePolicy != admissionregistrationv1.Fail {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("policy %q must have failurePolicy=Fail", policyName))
	}

	if !bindingDenies(binding) {
		depStatus.Ready = false
		depStatus.Issues = append(depStatus.Issues, fmt.Sprintf("binding %q must include validationActions=Deny", bindingName))
	}

	return depStatus
}

func buildNameCandidates(base string, namePrefixes []string) []string {
	candidates := make([]string, 0, len(namePrefixes))
	for _, prefix := range namePrefixes {
		candidates = append(candidates, prefix+base)
	}
	return candidates
}

func getFirstFoundBinding(ctx context.Context, c client.Reader, candidates []string) (*admissionregistrationv1.ValidatingAdmissionPolicyBinding, string, error) {
	for _, name := range candidates {
		binding := &admissionregistrationv1.ValidatingAdmissionPolicyBinding{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, binding); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return nil, "", err
		}
		return binding, name, nil
	}
	return nil, "", nil
}

func bindingDenies(binding *admissionregistrationv1.ValidatingAdmissionPolicyBinding) bool {
	if binding == nil {
		return false
	}
	for _, action := range binding.Spec.ValidationActions {
		if action == admissionregistrationv1.Deny {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
