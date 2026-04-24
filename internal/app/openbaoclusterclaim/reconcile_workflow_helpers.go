package openbaoclusterclaim

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func listAndSelectClaimWorkflow[T any, L client.ObjectList](
	ctx context.Context,
	reader client.Reader,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	list L,
	items func(L) []T,
	matches func(*T, *openbaov1alpha1.OpenBaoClusterClaim) bool,
	earlier func(a, b *T) bool,
) (*T, error) {
	if claim == nil || claim.Namespace == "" || claim.Name == "" {
		return nil, nil
	}
	if err := reader.List(ctx, list, client.InNamespace(claim.Namespace)); err != nil {
		return nil, err
	}
	return earliestMatchingWorkflow(
		items(list),
		func(candidate *T) bool {
			return matches(candidate, claim)
		},
		earlier,
	), nil
}

func earliestMatchingWorkflow[T any](items []T, matches func(*T) bool, earlier func(a, b *T) bool) *T {
	var active *T
	for i := range items {
		candidate := &items[i]
		if !matches(candidate) {
			continue
		}
		if active == nil || earlier(candidate, active) {
			active = candidate
		}
	}
	return active
}

func workflowSourceRef(kind string, obj metav1.Object) *openbaov1alpha1.TypedObjectReference {
	if obj == nil {
		return nil
	}
	return &openbaov1alpha1.TypedObjectReference{
		Kind:      kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

func workflowObjectIsEarlier(a, b metav1.Object) bool {
	if a == nil || b == nil {
		return false
	}
	if a.GetCreationTimestamp().Time.Equal(b.GetCreationTimestamp().Time) {
		return a.GetName() < b.GetName()
	}
	return a.GetCreationTimestamp().Time.Before(b.GetCreationTimestamp().Time)
}

func stateIn[T comparable](state T, terminal ...T) bool {
	for _, candidate := range terminal {
		if state == candidate {
			return true
		}
	}
	return false
}
