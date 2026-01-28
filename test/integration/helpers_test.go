//go:build integration
// +build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
)

func requireAdmissionDenied(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected admission to deny the request, got nil error")
	}

	if apierrors.IsForbidden(err) {
		return
	}

	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		status := apiStatus.Status()
		if status.Code == http.StatusForbidden || status.Reason == metav1.StatusReasonForbidden {
			return
		}
	}

	if strings.Contains(err.Error(), "is forbidden") {
		return
	}

	t.Fatalf("expected Forbidden admission error, got %T: %v", err, err)
}

func newTestNamespace(t *testing.T) string {
	t.Helper()

	base := strings.ToLower(t.Name())
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "_", "-")
	if len(base) > 40 {
		base = base[:40]
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("it-%s-%d", base, time.Now().UnixNano()),
		},
	}
	if err := k8sClient.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}

	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})

	return ns.Name
}

func createTLSSecret(t *testing.T, namespace, clusterName string) {
	t.Helper()

	secretName := clusterName + constants.SuffixTLSServer
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-cert"),
			"tls.key": []byte("test-key"),
			"ca.crt":  []byte("test-ca"),
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create TLS secret: %v", err)
	}
}

func createCASecret(t *testing.T, namespace, clusterName string, caPEM []byte) {
	t.Helper()

	secretName := clusterName + constants.SuffixTLSCA
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"ca.crt": caPEM,
		},
	}

	if err := k8sClient.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create CA secret: %v", err)
	}
}

func createMinimalCluster(t *testing.T, namespace, name string) *openbaov1alpha1.OpenBaoCluster {
	t.Helper()

	cluster := newMinimalClusterObj(namespace, name)

	if err := k8sClient.Create(ctx, cluster); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create OpenBaoCluster: %v", err)
	}

	return cluster
}

func newMinimalClusterObj(namespace, name string) *openbaov1alpha1.OpenBaoCluster {
	return &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:  "2.4.4",
			Image:    "openbao/openbao:2.4.4",
			Replicas: 3,
			TLS: openbaov1alpha1.TLSConfig{
				Enabled:        true,
				RotationPeriod: "720h",
			},
			Storage: openbaov1alpha1.StorageConfig{
				Size: "10Gi",
			},
			InitContainer: &openbaov1alpha1.InitContainerConfig{
				Image: "openbao/openbao-init:latest",
			},
		},
	}
}

func updateClusterStatus(t *testing.T, cluster *openbaov1alpha1.OpenBaoCluster, mutate func(*openbaov1alpha1.OpenBaoClusterStatus)) {
	t.Helper()

	var latest openbaov1alpha1.OpenBaoCluster
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name}, &latest); err != nil {
		t.Fatalf("get OpenBaoCluster for status update: %v", err)
	}
	mutate(&latest.Status)
	if err := k8sClient.Status().Update(ctx, &latest); err != nil {
		t.Fatalf("update OpenBaoCluster status: %v", err)
	}
	cluster.Status = latest.Status
}

func discardLogger() logr.Logger {
	return logr.Discard()
}

// newTestStatefulSetSpec creates a minimal StatefulSetSpec for testing.
func newTestStatefulSetSpec(cluster *openbaov1alpha1.OpenBaoCluster) infra.StatefulSetSpec {
	return infra.StatefulSetSpec{
		Name:               cluster.Name,
		Revision:           "",
		Image:              cluster.Spec.Image,
		InitContainerImage: "",
		Replicas:           cluster.Spec.Replicas,
		ConfigHash:         "",
		DisableSelfInit:    false,
		SkipReconciliation: false,
	}
}
