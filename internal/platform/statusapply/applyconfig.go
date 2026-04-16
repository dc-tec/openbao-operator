package statusapply

import (
	"fmt"
	"strings"

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

	return toApplyConfigurationFromMap(obj, resolver, unstructuredMap)
}

// ToApplyConfigurationWithExplicitNulls converts a typed object into an apply
// configuration while preserving explicit nulls for the provided dotted field
// paths after normal nil-pruning.
//
// This is needed when omission-based SSA clears are insufficient because the
// target field may have been seeded outside the current field manager.
func ToApplyConfigurationWithExplicitNulls(
	obj client.Object,
	resolver GVKResolver,
	explicitNullPaths ...string,
) (runtime.ApplyConfiguration, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}
	pruneNilValues(unstructuredMap)
	for _, path := range explicitNullPaths {
		if path == "" {
			continue
		}
		if err := setExplicitNull(unstructuredMap, strings.Split(path, ".")); err != nil {
			return nil, err
		}
	}

	return toApplyConfigurationFromMap(obj, resolver, unstructuredMap)
}

func toApplyConfigurationFromMap(
	obj client.Object,
	resolver GVKResolver,
	unstructuredMap map[string]interface{},
) (runtime.ApplyConfiguration, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		if resolver == nil {
			return nil, fmt.Errorf("resolver is required when object GVK is empty")
		}
		resolvedGVK, err := resolver.GroupVersionKindFor(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to get GVK for object: %w", err)
		}
		gvk = resolvedGVK
	}
	unstructuredObj.SetGroupVersionKind(gvk)

	return client.ApplyConfigurationFromUnstructured(unstructuredObj), nil
}

func setExplicitNull(m map[string]interface{}, path []string) error {
	if len(path) == 0 {
		return fmt.Errorf("explicit null path is required")
	}
	current := m
	for _, segment := range path[:len(path)-1] {
		next, ok := current[segment]
		if !ok {
			child := map[string]interface{}{}
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("explicit null path %q traverses non-object segment %q", strings.Join(path, "."), segment)
		}
		current = child
	}
	current[path[len(path)-1]] = nil
	return nil
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
