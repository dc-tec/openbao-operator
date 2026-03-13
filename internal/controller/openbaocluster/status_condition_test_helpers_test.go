package openbaocluster

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func assertClusterCondition(
	t *testing.T,
	cluster *openbaov1alpha1.OpenBaoCluster,
	conditionType openbaov1alpha1.ConditionType,
	wantPresent bool,
	wantStatus metav1.ConditionStatus,
	wantReason string,
	wantMessageIn string,
) {
	t.Helper()

	cond := meta.FindStatusCondition(cluster.Status.Conditions, string(conditionType))
	if !wantPresent {
		if cond != nil {
			t.Fatalf("expected %s condition to be removed, got %#v", conditionType, cond)
		}
		return
	}
	if cond == nil {
		t.Fatalf("expected %s condition", conditionType)
	}
	if cond.Status != wantStatus || cond.Reason != wantReason {
		t.Fatalf("%s = %#v, want status=%s reason=%s", conditionType, cond, wantStatus, wantReason)
	}
	if wantMessageIn != "" && !strings.Contains(cond.Message, wantMessageIn) {
		t.Fatalf("message = %q, want substring %q", cond.Message, wantMessageIn)
	}
}
