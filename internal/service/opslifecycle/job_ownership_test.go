package opslifecycle

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestPrepareManagedJobOwner(t *testing.T) {
	t.Parallel()

	scheme := runtime.NewScheme()
	if err := openbaov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	owner := testJobOwner()
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:      "backup",
		Namespace: owner.Namespace,
	}}

	if err := PrepareManagedJobOwner(job, owner, scheme); err != nil {
		t.Fatalf("PrepareManagedJobOwner() error = %v", err)
	}
	if err := RequireManagedJobOwner(
		"observe backup",
		job,
		owner,
		openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster"),
	); err != nil {
		t.Fatalf("RequireManagedJobOwner() error = %v", err)
	}
	if got := job.Annotations[constants.AnnotationOpenBaoOwnerUID]; got != string(owner.UID) {
		t.Fatalf("owner UID annotation = %q, want %q", got, owner.UID)
	}
}

func TestReadManagedJobRejectsIncompleteOwnershipProof(t *testing.T) {
	t.Parallel()

	owner := testJobOwner()
	ownerGVK := openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster")
	controllerRef := *metav1.NewControllerRef(owner, ownerGVK)

	tests := []struct {
		name        string
		status      batchv1.JobStatus
		ownerRefs   []metav1.OwnerReference
		annotations map[string]string
	}{
		{
			name:   "active foreign Job",
			status: batchv1.JobStatus{Active: 1},
		},
		{
			name:      "succeeded Job with spoofable owner reference only",
			status:    batchv1.JobStatus{Succeeded: 1},
			ownerRefs: []metav1.OwnerReference{controllerRef},
		},
		{
			name:   "failed Job with reserved annotation only",
			status: batchv1.JobStatus{Failed: 1},
			annotations: map[string]string{
				constants.AnnotationOpenBaoOwnerUID: string(owner.UID),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			if err := batchv1.AddToScheme(scheme); err != nil {
				t.Fatalf("AddToScheme() error = %v", err)
			}
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "lifecycle-job",
					Namespace:       owner.Namespace,
					OwnerReferences: tt.ownerRefs,
					Annotations:     tt.annotations,
				},
				Status: tt.status,
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()

			_, err := ReadManagedJob(
				context.Background(),
				reader,
				client.ObjectKeyFromObject(job),
				owner,
				ownerGVK,
				"observe lifecycle",
			)
			if err == nil {
				t.Fatal("ReadManagedJob() error = nil, want ownership error")
			}
		})
	}
}

func testJobOwner() *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{
		Name:      "example",
		Namespace: "default",
		UID:       types.UID("cluster-uid"),
	}}
}
