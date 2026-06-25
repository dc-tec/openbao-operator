package resourceownership

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/proptest"
	"pgregory.net/rapid"
)

func TestOwnerProofProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		ownerUID := uidGenerator().Draw(rt, "owner_uid")
		owner := generatedOwner(ownerUID)
		obj := generatedObject(rt, ownerUID)

		wantControllerProof := false
		wantAnnotationProof := false
		if ownerUID != "" {
			for _, ref := range obj.OwnerReferences {
				if ref.Controller != nil && *ref.Controller && ref.UID == ownerUID {
					wantControllerProof = true
					break
				}
			}
			wantAnnotationProof = obj.Annotations[constants.AnnotationOpenBaoOwnerUID] == string(ownerUID)
		}
		wantOwnerProof := wantControllerProof || wantAnnotationProof

		if got := HasControllerOwnerReference(obj, owner); got != wantControllerProof {
			t.Fatalf("HasControllerOwnerReference() = %t, want %t for ownerUID=%q refs=%+v",
				got, wantControllerProof, ownerUID, obj.OwnerReferences)
		}
		if got := HasOwnerUIDAnnotation(obj, owner); got != wantAnnotationProof {
			t.Fatalf("HasOwnerUIDAnnotation() = %t, want %t for ownerUID=%q annotations=%+v",
				got, wantAnnotationProof, ownerUID, obj.Annotations)
		}
		if got := HasOwnerProof(obj, owner); got != wantOwnerProof {
			t.Fatalf("HasOwnerProof() = %t, want %t for ownerUID=%q refs=%+v annotations=%+v",
				got, wantOwnerProof, ownerUID, obj.OwnerReferences, obj.Annotations)
		}

		if err := RequireOwnerProof("manage", obj, owner); (err == nil) != wantOwnerProof {
			t.Fatalf("RequireOwnerProof() error = %v, ownerProof=%t", err, wantOwnerProof)
		}
	})
}

func TestOwnerProofNilAndEmptyOwnerProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		ownerUID := uidGenerator().Draw(rt, "owner_uid")
		obj := generatedObject(rt, ownerUID)

		if HasOwnerProof(nil, generatedOwner(ownerUID)) {
			t.Fatalf("HasOwnerProof(nil, owner) = true, want false")
		}
		if HasOwnerProof(obj, nil) {
			t.Fatalf("HasOwnerProof(obj, nil) = true, want false")
		}
		if HasOwnerProof(obj, generatedOwner("")) {
			t.Fatalf("HasOwnerProof(obj, empty-owner-uid) = true, want false")
		}
		if err := RequireOwnerUID(nil); err == nil {
			t.Fatalf("RequireOwnerUID(nil) error = nil, want error")
		}
		if err := RequireOwnerUID(generatedOwner("")); err == nil {
			t.Fatalf("RequireOwnerUID(empty UID) error = nil, want error")
		}
		if ownerUID != "" {
			if err := RequireOwnerUID(generatedOwner(ownerUID)); err != nil {
				t.Fatalf("RequireOwnerUID(non-empty UID) error = %v, want nil", err)
			}
		}
	})
}

func TestSetOwnerUIDAnnotationProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		ownerUID := uidGenerator().Draw(rt, "owner_uid")
		owner := generatedOwner(ownerUID)
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:        proptest.Identifier().Draw(rt, "object_name"),
				Namespace:   "default",
				Annotations: annotationMapGenerator().Draw(rt, "annotations"),
			},
		}
		before := copyAnnotations(obj.Annotations)

		if err := SetOwnerUIDAnnotation(obj, owner); err != nil {
			t.Fatalf("SetOwnerUIDAnnotation() error = %v", err)
		}

		if ownerUID == "" {
			if !stringMapsEqual(obj.Annotations, before) {
				t.Fatalf("empty owner UID changed annotations: got %+v want %+v", obj.Annotations, before)
			}
			return
		}

		if got := obj.Annotations[constants.AnnotationOpenBaoOwnerUID]; got != string(ownerUID) {
			t.Fatalf("owner UID annotation = %q, want %q", got, ownerUID)
		}
		for key, value := range before {
			if key == constants.AnnotationOpenBaoOwnerUID {
				continue
			}
			if obj.Annotations[key] != value {
				t.Fatalf("annotation %q = %q, want preserved value %q", key, obj.Annotations[key], value)
			}
		}
	})
}

