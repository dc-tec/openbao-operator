package statusapply

import (
	"encoding/json"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestToApplyConfiguration_PrunesNestedNilStatusFields(t *testing.T) {
	t.Parallel()

	startedAt := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion:    "2.5.0",
				FromVersion:      "2.4.4",
				CurrentPartition: 2,
				StartedAt:        &startedAt,
				Failure:          nil,
				LastErrorAt:      nil,
				LastStepDownTime: nil,
			},
		},
	}

	applyConfig, err := ToApplyConfiguration(cluster, nil)
	if err != nil {
		t.Fatalf("ToApplyConfiguration() error = %v", err)
	}

	payload, err := json.Marshal(applyConfig)
	if err != nil {
		t.Fatalf("json.Marshal(applyConfig) error = %v", err)
	}

	got := string(payload)
	for _, forbidden := range []string{
		`"failure":null`,
		`"lastErrorAt":null`,
		`"lastStepDownTime":null`,
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("apply payload unexpectedly contains %s: %s", forbidden, got)
		}
	}
}

func TestToApplyConfiguration_PreservesEmptyObjectClearSemantics(t *testing.T) {
	t.Parallel()

	startedAt := metav1.Now()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: openbaov1alpha1.GroupVersion.String(),
			Kind:       "OpenBaoCluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{
			Upgrade: &openbaov1alpha1.UpgradeProgress{
				TargetVersion:    "2.5.0",
				FromVersion:      "2.4.4",
				CurrentPartition: 2,
				StartedAt:        &startedAt,
				Failure:          &openbaov1alpha1.ControllerErrorStatus{},
			},
		},
	}

	applyConfig, err := ToApplyConfiguration(cluster, nil)
	if err != nil {
		t.Fatalf("ToApplyConfiguration() error = %v", err)
	}

	payload, err := json.Marshal(applyConfig)
	if err != nil {
		t.Fatalf("json.Marshal(applyConfig) error = %v", err)
	}

	got := string(payload)
	if strings.Contains(got, `"failure":null`) {
		t.Fatalf("apply payload unexpectedly contains null failure object: %s", got)
	}
	if !strings.Contains(got, `"failure":{}`) {
		t.Fatalf("apply payload missing empty failure object clear semantics: %s", got)
	}
}
