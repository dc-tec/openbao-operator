package infra

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func (m *Manager) ensureHeadlessService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	svcName := headlessServiceName(cluster)

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector:                 podSelectorLabels(cluster),
			Ports: []corev1.ServicePort{
				{
					Name:     "api",
					Port:     constants.PortAPI,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure headless Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	return nil
}

// ensureExternalService manages the external-facing Service for the OpenBaoCluster using Server-Side Apply.
func (m *Manager) ensureExternalService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	serviceCfg := cluster.Spec.Service
	ingressCfg := cluster.Spec.Ingress
	gatewayCfg := cluster.Spec.Gateway

	needsService := serviceCfg != nil ||
		(ingressCfg != nil && ingressCfg.Enabled) ||
		(gatewayCfg != nil && gatewayCfg.Enabled)
	svcName := externalServiceName(cluster)

	// If service is not needed, check if it exists and delete it
	if !needsService {
		// Delete main external service
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, svcName); err != nil {
			return fmt.Errorf("failed to delete external Service %s/%s: %w", cluster.Namespace, svcName, err)
		}
		// Delete any blue/green-specific services that might exist from previous runs
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameBlue(cluster)); err != nil {
			return fmt.Errorf("failed to delete blue external Service %s/%s: %w", cluster.Namespace, externalServiceNameBlue(cluster), err)
		}
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameGreen(cluster)); err != nil {
			return fmt.Errorf("failed to delete green external Service %s/%s: %w", cluster.Namespace, externalServiceNameGreen(cluster), err)
		}
		return nil
	}

	// Build the desired service spec
	svcType := corev1.ServiceTypeClusterIP
	annotations := map[string]string{}
	if serviceCfg != nil {
		if serviceCfg.Type != "" {
			svcType = serviceCfg.Type
		}
		for k, v := range serviceCfg.Annotations {
			annotations[k] = v
		}
	}

	selectorLabels := podSelectorLabels(cluster)
	if activeRevision := BlueGreenActiveRevision(cluster); activeRevision != "" {
		selectorLabels[constants.LabelOpenBaoRevision] = activeRevision
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        svcName,
			Namespace:   cluster.Namespace,
			Labels:      infraLabels(cluster),
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:     "api",
					Port:     constants.PortAPI,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure external Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	// Gateway-weighted traffic switching was removed. Clean up any stale Services
	// from previous iterations that used revision-specific HTTPRoute backends.
	if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameBlue(cluster)); err != nil {
		return fmt.Errorf("failed to delete stale blue external Service: %w", err)
	}
	if err := m.deleteServiceIfExists(ctx, cluster.Namespace, externalServiceNameGreen(cluster)); err != nil {
		return fmt.Errorf("failed to delete stale green external Service: %w", err)
	}

	return nil
}

// ensureACMEChallengeService manages a dedicated Service for ACME validation in ACME TLS mode.
//
// In ACME mode, OpenBao must complete ACME challenges before it can become Ready (it has no
// serving certificate yet). Most Kubernetes Services only publish ready pod endpoints, creating
// a circular dependency. This Service sets PublishNotReadyAddresses so ACME validators can reach
// pods while they are still initializing.
//
// The Service exposes standard ACME ports (80/443) and forwards to the OpenBao listener port
// (8200). This is particularly useful for private ACME CAs running inside the cluster.
func (m *Manager) ensureACMEChallengeService(ctx context.Context, _ logr.Logger, cluster *openbaov1alpha1.OpenBaoCluster) error {
	enabled := usesACMEMode(cluster)
	svcName := acmeServiceName(cluster)

	if !enabled {
		if err := m.deleteServiceIfExists(ctx, cluster.Namespace, svcName); err != nil {
			return fmt.Errorf("failed to delete ACME challenge Service %s/%s: %w", cluster.Namespace, svcName, err)
		}
		return nil
	}

	selectorLabels := podSelectorLabels(cluster)
	if activeRevision := BlueGreenActiveRevision(cluster); activeRevision != "" {
		selectorLabels[constants.LabelOpenBaoRevision] = activeRevision
	}

	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      svcName,
			Namespace: cluster.Namespace,
			Labels:    infraLabels(cluster),
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			PublishNotReadyAddresses: true,
			Selector:                 selectorLabels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http-80",
					Protocol:   corev1.ProtocolTCP,
					Port:       80,
					TargetPort: intstr.FromInt(constants.PortAPI),
				},
				{
					Name:       "https-443",
					Protocol:   corev1.ProtocolTCP,
					Port:       443,
					TargetPort: intstr.FromInt(constants.PortAPI),
				},
			},
		},
	}

	if err := m.applyResource(ctx, service, cluster); err != nil {
		return fmt.Errorf("failed to ensure ACME challenge Service %s/%s: %w", cluster.Namespace, svcName, err)
	}

	return nil
}

// deleteServiceIfExists deletes the Service with the given namespace/name if it exists.
func (m *Manager) deleteServiceIfExists(ctx context.Context, namespace, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}

	service := &corev1.Service{}
	err := m.client.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, service)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := m.client.Delete(ctx, service); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}
