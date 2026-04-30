package workload

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceapply"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
)

// Manager reconciles workload-owned resources for an OpenBaoCluster.
type Manager struct {
	client   client.Client
	reader   client.Reader
	scheme   *runtime.Scheme
	platform string
}

func NewManager(c client.Client, scheme *runtime.Scheme, platform string) *Manager {
	return &Manager{client: c, reader: c, scheme: scheme, platform: platform}
}

func (m *Manager) WithReader(reader client.Reader) *Manager {
	if reader != nil {
		m.reader = reader
	}
	return m
}

func (m *Manager) EnsureBlueGreenStatus(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) {
	EnsureBlueGreenStatus(ctx, logger, m.reader, cluster)
}

func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, configContent string, spec StatefulSetSpec) error {
	if spec.SkipReconciliation {
		logger.Info("Skipping StatefulSet reconciliation per spec", "reason", "skip flag set")
		return nil
	}

	if spec.Pool == "" {
		spec.Pool = constants.LabelValueOpenBaoWorkloadPoolVoter
	}

	if spec.Pool == constants.LabelValueOpenBaoWorkloadPoolVoter && cluster != nil && spec.Replicas != cluster.Spec.Replicas {
		logger.V(1).Info(
			"Applying staged StatefulSet replica count",
			"statefulset", statefulSetNameForSpec(cluster, spec),
			"clusterDesiredReplicas", cluster.Spec.Replicas,
			"appliedStatefulSetReplicas", spec.Replicas,
		)
	}

	if err := m.EnsureStatefulSet(ctx, logger, cluster, configContent, spec); err != nil {
		return err
	}

	if err := m.ensurePodDisruptionBudget(ctx, logger, cluster, spec); err != nil {
		return err
	}

	return nil
}

func (m *Manager) ScaleDownStatefulSetIfExists(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) error {
	return m.ScaleStatefulSetIfExists(ctx, logger, cluster, spec, 0)
}

func (m *Manager) DeleteStatefulSetIfExists(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) error {
	name := statefulSetNameForSpec(cluster, spec)
	if name == "" {
		return nil
	}

	statefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: name}, statefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get StatefulSet %s/%s for deletion: %w", cluster.Namespace, name, err)
	}

	if err := m.client.Delete(ctx, statefulSet); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete StatefulSet %s/%s: %w", cluster.Namespace, name, err)
	}

	logger.Info("Deleted StatefulSet", "statefulset", name, "pool", spec.Pool)
	return nil
}

func (m *Manager) ScaleStatefulSetIfExists(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec, replicas int32) error {
	name := statefulSetNameForSpec(cluster, spec)
	if name == "" {
		return nil
	}

	statefulSet := &appsv1.StatefulSet{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: name}, statefulSet); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get StatefulSet %s/%s for staged scale down: %w", cluster.Namespace, name, err)
	}

	if statefulSet.Spec.Replicas != nil && *statefulSet.Spec.Replicas == replicas {
		return nil
	}

	updated := statefulSet.DeepCopy()
	updated.Spec.Replicas = int32Ptr(replicas)
	if err := m.client.Patch(ctx, updated, client.MergeFrom(statefulSet)); err != nil {
		return fmt.Errorf("failed to scale StatefulSet %s/%s to %d replicas: %w", cluster.Namespace, name, replicas, err)
	}

	logger.Info("Scaled StatefulSet", "statefulset", name, "pool", spec.Pool, "replicas", replicas)
	return nil
}

func (m *Manager) DeleteConfigMapIfExists(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) error {
	cmName := configMapNameForSpec(cluster, spec)
	if cmName == "" {
		return nil
	}

	configMap := &corev1.ConfigMap{}
	if err := m.client.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cmName}, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get ConfigMap %s/%s for deletion: %w", cluster.Namespace, cmName, err)
	}

	if err := m.client.Delete(ctx, configMap); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	logger.Info("Deleted ConfigMap", "configmap", cmName, "pool", spec.Pool)
	return nil
}

func (m *Manager) ensureConfigMapWithName(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, cmName string, configContent string) error {
	if cmName == "" {
		return fmt.Errorf("config ConfigMap name is required")
	}

	configMap := corev1apply.ConfigMap(cmName, cluster.Namespace).
		WithLabels(resourceidentity.Labels(cluster)).
		WithData(map[string]string{configFileName: configContent})
	configMap.Kind = ptrTo("ConfigMap")
	configMap.APIVersion = ptrTo("v1")

	applyOpts := []client.ApplyOption{client.ForceOwnership, client.FieldOwner(constants.FieldOwnerOpenBaoOperator)}
	if err := m.client.Apply(ctx, configMap, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to ensure config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err))
		}
		return fmt.Errorf("failed to ensure config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	return nil
}

func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if sts, ok := obj.(*appsv1.StatefulSet); ok &&
		(cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen) {
		if err := resourceapply.PrepareOwned(obj, cluster, m.scheme); err != nil {
			return err
		}
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
		if err != nil {
			return fmt.Errorf("failed to convert StatefulSet to unstructured: %w", err)
		}
		if spec, ok := u["spec"].(map[string]any); ok {
			delete(spec, "updateStrategy")
		}
		unstructuredObj := &metav1unstructured.Unstructured{Object: u}
		gvk := sts.GetObjectKind().GroupVersionKind()
		if gvk.Empty() {
			gvk, err = m.client.GroupVersionKindFor(sts)
			if err != nil {
				return fmt.Errorf("failed to resolve GVK for StatefulSet: %w", err)
			}
		}
		unstructuredObj.SetGroupVersionKind(gvk)
		return resourceapply.ApplyConfiguration(ctx, m.client, obj, client.ApplyConfigurationFromUnstructured(unstructuredObj))
	}

	return resourceapply.ApplyOwned(ctx, m.client, m.scheme, cluster, obj)
}

func ptrTo[T any](v T) *T { return &v }
