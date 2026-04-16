package statusapply

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GVKResolver resolves an object's GroupVersionKind when it is not already set.
type GVKResolver interface {
	GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error)
}

// ToApplyConfiguration converts a typed object into an apply configuration for
// controller-runtime server-side apply operations.
func ToApplyConfiguration(obj client.Object, resolver GVKResolver) (runtime.ApplyConfiguration, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}
	pruneNilValues(unstructuredMap)

	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		if resolver == nil {
			return nil, fmt.Errorf("resolver is required when object GVK is empty")
		}
		gvk, err = resolver.GroupVersionKindFor(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to get GVK for object: %w", err)
		}
	}
	unstructuredObj.SetGroupVersionKind(gvk)

	return client.ApplyConfigurationFromUnstructured(unstructuredObj), nil
}

func pruneNilValues(m map[string]interface{}) {
	for key, value := range m {
		switch typed := value.(type) {
		case nil:
			delete(m, key)
		case map[string]interface{}:
			pruneNilValues(typed)
		case []interface{}:
			m[key] = pruneNilSlice(typed)
		}
	}
}

func pruneNilSlice(items []interface{}) []interface{} {
	pruned := make([]interface{}, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case nil:
			continue
		case map[string]interface{}:
			pruneNilValues(typed)
			pruned = append(pruned, typed)
		case []interface{}:
			pruned = append(pruned, pruneNilSlice(typed))
		default:
			pruned = append(pruned, item)
		}
	}
	return pruned
}
