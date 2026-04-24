package openbaoclusterclaim

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/claimcontract"
)

const bootstrapProjectionComponent = "claim-bootstrap-projection"

func (r runtimeReconciler) ensureLocalBootstrapProjectedArtifacts(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	localTarget *openbaov1alpha1.NamespacedReference,
	inputs claimcontract.SameClusterBootstrapResolvedInputs,
) (bool, error) {
	if claim == nil || localTarget == nil || localTarget.Namespace == "" {
		return false, nil
	}

	desiredConfigMaps, desiredSecrets, desiredRefs := desiredLocalBootstrapProjectedArtifacts(claim, localTarget.Namespace, inputs)
	previousRefs := appliedBootstrapProjectionRefs(claim)
	desiredSet := typedObjectReferenceSet(desiredRefs)
	previousSet := typedObjectReferenceSet(previousRefs)
	refsChanged := len(desiredSet) != len(previousSet)
	if !refsChanged {
		for key := range desiredSet {
			if _, ok := previousSet[key]; !ok {
				refsChanged = true
				break
			}
		}
	}

	for _, desired := range desiredConfigMaps {
		_, err := r.applyProjectedBootstrapConfigMap(ctx, claim, desired)
		if err != nil {
			return false, err
		}
	}
	for _, desired := range desiredSecrets {
		if err := r.applyProjectedBootstrapSecret(ctx, claim, desired); err != nil {
			return false, err
		}
	}

	pruned, err := r.pruneLocalBootstrapProjectedArtifacts(ctx, claim, localTarget.Namespace, previousRefs, desiredRefs)
	if err != nil {
		return false, err
	}
	return refsChanged || pruned, nil
}

func desiredLocalBootstrapProjectedArtifacts(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	namespace string,
	inputs claimcontract.SameClusterBootstrapResolvedInputs,
) (map[string]*corev1.ConfigMap, map[string]*corev1.Secret, []openbaov1alpha1.TypedObjectReference) {
	configMaps := map[string]*corev1.ConfigMap{}
	secrets := map[string]*corev1.Secret{}
	refs := make([]openbaov1alpha1.TypedObjectReference, 0)

	appendArtifact := func(artifact claimcontract.ProjectedBootstrapArtifact) {
		if artifact.Ref.Name == "" || artifact.Ref.Kind == "" {
			return
		}
		refs = append(refs, artifact.Ref)
		switch artifact.Ref.Kind {
		case kindConfigMap:
			if len(artifact.ConfigMapData) == 0 {
				return
			}
			configMaps[artifact.Ref.Name] = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      artifact.Ref.Name,
					Labels:    bootstrapProjectionLabels(claim),
				},
				Data: copyStringMap(artifact.ConfigMapData),
			}
		case kindSecret:
			if len(artifact.SecretData) == 0 {
				return
			}
			secrets[artifact.Ref.Name] = &corev1.Secret{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindSecret},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      artifact.Ref.Name,
					Labels:    bootstrapProjectionLabels(claim),
				},
				Type: corev1.SecretTypeOpaque,
				Data: copySecretData(artifact.SecretData),
			}
		}
	}

	for _, artifact := range inputs.AuthMethodConfigs {
		appendArtifact(artifact)
	}
	for _, artifact := range inputs.PolicyBundleContents {
		appendArtifact(artifact)
	}
	for _, sink := range inputs.AuditDeviceSinks {
		appendArtifact(sink.Artifact)
	}

	return configMaps, secrets, refs
}

func bootstrapProjectionLabels(claim *openbaov1alpha1.OpenBaoClusterClaim) map[string]string {
	return map[string]string{
		constants.LabelAppManagedBy:          constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoOwnershipMode:  constants.LabelValueOpenBaoOwnershipClaimManaged,
		constants.LabelOpenBaoClaimNamespace: claim.Namespace,
		constants.LabelOpenBaoClaimName:      claim.Name,
		constants.LabelOpenBaoComponent:      bootstrapProjectionComponent,
	}
}

