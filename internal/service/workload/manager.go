package workload

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1unstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
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

	statefulSetCluster := clusterForStatefulSetSpec(cluster, spec)
	if cluster != nil && statefulSetCluster != cluster {
		logger.V(1).Info(
			"Applying staged StatefulSet replica count",
			"statefulset", spec.Name,
			"clusterDesiredReplicas", cluster.Spec.Replicas,
			"appliedStatefulSetReplicas", spec.Replicas,
		)
	}

	if err := m.EnsureStatefulSetWithRevision(ctx, logger, statefulSetCluster, configContent, spec.Image, spec.InitContainerImage, spec.Revision, spec.DisableSelfInit); err != nil {
		return err
	}

	if err := m.ensurePodDisruptionBudget(ctx, logger, statefulSetCluster); err != nil {
		return err
	}

	return nil
}

func clusterForStatefulSetSpec(cluster *openbaov1alpha1.OpenBaoCluster, spec StatefulSetSpec) *openbaov1alpha1.OpenBaoCluster {
	if cluster == nil || spec.Replicas == cluster.Spec.Replicas {
		return cluster
	}

	statefulSetCluster := cluster.DeepCopy()
	statefulSetCluster.Spec.Replicas = spec.Replicas
	return statefulSetCluster
}

func (m *Manager) ensureConfigMapWithRevision(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, revision string, configContent string) error {
	if revision == "" {
		return nil
	}

	return m.ensureConfigMapWithName(ctx, cluster, configMapNameWithRevision(cluster, revision), configContent)
}

func (m *Manager) ensureConfigMapWithName(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, cmName string, configContent string) error {
	if cmName == "" {
		return fmt.Errorf("config ConfigMap name is required")
	}

	configMap := corev1apply.ConfigMap(cmName, cluster.Namespace).
		WithLabels(infraLabels(cluster)).
		WithData(map[string]string{configFileName: configContent})
	configMap.Kind = ptrTo("ConfigMap")
	configMap.APIVersion = ptrTo("v1")

	applyOpts := []client.ApplyOption{client.ForceOwnership, client.FieldOwner("openbao-operator")}
	if err := m.client.Apply(ctx, configMap, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to ensure config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err))
		}
		return fmt.Errorf("failed to ensure config ConfigMap %s/%s: %w", cluster.Namespace, cmName, err)
	}

	return nil
}

func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	if sts, ok := obj.(*appsv1.StatefulSet); ok &&
		(cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen) {
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
		applyConfig = client.ApplyConfigurationFromUnstructured(unstructuredObj)
	}

	applyOpts := []client.ApplyOption{client.ForceOwnership, client.FieldOwner("openbao-operator")}
	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) || apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

func ptrTo[T any](v T) *T { return &v }
