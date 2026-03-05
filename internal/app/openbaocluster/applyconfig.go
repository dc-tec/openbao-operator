package openbaocluster

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type gvkResolver interface {
	GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error)
}

func toApplyConfiguration(obj client.Object, resolver gvkResolver) (runtime.ApplyConfiguration, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

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
