package opslifecycle

import (
	"errors"
	"fmt"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/proptest"
	"pgregory.net/rapid"
)

func TestOperationLockIsHeldByProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		holder := proptest.Identifier().Draw(rt, "holder")
		operation := clusterOperationGenerator().Draw(rt, "operation")

		statusHolder := holder
		if !rapid.Bool().Draw(rt, "holder_matches") {
			statusHolder = proptest.DifferentIdentifier(rt, "status_holder", holder)
		}

		statusOperation := operation
		if !rapid.Bool().Draw(rt, "operation_matches") {
			statusOperation = differentClusterOperation(rt, "status_operation", operation)
		}

		lock := OperationLock{
			Holder:    holder,
			Operation: operation,
		}
		statusLock := &openbaov1alpha1.OperationLockStatus{
			Holder:    statusHolder,
			Operation: statusOperation,
		}

		want := statusHolder == holder && statusOperation == operation
		if got := lock.IsHeldBy(statusLock); got != want {
			t.Fatalf("IsHeldBy() = %t, want %t for lock=%+v status=%+v", got, want, lock, statusLock)
		}
		if lock.IsHeldBy(nil) {
			t.Fatalf("IsHeldBy(nil) = true, want false for lock=%+v", lock)
		}
	})
}

func TestHeldErrorClassificationProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		heldErr := &HeldError{
			Operation: clusterOperationGenerator().Draw(rt, "operation"),
			Holder:    proptest.Identifier().Draw(rt, "holder"),
			Message:   proptest.OptionalIdentifier().Draw(rt, "message"),
		}
		wrappedErr := fmt.Errorf("wrapped: %w", heldErr)

		if !IsLockHeld(wrappedErr) {
			t.Fatalf("IsLockHeld() = false, want true for %v", wrappedErr)
		}
		gotHeldErr, ok := AsHeldError(wrappedErr)
		if !ok {
			t.Fatalf("AsHeldError() ok = false, want true for %v", wrappedErr)
		}
		if gotHeldErr.Operation != heldErr.Operation || gotHeldErr.Holder != heldErr.Holder {
			t.Fatalf("AsHeldError() = %+v, want operation=%q holder=%q", gotHeldErr, heldErr.Operation, heldErr.Holder)
		}

		fields := map[string]string{
			"existing": proptest.Identifier().Draw(rt, "existing_field"),
		}
		AddHeldAuditFields(fields, wrappedErr)
		if fields["held_by_operation"] != string(heldErr.Operation) {
			t.Fatalf("held_by_operation = %q, want %q", fields["held_by_operation"], heldErr.Operation)
		}
		if fields["held_by_holder"] != heldErr.Holder {
			t.Fatalf("held_by_holder = %q, want %q", fields["held_by_holder"], heldErr.Holder)
		}
		if fields["existing"] == "" {
			t.Fatalf("existing field was not preserved: %+v", fields)
		}

		plainErr := errors.New(proptest.Identifier().Draw(rt, "plain_error"))
		if IsLockHeld(plainErr) {
			t.Fatalf("IsLockHeld() = true, want false for %v", plainErr)
		}
		before := copyStringMap(fields)
		AddHeldAuditFields(fields, plainErr)
		if !stringMapsEqual(fields, before) {
			t.Fatalf("AddHeldAuditFields() changed fields for unrelated error: got %+v want %+v", fields, before)
		}

		AddHeldAuditFields(nil, wrappedErr)
	})
}

func TestPhaseAndRequeueProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		phaseFrom := proptest.Identifier().Draw(rt, "phase_from")
		phaseTo := proptest.Identifier().Draw(rt, "phase_to")
		fields := rapid.MapOfN(fieldKeyGenerator(), proptest.OptionalIdentifier(), 0, 8).Draw(rt, "fields")
		fieldsBefore := copyStringMap(fields)

		got := phaseTransitionFields(phaseFrom, phaseTo, fields)
		if got["phase_from"] != phaseFrom {
			t.Fatalf("phase_from = %q, want %q", got["phase_from"], phaseFrom)
		}
		if got["phase_to"] != phaseTo {
			t.Fatalf("phase_to = %q, want %q", got["phase_to"], phaseTo)
		}
		for key, value := range fieldsBefore {
			if key == "phase_from" || key == "phase_to" {
				continue
			}
			if got[key] != value {
				t.Fatalf("field %q = %q, want %q", key, got[key], value)
			}
		}
		got["new_field"] = "new-value"
		if !stringMapsEqual(fields, fieldsBefore) {
			t.Fatalf("phaseTransitionFields() mutated input: got %+v want %+v", fields, fieldsBefore)
		}

		retryClass := retryClassGenerator().Draw(rt, "retry_class")
		delay := RequeueDelay(retryClass)
		switch retryClass {
		case RetryClassLockContention, RetryClassProgressPoll:
			if delay != requeueShort {
				t.Fatalf("RequeueDelay(%q) = %s, want %s", retryClass, delay, requeueShort)
			}
		case RetryClassStandard:
			if delay != requeueStandard {
				t.Fatalf("RequeueDelay(%q) = %s, want %s", retryClass, delay, requeueStandard)
			}
		default:
			if delay != requeueStandard {
				t.Fatalf("RequeueDelay(%q) = %s, want default %s", retryClass, delay, requeueStandard)
			}
		}
	})
}

func clusterOperationGenerator() *rapid.Generator[openbaov1alpha1.ClusterOperation] {
	return rapid.SampledFrom([]openbaov1alpha1.ClusterOperation{
		openbaov1alpha1.ClusterOperationUpgrade,
		openbaov1alpha1.ClusterOperationBackup,
		openbaov1alpha1.ClusterOperationRestore,
		openbaov1alpha1.ClusterOperation("Generated"),
	})
}

func differentClusterOperation(
	t *rapid.T,
	label string,
	other openbaov1alpha1.ClusterOperation,
) openbaov1alpha1.ClusterOperation {
	return clusterOperationGenerator().Filter(func(candidate openbaov1alpha1.ClusterOperation) bool {
		return candidate != other
	}).Draw(t, label)
}

func fieldKeyGenerator() *rapid.Generator[string] {
	return rapid.OneOf(
		proptest.Identifier(),
		rapid.SampledFrom([]string{"phase_from", "phase_to", "cluster_name", "restore_name", "operation"}),
	)
}

func retryClassGenerator() *rapid.Generator[RetryClass] {
	return rapid.SampledFrom([]RetryClass{
		RetryClassLockContention,
		RetryClassProgressPoll,
		RetryClassStandard,
		"",
		RetryClass("unknown"),
	})
}

func copyStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok || rightValue != leftValue {
			return false
		}
	}
	return true
}
