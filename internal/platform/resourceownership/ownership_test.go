package resourceownership

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestHasOwnerProof(t *testing.T) {
	owner := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default", UID: types.UID("cluster-uid")},
	}

	controller := true
	tests := []struct {
		name string
		obj  *corev1.ConfigMap
		want bool
	}{
		{
			name: "controller owner reference",
			obj: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:      "owned",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: openbaov1alpha1.GroupVersion.String(),
					Kind:       "OpenBaoCluster",
					Name:       owner.Name,
					UID:        owner.UID,
					Controller: &controller,
				}},
			}},
			want: true,
		},
		{
			name: "owner uid annotation",
			obj: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:        "retained",
				Namespace:   "default",
				Annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
			}},
			want: true,
		},
		{
			name: "non-controller owner reference is insufficient",
			obj: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:      "weak",
				Namespace: "default",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: openbaov1alpha1.GroupVersion.String(),
					Kind:       "OpenBaoCluster",
					Name:       owner.Name,
					UID:        owner.UID,
				}},
			}},
			want: false,
		},
		{
			name: "wrong uid is rejected",
			obj: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
				Name:        "wrong",
				Namespace:   "default",
				Annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: "other"},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasOwnerProof(tt.obj, owner); got != tt.want {
				t.Fatalf("HasOwnerProof() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetOwnerUIDAnnotation(t *testing.T) {
	owner := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default", UID: types.UID("cluster-uid")},
	}
	obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"}}

	if err := SetOwnerUIDAnnotation(obj, owner); err != nil {
		t.Fatalf("SetOwnerUIDAnnotation() error = %v", err)
	}
	if got := obj.Annotations[constants.AnnotationOpenBaoOwnerUID]; got != string(owner.UID) {
		t.Fatalf("owner UID annotation = %q, want %q", got, owner.UID)
	}
}

func TestHasManagedControllerOwnerProof(t *testing.T) {
	owner := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default", UID: types.UID("cluster-uid")},
	}
	ownerGVK := openbaov1alpha1.GroupVersion.WithKind("OpenBaoCluster")
	controller := true
	exactRef := metav1.OwnerReference{
		APIVersion: ownerGVK.GroupVersion().String(),
		Kind:       ownerGVK.Kind,
		Name:       owner.Name,
		UID:        owner.UID,
		Controller: &controller,
	}

	tests := []struct {
		name        string
		namespace   string
		ownerRef    *metav1.OwnerReference
		annotations map[string]string
		want        bool
	}{
		{
			name:        "exact controller reference and reserved annotation",
			namespace:   owner.Namespace,
			ownerRef:    &exactRef,
			annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
			want:        true,
		},
		{
			name:      "controller reference without reserved annotation",
			namespace: owner.Namespace,
			ownerRef:  &exactRef,
		},
		{
			name:        "reserved annotation without controller reference",
			namespace:   owner.Namespace,
			annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
		},
		{
			name:      "different namespace",
			namespace: "other",
			ownerRef:  &exactRef,
			annotations: map[string]string{
				constants.AnnotationOpenBaoOwnerUID: string(owner.UID),
			},
		},
		{
			name:      "wrong controller kind",
			namespace: owner.Namespace,
			ownerRef: func() *metav1.OwnerReference {
				ref := exactRef
				ref.Kind = "OpenBaoRestore"
				return &ref
			}(),
			annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
		},
		{
			name:      "wrong controller name",
			namespace: owner.Namespace,
			ownerRef: func() *metav1.OwnerReference {
				ref := exactRef
				ref.Name = "other"
				return &ref
			}(),
			annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
		},
		{
			name:      "wrong controller UID",
			namespace: owner.Namespace,
			ownerRef: func() *metav1.OwnerReference {
				ref := exactRef
				ref.UID = "other-uid"
				return &ref
			}(),
			annotations: map[string]string{constants.AnnotationOpenBaoOwnerUID: string(owner.UID)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
				Name:        "job",
				Namespace:   tt.namespace,
				Annotations: tt.annotations,
			}}
			if tt.ownerRef != nil {
				job.OwnerReferences = []metav1.OwnerReference{*tt.ownerRef}
			}

			if got := HasManagedControllerOwnerProof(job, owner, ownerGVK); got != tt.want {
				t.Fatalf("HasManagedControllerOwnerProof() = %t, want %t", got, tt.want)
			}
		})
	}
}