func (r runtimeReconciler) applyProjectedBootstrapConfigMap(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *corev1.ConfigMap,
) (bool, error) {
	if desired == nil {
		return false, nil
	}
	if !bootstrapProjectionRefPreviouslyApplied(claim, kindConfigMap, desired.Name) {
		if err := r.client.Create(ctx, desired.DeepCopy()); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return false, invalidResultError(conflictingBootstrapArtifactMessage(kindConfigMap, desired.Namespace, desired.Name))
			}
			return false, fmt.Errorf("create projected bootstrap ConfigMap %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return true, nil
	}
	if err := r.validateProjectedBootstrapConfigMapCustody(ctx, claim, desired); err != nil {
		return false, err
	}

	current := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: desired.Namespace, Name: desired.Name}}
	result, err := controllerutil.CreateOrUpdate(ctx, r.client, current, func() error {
		current.Labels = copyStringMap(desired.Labels)
		current.Data = copyStringMap(desired.Data)
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("upsert projected bootstrap ConfigMap %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return result != controllerutil.OperationResultNone, nil
}

func (r runtimeReconciler) applyProjectedBootstrapSecret(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *corev1.Secret,
) error {
	if desired == nil {
		return nil
	}
	if !bootstrapProjectionRefPreviouslyApplied(claim, kindSecret, desired.Name) {
		if err := r.client.Create(ctx, desired.DeepCopy()); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return invalidResultError(conflictingBootstrapArtifactMessage(kindSecret, desired.Namespace, desired.Name))
			}
			return fmt.Errorf("create projected bootstrap Secret %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}
	if err := r.validateProjectedBootstrapSecretCustody(ctx, claim, desired); err != nil {
		return err
	}

	if err := applySecretWithFallback(ctx, r.client, nil, nil, desired); err != nil {
		return fmt.Errorf("apply projected bootstrap Secret %s/%s: %w", desired.Namespace, desired.Name, err)
	}
	return nil
}

func (r runtimeReconciler) pruneLocalBootstrapProjectedArtifacts(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	namespace string,
	previousRefs []openbaov1alpha1.TypedObjectReference,
	desiredRefs []openbaov1alpha1.TypedObjectReference,
) (bool, error) {
	if namespace == "" {
		return false, nil
	}

	desired := typedObjectReferenceSet(desiredRefs)
	changed := false
	for _, ref := range previousRefs {
		key := typedObjectReferenceKey(ref)
		if key == "" {
			continue
		}
		if _, keep := desired[key]; keep {
			continue
		}
		deleted, err := r.deleteProjectedBootstrapArtifact(ctx, claim, namespace, ref)
		if err != nil {
			return false, err
		}
		changed = changed || deleted
	}
	return changed, nil
}

func (r runtimeReconciler) deleteLocalBootstrapProjectedArtifacts(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	namespace string,
	refs []openbaov1alpha1.TypedObjectReference,
) error {
	if claim == nil || namespace == "" {
		return nil
	}

	for _, ref := range refs {
		_, err := r.deleteProjectedBootstrapArtifact(ctx, claim, namespace, ref)
		if err != nil {
			return err
		}
	}
	// Projected bootstrap artifacts are plain ConfigMaps and Secrets without
	// finalizers. Once delete succeeds, claim finalization does not need to wait
	// for a follow-up reconcile to continue.
	return nil
}

func (r runtimeReconciler) deleteProjectedBootstrapArtifact(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	namespace string,
	ref openbaov1alpha1.TypedObjectReference,
) (bool, error) {
	if ref.Name == "" {
		return false, nil
	}

	obj, description := projectedBootstrapDeletionObject(namespace, ref)
	if obj == nil {
		return false, nil
	}
	if err := r.readClient().Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get projected bootstrap %s %s/%s before delete: %w", description, namespace, ref.Name, err)
	}
	if !bootstrapProjectionObjectOwnedByClaim(obj, claim) {
		return false, nil
	}
	if err := r.client.Delete(ctx, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("delete projected bootstrap %s %s/%s: %w", description, namespace, ref.Name, err)
	}
	return true, nil
}

func projectedBootstrapDeletionObject(namespace string, ref openbaov1alpha1.TypedObjectReference) (client.Object, string) {
	switch ref.Kind {
	case kindConfigMap:
		return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: ref.Name}}, kindConfigMap
	case kindSecret:
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: ref.Name}}, kindSecret
	default:
		return nil, ""
	}
}

