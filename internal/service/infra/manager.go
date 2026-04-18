package infra

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/adapter/kube"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// Manager reconciles infra-owned identity resources and cleanup behavior for an OpenBaoCluster.
type Manager struct {
	client            client.Client
	reader            client.Reader
	scheme            *runtime.Scheme
	operatorNamespace string
	Platform          string
}

// Reconcile ensures infra-owned identity resources for an OpenBaoCluster.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.ensureServiceAccount(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureRBAC(ctx, logger, cluster); err != nil {
		return err
	}
	return nil
}

// NewManager constructs a Manager that uses the provided Kubernetes client.
// The scheme is used to set OwnerReferences on managed resources for garbage collection.
func NewManager(c client.Client, scheme *runtime.Scheme, operatorNamespace string, platform string) *Manager {
	return &Manager{
		client:            c,
		reader:            c,
		scheme:            scheme,
		operatorNamespace: operatorNamespace,
		Platform:          platform,
	}
}

// NewManagerWithReader constructs a Manager with a dedicated reader.
// Use this when the controller-runtime client is backed by a namespace-scoped cache
// (e.g. single-tenant mode) but the operator must still read cluster/system resources.
func NewManagerWithReader(c client.Client, r client.Reader, scheme *runtime.Scheme, operatorNamespace string, platform string) *Manager {
	m := NewManager(c, scheme, operatorNamespace, platform)
	if r != nil {
		m.reader = r
	}
	return m
}

// applyResource uses Server-Side Apply to create or update a Kubernetes resource.
// This eliminates the need for Get-then-Create-or-Update logic and manual diffing.
//
// The resource must have TypeMeta, ObjectMeta (with Name and Namespace), and the desired Spec set.
// Owner references are set automatically if the resource supports them.
func (m *Manager) applyResource(ctx context.Context, obj client.Object, cluster *openbaov1alpha1.OpenBaoCluster) error {
	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(cluster, obj, m.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Use Server-Side Apply with ForceOwnership to ensure the operator manages this resource
	applyConfig, err := kube.ToApplyConfiguration(obj, m.client)
	if err != nil {
		return fmt.Errorf("failed to convert object to ApplyConfiguration: %w", err)
	}

	// ROLLING UPGRADE SAFETY: For non-BlueGreen clusters, do not apply the StatefulSet
	// updateStrategy via SSA. The rolling upgrade manager owns RollingUpdate.Partition and
	// patches it via a strategic merge patch to orchestrate upgrades. Applying updateStrategy
	// here would risk clearing or overriding the partition and causing uncontrolled rollouts.
	if sts, ok := obj.(*appsv1.StatefulSet); ok &&
		(cluster.Spec.Upgrade == nil || cluster.Spec.Upgrade.Strategy != openbaov1alpha1.UpdateStrategyBlueGreen) {
		u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(sts)
		if err != nil {
			return fmt.Errorf("failed to convert StatefulSet to unstructured: %w", err)
		}

		if spec, ok := u["spec"].(map[string]any); ok {
			delete(spec, "updateStrategy")
		}

		unstructuredObj := &unstructured.Unstructured{Object: u}
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

	applyOpts := []client.ApplyOption{
		client.ForceOwnership,
		client.FieldOwner("openbao-operator"),
	}

	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		// Wrap transient Kubernetes API errors (rate limiting, temporary failures)
		if operatorerrors.IsTransientKubernetesAPI(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		// Check for conflict errors which are typically transient
		if apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}

// Cleanup handles resources that require special deletion logic beyond Kubernetes Garbage Collection.
//
// Most infrastructure resources (StatefulSet, Services, ConfigMaps, RBAC, etc.) have OwnerReferences
// set to the OpenBaoCluster and are automatically deleted by Kubernetes GC when the cluster is deleted.
// This method only handles resources that need explicit policy-based handling:
//   - PVCs: Only deleted when DeletionPolicy is DeletePVCs or DeleteAll
//
// Note: Secret preservation for DeletionPolicy=Retain is handled by the deletion controller
// (deletion.go orphanSecretsForRetention) which removes OwnerReferences before finalization.
//
// It is safe to call Cleanup multiple times; missing resources are treated as successfully deleted.
func (m *Manager) Cleanup(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster, policy openbaov1alpha1.DeletionPolicy) error {
	if policy == "" {
		policy = openbaov1alpha1.DeletionPolicyRetain
	}

	logger = logger.WithValues("deletionPolicy", string(policy))
	logger.Info("Processing cleanup for deleted OpenBaoCluster",
		"note", "Most resources are deleted by Kubernetes GC via OwnerReferences")

	// PVCs require explicit deletion based on policy because they are not owned by the
	// StatefulSet (they use volumeClaimTemplates which creates independent PVCs).
	// Kubernetes GC does not automatically delete these when the OpenBaoCluster is deleted.
	if policy == openbaov1alpha1.DeletionPolicyDeletePVCs || policy == openbaov1alpha1.DeletionPolicyDeleteAll {
		if err := m.deletePVCs(ctx, cluster); err != nil {
			return fmt.Errorf("failed to delete PVCs for OpenBaoCluster %s/%s: %w", cluster.Namespace, cluster.Name, err)
		}
		logger.Info("PVCs deleted per deletion policy")
	} else {
		logger.Info("Preserving PVCs per Retain policy")
	}

	return nil
}

// Helper functions used across multiple files

func infraLabels(cluster *openbaov1alpha1.OpenBaoCluster) map[string]string {
	return map[string]string{
		constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
		constants.LabelAppInstance:    cluster.Name,
		constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
		constants.LabelOpenBaoCluster: cluster.Name,
	}
}

// statefulSetNameWithRevision returns the StatefulSet name for a given revision.
// If rev is empty, returns the cluster name (for backward compatibility).
// Otherwise, returns "<cluster-name>-<revision>".
