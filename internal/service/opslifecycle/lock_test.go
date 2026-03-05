package opslifecycle

import (
	"errors"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/operationlock"
)

func TestOperationLockIsHeldBy(t *testing.T) {
	t.Parallel()

	lock := OperationLock{
		Holder:    "controller/restore",
		Operation: openbaov1alpha1.ClusterOperationRestore,
	}

	if !lock.IsHeldBy(&openbaov1alpha1.OperationLockStatus{
		Holder:    "controller/restore",
		Operation: openbaov1alpha1.ClusterOperationRestore,
	}) {
		t.Fatal("expected matching lock ownership to be true")
	}

	if lock.IsHeldBy(&openbaov1alpha1.OperationLockStatus{
		Holder:    "controller/backup",
		Operation: openbaov1alpha1.ClusterOperationRestore,
	}) {
		t.Fatal("expected mismatched holder to be false")
	}
}

func TestIsLockHeld(t *testing.T) {
	t.Parallel()

	heldErr := &operationlock.HeldError{
		Operation: openbaov1alpha1.ClusterOperationBackup,
		Holder:    "controller/backup",
	}

	if !IsLockHeld(heldErr) {
		t.Fatal("expected held error to match lock-held classification")
	}

	if !IsLockHeld(errors.Join(errors.New("wrapped"), heldErr)) {
		t.Fatal("expected wrapped held error to match lock-held classification")
	}

	if IsLockHeld(errors.New("other")) {
		t.Fatal("did not expect unrelated error to match lock-held classification")
	}
}

func TestAddHeldAuditFields(t *testing.T) {
	t.Parallel()

	fields := map[string]string{
		"cluster_name": "openbao",
	}
	err := &operationlock.HeldError{
		Operation: openbaov1alpha1.ClusterOperationUpgrade,
		Holder:    "controller/upgrade",
	}

	AddHeldAuditFields(fields, err)

	if fields["held_by_operation"] != string(openbaov1alpha1.ClusterOperationUpgrade) {
		t.Fatalf("expected held_by_operation=%q, got %q", openbaov1alpha1.ClusterOperationUpgrade, fields["held_by_operation"])
	}
	if fields["held_by_holder"] != "controller/upgrade" {
		t.Fatalf("expected held_by_holder=%q, got %q", "controller/upgrade", fields["held_by_holder"])
	}
}
