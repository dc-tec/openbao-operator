// Package kube provides Kubernetes-specific utilities and helpers.
package kube

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GVKResolver is a minimal interface for resolving GroupVersionKind from objects.
// This interface allows for easier testing and can be implemented by client.Client
// or other types that can resolve GVKs.
type GVKResolver interface {
	GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error)
}

// ToApplyConfiguration converts a client.Object to a runtime.ApplyConfiguration
// for use with client.Client.Apply() or client.Client.Status().Apply().
//
// This function handles the conversion from typed Kubernetes objects to the
// unstructured format required for Server-Side Apply. It automatically resolves
// the GroupVersionKind if not already set on the object.
//
// The resolver parameter is optional and only needed when the object's GVK is empty.
// If nil and GVK resolution is required, an error will be returned.
//
// The returned ApplyConfiguration can be used directly with:
//   - client.Client.Apply(ctx, applyConfig, opts...)
//   - client.Client.Status().Apply(ctx, applyConfig, opts...)
func ToApplyConfiguration(obj client.Object, resolver GVKResolver) (runtime.ApplyConfiguration, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	// Convert to unstructured for Apply
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert object to unstructured: %w", err)
	}

	unstructuredObj := &unstructured.Unstructured{Object: u}
	gvk := obj.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		if resolver == nil {
			return nil, fmt.Errorf("resolver is required when object GVK is empty")
		}
		var err error
		gvk, err = resolver.GroupVersionKindFor(obj)
		if err != nil {
			return nil, fmt.Errorf("failed to get GVK for object: %w", err)
		}
	}
	unstructuredObj.SetGroupVersionKind(gvk)

	return client.ApplyConfigurationFromUnstructured(unstructuredObj), nil
}
