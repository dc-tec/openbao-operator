package v1alpha1

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBlueGreenPhaseConstants(t *testing.T) {
	t.Parallel()

	if PhaseRollingBack != BlueGreenPhase("RollingBack") {
		t.Fatalf("PhaseRollingBack=%q", PhaseRollingBack)
	}
	if PhaseRollbackCleanup != BlueGreenPhase("RollbackCleanup") {
		t.Fatalf("PhaseRollbackCleanup=%q", PhaseRollbackCleanup)
	}
}

func TestAutoRollbackConfigFields(t *testing.T) {
	t.Parallel()

	cfg := &AutoRollbackConfig{
		Enabled:             true,
		OnJobFailure:        true,
		OnValidationFailure: true,
	}

	if !cfg.Enabled {
		t.Fatalf("Enabled=false")
	}
	if !cfg.OnJobFailure {
		t.Fatalf("OnJobFailure=false")
	}
	if !cfg.OnValidationFailure {
		t.Fatalf("OnValidationFailure=false")
	}
}

func TestBlueGreenStatusFields(t *testing.T) {
	t.Parallel()

	status := &BlueGreenStatus{
		Phase:           PhaseIdle,
		JobFailureCount: 3,
		LastJobFailure:  "test-job",
	}

	if status.JobFailureCount != 3 {
		t.Fatalf("JobFailureCount=%d", status.JobFailureCount)
	}
	if status.LastJobFailure != "test-job" {
		t.Fatalf("LastJobFailure=%q", status.LastJobFailure)
	}

	now := metav1.Now()
	status = &BlueGreenStatus{
		Phase:             PhaseRollingBack,
		RollbackReason:    "job failure threshold exceeded",
		RollbackStartTime: &now,
	}

	if status.RollbackReason != "job failure threshold exceeded" {
		t.Fatalf("RollbackReason=%q", status.RollbackReason)
	}
	if status.RollbackStartTime == nil {
		t.Fatalf("RollbackStartTime=nil")
	}
}

func TestValidationHookConfigFields(t *testing.T) {
	t.Parallel()

	timeout := int32(300)
	hook := &ValidationHookConfig{
		Image:          "busybox:latest",
		Command:        []string{"sh", "-c"},
		Args:           []string{"echo 'validation passed'"},
		TimeoutSeconds: &timeout,
	}

	if hook.Image != "busybox:latest" {
		t.Fatalf("Image=%q", hook.Image)
	}
	if !reflect.DeepEqual(hook.Command, []string{"sh", "-c"}) {
		t.Fatalf("Command=%v", hook.Command)
	}
	if !reflect.DeepEqual(hook.Args, []string{"echo 'validation passed'"}) {
		t.Fatalf("Args=%v", hook.Args)
	}
	if hook.TimeoutSeconds == nil || *hook.TimeoutSeconds != 300 {
		t.Fatalf("TimeoutSeconds=%v", hook.TimeoutSeconds)
	}
}
