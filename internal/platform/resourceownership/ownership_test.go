package resourceownership

import (
	"testing"

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
