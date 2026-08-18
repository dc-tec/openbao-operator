package resourceownership

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// HasOwnerProof reports whether obj is provably owned by owner.
//
// Controller owner references are preferred. The owner UID annotation is used
// for retained resources, such as PVCs, that must survive owner deletion.
func HasOwnerProof(obj client.Object, owner client.Object) bool {
	return HasControllerOwnerReference(obj, owner) || HasOwnerUIDAnnotation(obj, owner)
}

// HasControllerOwnerReference reports whether obj has a controller owner
// reference for owner.
func HasControllerOwnerReference(obj client.Object, owner client.Object) bool {
	if obj == nil || owner == nil {
		return false
	}
	ownerUID := owner.GetUID()
	if ownerUID == "" {
		return false
	}
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if ref.UID == ownerUID {
			return true
		}
	}
	return false
}

// HasManagedControllerOwnerProof reports whether obj has both the exact
// controller owner reference and the reserved owner UID annotation for owner.
// The annotation prevents namespace users from claiming operator ownership
// when the managed-resource admission policy is enforced.
func HasManagedControllerOwnerProof(
	obj client.Object,
	owner client.Object,
	ownerGVK schema.GroupVersionKind,
) bool {
	if obj == nil || owner == nil || owner.GetUID() == "" || ownerGVK.Empty() {
		return false
	}
	if obj.GetNamespace() != owner.GetNamespace() {
		return false
	}

	ref := metav1.GetControllerOfNoCopy(obj)
	if ref == nil ||
		ref.APIVersion != ownerGVK.GroupVersion().String() ||
		ref.Kind != ownerGVK.Kind ||
		ref.Name != owner.GetName() ||
		ref.UID != owner.GetUID() {
		return false
	}

	return HasOwnerUIDAnnotation(obj, owner)
}

// HasOwnerUIDAnnotation reports whether obj carries the retained-resource
// provenance annotation for owner.
func HasOwnerUIDAnnotation(obj client.Object, owner client.Object) bool {
	if obj == nil || owner == nil || owner.GetUID() == "" {
		return false
	}
	return obj.GetAnnotations()[constants.AnnotationOpenBaoOwnerUID] == string(owner.GetUID())
}

// SetOwnerUIDAnnotation records the owning resource UID on obj.
func SetOwnerUIDAnnotation(obj client.Object, owner client.Object) error {
	if obj == nil {
		return fmt.Errorf("object is required")
	}
	if owner == nil {
		return fmt.Errorf("owner is required")
	}
	uid := owner.GetUID()
	if uid == "" {
		return nil
	}
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[constants.AnnotationOpenBaoOwnerUID] = string(uid)
	obj.SetAnnotations(annotations)
	return nil
}

func RequireOwnerUID(owner client.Object) error {
	if owner == nil {
		return fmt.Errorf("owner is required")
	}
	if owner.GetUID() == "" {
		return fmt.Errorf("owner UID is required")
	}
	return nil
}

// RequireOwnerProof returns an error when obj exists but is not proven to
// belong to owner.
func RequireOwnerProof(action string, obj client.Object, owner client.Object) error {
	if HasOwnerProof(obj, owner) {
		return nil
	}
	namespace, name := "", ""
	if obj != nil {
		namespace = obj.GetNamespace()
		name = obj.GetName()
	}
	return fmt.Errorf("%s %s %s/%s requires OpenBaoCluster owner proof", strings.TrimSpace(action), kindOf(obj), namespace, name)
}

// RequireManagedControllerOwnerProof rejects an object that does not have the
// exact managed controller ownership proof for owner.
func RequireManagedControllerOwnerProof(
	action string,
	obj client.Object,
	owner client.Object,
	ownerGVK schema.GroupVersionKind,
) error {
	if HasManagedControllerOwnerProof(obj, owner, ownerGVK) {
		return nil
	}

	namespace, name := "", ""
	if obj != nil {
		namespace = obj.GetNamespace()
		name = obj.GetName()
	}
	ownerNamespace, ownerName, ownerUID := "", "", ""
	if owner != nil {
		ownerNamespace = owner.GetNamespace()
		ownerName = owner.GetName()
		ownerUID = string(owner.GetUID())
	}
	return fmt.Errorf(
		"%s %s %s/%s requires managed controller owner %s %s/%s with UID %q",
		strings.TrimSpace(action),
		kindOf(obj),
		namespace,
		name,
		ownerGVK.Kind,
		ownerNamespace,
		ownerName,
		ownerUID,
	)
}

func kindOf(obj client.Object) string {
	if obj == nil {
		return "resource"
	}
	if kind := obj.GetObjectKind().GroupVersionKind().Kind; kind != "" {
		return kind
	}
	return "resource"
}
