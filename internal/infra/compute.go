package infra

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

// ErrStatefulSetPrerequisitesMissing indicates that required prerequisites (ConfigMap or TLS Secret)
// are missing and the StatefulSet cannot be created. Callers can use this error to set a condition
// and requeue instead of failing reconciliation.
var ErrStatefulSetPrerequisitesMissing = errors.New("StatefulSet prerequisites missing")

// checkStatefulSetPrerequisites verifies that all required resources exist before creating or updating the StatefulSet.
// This prevents pods from failing to start due to missing ConfigMaps or Secrets.
// Returns ErrStatefulSetPrerequisitesMissing if prerequisites are not found (callers should handle this
// by setting a condition and requeuing). Returns other errors for unexpected failures.
func (m *Manager) checkStatefulSetPrerequisites(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, revision string) error {
	// Always check for the config ConfigMap
	configMapName := configMapNameWithRevision(cluster, revision)
	configMap := &corev1.ConfigMap{}
	if err := m.client.Get(ctx, types.NamespacedName{
		Namespace: cluster.Namespace,
		Name:      configMapName,
	}, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: config ConfigMap %s/%s not found; cannot create StatefulSet", ErrStatefulSetPrerequisitesMissing, cluster.Namespace, configMapName)
		}
		return fmt.Errorf("failed to get config ConfigMap %s/%s: %w", cluster.Namespace, configMapName, err)
	}

	// Check for TLS secret if TLS is enabled and not in ACME mode
	// In ACME mode, OpenBao manages certificates internally, so no secret is needed
	if cluster.Spec.TLS.Enabled && !usesACMEMode(cluster) {
		tlsSecretName := tlsServerSecretName(cluster)
		tlsSecret := &corev1.Secret{}
		if err := m.client.Get(ctx, types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      tlsSecretName,
		}, tlsSecret); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("%w: TLS server Secret %s/%s not found; cannot create StatefulSet (waiting for TLS reconciliation or external provider)", ErrStatefulSetPrerequisitesMissing, cluster.Namespace, tlsSecretName)
			}
			return fmt.Errorf("failed to get TLS server Secret %s/%s: %w", cluster.Namespace, tlsSecretName, err)
		}
	}

	return nil
}

// EnsureStatefulSetWithRevision manages the StatefulSet for the OpenBaoCluster using Server-Side Apply.
// This is exported for use by BlueGreenManager.
// verifiedImageDigest is the verified image digest to use (if provided, overrides cluster.Spec.Image).
// verifiedInitContainerDigest is the verified init container image digest to use (if provided, overrides cluster.Spec.InitContainer.Image).
// revision is an optional revision identifier for blue/green deployments (e.g., "blue-v1hash" or "green-v2hash").
// disableSelfInit prevents the pod from attempting to initialize itself (used for Green pods that must join).
// If revision is empty, uses the cluster name (backward compatible behavior).
//
// Note: UpdateStrategy is intentionally not set here to allow UpgradeManager to manage it.
// SSA will preserve fields not specified in the desired object.
func (m *Manager) EnsureStatefulSetWithRevision(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string, verifiedImageDigest string, verifiedInitContainerDigest string, revision string, disableSelfInit bool) error {
	name := statefulSetNameWithRevision(cluster, revision)

	if err := m.ensureConfigMapWithRevision(ctx, cluster, revision, configContent); err != nil {
		return fmt.Errorf("failed to ensure config ConfigMap for StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	// Before creating/updating the StatefulSet, verify all prerequisites exist
	// This is important for External TLS mode where secrets might be deleted/recreated
	if err := m.checkStatefulSetPrerequisites(ctx, cluster, revision); err != nil {
		return err
	}

	initialized := cluster.Status.Initialized
	desiredReplicas := cluster.Spec.Replicas

	// If not initialized, keep at 1 replica until initialization completes
	if !initialized {
		desiredReplicas = 1
		logger.Info("Cluster not yet initialized; keeping StatefulSet at 1 replica", "statefulset", name)
	} else {
		logger.Info("Cluster initialized; ensuring StatefulSet has desired replicas",
			"statefulset", name,
			"desiredReplicas", desiredReplicas)
	}

	desired, buildErr := buildStatefulSetWithRevision(cluster, configContent, initialized, verifiedImageDigest, verifiedInitContainerDigest, revision, disableSelfInit)
	if buildErr != nil {
		return fmt.Errorf("failed to build StatefulSet for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, buildErr)
	}

	// Set the desired replica count (SSA will handle create/update)
	desired.Spec.Replicas = int32Ptr(desiredReplicas)

	// Set TypeMeta for SSA
	desired.TypeMeta = metav1.TypeMeta{
		Kind:       "StatefulSet",
		APIVersion: "apps/v1",
	}

	// Check if the StatefulSet already exists to preserve fields managed by other controllers (UpgradeManager)
	existing := &appsv1.StatefulSet{}
	err := m.client.Get(ctx, types.NamespacedName{Name: name, Namespace: cluster.Namespace}, existing)
	if err == nil {
		// If existing STS has a partition set, preserve it in our desired object.
		// The UpgradeManager sets this field via Patch/SSA, but if we don't include it here,
		// our SSA apply might interfere or try to reset it to nil (depending on ownership).
		// By explicitly including the current value, we tell SSA we are okay with this state.
		if existing.Spec.UpdateStrategy.RollingUpdate != nil && existing.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			if desired.Spec.UpdateStrategy.RollingUpdate == nil {
				desired.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
			}
			desired.Spec.UpdateStrategy.RollingUpdate.Partition = existing.Spec.UpdateStrategy.RollingUpdate.Partition
		}
	} else if !apierrors.IsNotFound(err) {
		// If error is something other than NotFound, return it
		return fmt.Errorf("failed to get existing StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	// Note: We intentionally do NOT set UpdateStrategy here. The UpgradeManager manages
	// UpdateStrategy (including RollingUpdate.Partition) for rollout orchestration.
	// SSA will preserve the existing UpdateStrategy if it's not specified in our desired object.

	if err := m.applyResource(ctx, desired, cluster); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	return nil
}

// deletePVCs removes all PersistentVolumeClaims associated with the OpenBaoCluster.
func (m *Manager) deletePVCs(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) error {
	var pvcList corev1.PersistentVolumeClaimList
	if err := m.client.List(ctx, &pvcList,
		client.InNamespace(cluster.Namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelOpenBaoCluster: cluster.Name,
		}),
	); err != nil {
		return err
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if err := m.client.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