func TestSetOwnerUIDAnnotationNilInputProperties(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		owner := generatedOwner(uidGenerator().Draw(rt, "owner_uid"))
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cfg", Namespace: "default"}}

		if err := SetOwnerUIDAnnotation(nil, owner); err == nil {
			t.Fatalf("SetOwnerUIDAnnotation(nil, owner) error = nil, want error")
		}
		if err := SetOwnerUIDAnnotation(obj, nil); err == nil {
			t.Fatalf("SetOwnerUIDAnnotation(obj, nil) error = nil, want error")
		}
	})
}

func generatedOwner(uid types.UID) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "default",
			UID:       uid,
		},
	}
}

func generatedObject(t *rapid.T, ownerUID types.UID) *corev1.ConfigMap {
	annotations := annotationMapGenerator().Draw(t, "annotations")
	if rapid.Bool().Draw(t, "owner_annotation_matches") {
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[constants.AnnotationOpenBaoOwnerUID] = string(ownerUID)
	}

	refs := rapid.SliceOfN(ownerReferenceGenerator(ownerUID), 0, 5).Draw(t, "owner_references")
	if rapid.Bool().Draw(t, "include_matching_controller_ref") {
		refs = append(refs, ownerReference(ownerUID, boolPtr(true)))
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            proptest.Identifier().Draw(t, "object_name"),
			Namespace:       "default",
			Annotations:     annotations,
			OwnerReferences: refs,
		},
	}
}

func ownerReferenceGenerator(ownerUID types.UID) *rapid.Generator[metav1.OwnerReference] {
	return rapid.Custom(func(t *rapid.T) metav1.OwnerReference {
		uid := differentUID(t, "ref_uid", ownerUID)
		if rapid.Bool().Draw(t, "ref_uid_matches") {
			uid = ownerUID
		}
		return ownerReference(uid, controllerPointerGenerator().Draw(t, "controller"))
	})
}

func ownerReference(uid types.UID, controller *bool) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: openbaov1alpha1.GroupVersion.String(),
		Kind:       "OpenBaoCluster",
		Name:       "cluster",
		UID:        uid,
		Controller: controller,
	}
}

func controllerPointerGenerator() *rapid.Generator[*bool] {
	return rapid.SampledFrom([]*bool{nil, boolPtr(false), boolPtr(true)})
}

func uidGenerator() *rapid.Generator[types.UID] {
	return rapid.Map(proptest.OptionalIdentifier(), func(value string) types.UID {
		return types.UID(value)
	})
}

func differentUID(t *rapid.T, label string, other types.UID) types.UID {
	if other == "" {
		return uidGenerator().Filter(func(candidate types.UID) bool {
			return candidate != ""
		}).Draw(t, label)
	}
	return uidGenerator().Filter(func(candidate types.UID) bool {
		return candidate != other
	}).Draw(t, label)
}

func annotationMapGenerator() *rapid.Generator[map[string]string] {
	return rapid.MapOfN(annotationKeyGenerator(), proptest.OptionalIdentifier(), 0, 5)
}

func annotationKeyGenerator() *rapid.Generator[string] {
	return rapid.Map(proptest.Identifier(), func(value string) string {
		if strings.HasPrefix(value, "openbao.org/") {
			return fmt.Sprintf("test.openbao.org/%s", strings.TrimPrefix(value, "openbao.org/"))
		}
		return value
	})
}

func boolPtr(value bool) *bool {
	return &value
}

func copyAnnotations(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
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