func (r runtimeReconciler) validateProjectedBootstrapConfigMapCustody(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *corev1.ConfigMap,
) error {
	if claim == nil || desired == nil {
		return nil
	}
	current := &corev1.ConfigMap{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.readClient().Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get projected bootstrap ConfigMap %s/%s: %w", key.Namespace, key.Name, err)
	}
	if !bootstrapProjectionObjectOwnedByClaim(current, claim) {
		return invalidResultError(conflictingBootstrapArtifactMessage(kindConfigMap, key.Namespace, key.Name))
	}
	return nil
}

func (r runtimeReconciler) validateProjectedBootstrapSecretCustody(
	ctx context.Context,
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	desired *corev1.Secret,
) error {
	if claim == nil || desired == nil {
		return nil
	}
	current := &corev1.Secret{}
	key := client.ObjectKeyFromObject(desired)
	if err := r.readClient().Get(ctx, key, current); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get projected bootstrap Secret %s/%s: %w", key.Namespace, key.Name, err)
	}
	if !bootstrapProjectionObjectOwnedByClaim(current, claim) {
		return invalidResultError(conflictingBootstrapArtifactMessage(kindSecret, key.Namespace, key.Name))
	}
	return nil
}

func appliedBootstrapProjectionRefs(claim *openbaov1alpha1.OpenBaoClusterClaim) []openbaov1alpha1.TypedObjectReference {
	if claim == nil || claim.Status.Applied.RenderedDependencies == nil {
		return nil
	}
	refs := claim.Status.Applied.RenderedDependencies.BootstrapProjectionRefs
	if len(refs) == 0 {
		return nil
	}
	return append([]openbaov1alpha1.TypedObjectReference(nil), refs...)
}

func bootstrapProjectionRefPreviouslyApplied(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	kind string,
	name string,
) bool {
	if kind == "" || name == "" {
		return false
	}
	for _, ref := range appliedBootstrapProjectionRefs(claim) {
		if ref.Kind == kind && ref.Name == name {
			return true
		}
	}
	return false
}

func bootstrapProjectionRefsForDeletion(
	claim *openbaov1alpha1.OpenBaoClusterClaim,
	cluster *openbaov1alpha1.OpenBaoCluster,
) []openbaov1alpha1.TypedObjectReference {
	if refs := appliedBootstrapProjectionRefs(claim); len(refs) > 0 {
		return refs
	}
	return projectedBootstrapRefsFromCluster(cluster)
}

func projectedBootstrapRefsFromCluster(cluster *openbaov1alpha1.OpenBaoCluster) []openbaov1alpha1.TypedObjectReference {
	if cluster == nil || cluster.Spec.SelfInit == nil {
		return nil
	}

	refs := make([]openbaov1alpha1.TypedObjectReference, 0)
	for _, request := range cluster.Spec.SelfInit.Requests {
		if request.AuthMethod != nil && request.AuthMethod.ConfigFromRef != nil && request.AuthMethod.ConfigFromRef.Name != "" && request.AuthMethod.ConfigFromRef.Kind != "" {
			refs = append(refs, *request.AuthMethod.ConfigFromRef)
		}
		if request.Policy != nil && request.Policy.ContentFromRef != nil && request.Policy.ContentFromRef.Name != "" && request.Policy.ContentFromRef.Kind != "" {
			refs = append(refs, *request.Policy.ContentFromRef)
		}
		if request.AuditDevice != nil && request.AuditDevice.SinkFromRef != nil && request.AuditDevice.SinkFromRef.Name != "" && request.AuditDevice.SinkFromRef.Kind != "" {
			refs = append(refs, *request.AuditDevice.SinkFromRef)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}

func typedObjectReferenceSet(refs []openbaov1alpha1.TypedObjectReference) map[string]struct{} {
	out := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := typedObjectReferenceKey(ref)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func typedObjectReferenceKey(ref openbaov1alpha1.TypedObjectReference) string {
	if ref.Kind == "" || ref.Name == "" {
		return ""
	}
	return ref.Kind + "/" + ref.Name
}
