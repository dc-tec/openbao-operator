package admission

import (
	"context"
	"strings"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func FuzzDefaultNamePrefixes(f *testing.F) {
	seeds := []string{
		"",
		"demo",
		"demo-",
		"openbao-operator-",
		"custom/prefix",
		"UPPERCASE",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, envPrefix string) {
		if len(envPrefix) > 256 {
			t.Skip()
		}
		t.Setenv("OPERATOR_NAME_PREFIX", sanitizeAdmissionEnvValue(envPrefix, ""))

		got := DefaultNamePrefixes()
		if len(got) == 0 {
			t.Fatal("expected at least one prefix")
		}
		seen := map[string]struct{}{}
		for _, prefix := range got {
			if _, ok := seen[prefix]; ok {
				t.Fatalf("duplicate prefix %q", prefix)
			}
			seen[prefix] = struct{}{}
		}
	})
}

func FuzzStatusSummaryMessage(f *testing.F) {
	f.Add("dep-a", "missing binding", false, false)
	f.Add("dep-b", "", false, true)
	f.Add("", "", true, false)

	f.Fuzz(func(t *testing.T, depName, issue string, ready, overallReady bool) {
		depName = sanitizeAdmissionSummaryName(depName, "")
		issue = sanitizeAdmissionSummaryText(issue)

		status := Status{
			OverallReady: overallReady,
			Dependencies: []DependencyStatus{
				{
					Dependency: Dependency{Name: depName},
					Ready:      ready,
					Issues:     []string{issue},
				},
			},
		}

		_ = status.SummaryMessage()
	})
}

func FuzzCheckDependencies(f *testing.F) {
	f.Add("dep", "policy", "binding", "p-", true, true, true, true)
	f.Add("dep", "policy", "binding", "", true, false, true, false)
	f.Add("dep", "policy", "binding", "openbao-operator-", false, true, false, true)

	f.Fuzz(func(t *testing.T, depName, policyName, bindingName, prefix string, createBinding, createPolicy, denyAction, failPolicy bool) {
		if len(depName) > 128 || len(policyName) > 128 || len(bindingName) > 128 || len(prefix) > 128 {
			t.Skip()
		}

		depName = sanitizeAdmissionName(depName, "dep")
		policyName = sanitizeAdmissionName(policyName, "policy")
		bindingName = sanitizeAdmissionName(bindingName, "binding")

		fullPolicyName := prefix + policyName
		fullBindingName := prefix + bindingName

		var objs []client.Object
		if createBinding {
			actions := []admissionregistrationv1.ValidationAction{admissionregistrationv1.Warn}
			if denyAction {
				actions = []admissionregistrationv1.ValidationAction{admissionregistrationv1.Deny}
			}
			objs = append(objs, newBinding(fullBindingName, fullPolicyName, actions...))
		}
		if createPolicy {
			policyMode := ptrFailurePolicy(admissionregistrationv1.Ignore)
			if failPolicy {
				policyMode = ptrFailurePolicy(admissionregistrationv1.Fail)
			}
			objs = append(objs, newPolicy(fullPolicyName, policyMode))
		}

		reader := newAdmissionClient(t, objs...)
		_, _ = CheckDependencies(
			context.Background(),
			reader,
			[]Dependency{{Name: depName, PolicyName: policyName, BindingName: bindingName}},
			[]string{prefix, ""},
		)
	})
}

func sanitizeAdmissionName(v, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	out := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		ch := v[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			out = append(out, ch)
		case ch >= 'A' && ch <= 'Z':
			out = append(out, ch+('a'-'A'))
		case ch >= '0' && ch <= '9':
			out = append(out, ch)
		case ch == '-':
			out = append(out, ch)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return string(out)
}

func sanitizeAdmissionEnvValue(v, fallback string) string {
	if v == "" {
		return fallback
	}
	out := make([]byte, 0, len(v))
	for i := 0; i < len(v); i++ {
		if v[i] == 0 {
			continue
		}
		out = append(out, v[i])
	}
	if len(out) == 0 {
		return fallback
	}
	return string(out)
}

func sanitizeAdmissionSummaryName(v, fallback string) string {
	v = sanitizeAdmissionName(v, fallback)
	if len(v) > 64 {
		return v[:64]
	}
	return v
}

func sanitizeAdmissionSummaryText(v string) string {
	v = strings.TrimSpace(strings.ReplaceAll(v, "\x00", ""))
	if len(v) > 256 {
		return v[:256]
	}
	return v
}
