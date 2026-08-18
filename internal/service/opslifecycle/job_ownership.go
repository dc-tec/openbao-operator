package opslifecycle

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/dc-tec/openbao-operator/internal/platform/resourceownership"
)

// PrepareManagedJobOwner stamps the controller reference and reserved owner
// UID annotation used to prove lifecycle Job provenance.
func PrepareManagedJobOwner(job *batchv1.Job, owner client.Object, scheme *runtime.Scheme) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	if scheme == nil {
		return fmt.Errorf("scheme is required")
	}
	if err := resourceownership.RequireOwnerUID(owner); err != nil {
		return err
	}
	if err := controllerutil.SetControllerReference(owner, job, scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	if err := resourceownership.SetOwnerUIDAnnotation(job, owner); err != nil {
		return fmt.Errorf("failed to set owner UID annotation: %w", err)
	}
	return nil
}

// RequireManagedJobOwner rejects a lifecycle Job that lacks exact managed
// controller ownership proof.
func RequireManagedJobOwner(
	action string,
	job *batchv1.Job,
	owner client.Object,
	ownerGVK schema.GroupVersionKind,
) error {
	return resourceownership.RequireManagedControllerOwnerProof(action, job, owner, ownerGVK)
}

// ReadManagedJob reads a lifecycle Job and validates its provenance before a
// caller interprets its status or performs a mutation.
func ReadManagedJob(
	ctx context.Context,
	reader client.Reader,
	key client.ObjectKey,
	owner client.Object,
	ownerGVK schema.GroupVersionKind,
	action string,
) (*batchv1.Job, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader is required")
	}

	job := &batchv1.Job{}
	if err := reader.Get(ctx, key, job); err != nil {
		return nil, err
	}
	if err := RequireManagedJobOwner(action, job, owner, ownerGVK); err != nil {
		return nil, err
	}
	return job, nil
}
