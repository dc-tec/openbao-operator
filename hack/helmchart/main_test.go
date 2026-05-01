package main

import (
	"strings"
	"testing"
)

func TestTransformPolicyToHelm_RewritesProvisionerServiceAccountVariable(t *testing.T) {
	input := `apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: openbao-restrict-provisioner-rbac
spec:
  variables:
    - name: provisioner_serviceaccount_name
      expression: >-
        'openbao-operator-provisioner'
`

	got := transformPolicyToHelm(input)
	want := `'{{ include "openbao-operator.provisionerServiceAccountName" . }}'`
	if !strings.Contains(got, want) {
		t.Fatalf("transformed policy missing provisioner helper expression %q:\n%s", want, got)
	}
	if strings.Contains(got, `'openbao-operator-provisioner'`) {
		t.Fatalf("transformed policy still contains stale provisioner ServiceAccount name:\n%s", got)
	}
}
