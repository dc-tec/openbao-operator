package networking

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
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
)

// Manager reconciles network-owned resources and validations for an OpenBaoCluster.
type Manager struct {
	client            client.Client
	reader            client.Reader
	scheme            *runtime.Scheme
	operatorNamespace string
	Platform          string
}

func NewManager(c client.Client, scheme *runtime.Scheme, operatorNamespace string, platform string) *Manager {
	return &Manager{
		client:            c,
		reader:            c,
		scheme:            scheme,
		operatorNamespace: operatorNamespace,
		Platform:          platform,
	}
}

func NewManagerWithReader(c client.Client, r client.Reader, scheme *runtime.Scheme, operatorNamespace string, platform string) *Manager {
	m := NewManager(c, scheme, operatorNamespace, platform)
	if r != nil {
		m.reader = r
	}
	return m
}

// Reconcile ensures network-owned resources are aligned with the desired state for the cluster.
func (m *Manager) Reconcile(ctx context.Context, logger logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	if err := m.runACMEPreflight(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureHeadlessService(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureExternalService(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureACMEChallengeService(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureIngress(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureHTTPRoute(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureTLSRoute(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureGatewayCAConfigMap(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureBackendTLSPolicy(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureNetworkPolicy(ctx, logger, cluster); err != nil {
		return err
	}
	if err := m.ensureJobNetworkPolicy(ctx, logger, cluster); err != nil {
		return err
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

	applyOpts := []client.ApplyOption{client.ForceOwnership, client.FieldOwner("openbao-operator")}
	if err := m.client.Apply(ctx, applyConfig, applyOpts...); err != nil {
		if operatorerrors.IsTransientKubernetesAPI(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		if apierrors.IsConflict(err) {
			return operatorerrors.WrapTransientKubernetesAPI(fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err))
		}
		return fmt.Errorf("failed to apply resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	}

	return nil
}
