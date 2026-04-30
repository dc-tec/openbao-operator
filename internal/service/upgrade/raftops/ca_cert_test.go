package raftops

import (
	"context"
	"strings"
	"testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestLoadClusterCACert(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{Enabled: true},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1-tls-ca"},
		Data: map[string][]byte{
			"ca.crt": []byte("cert"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	got, err := LoadClusterCACert(ctx, c, cluster)
	require.NoError(t, err)
	require.Equal(t, []byte("cert"), got)
}

func TestLoadClusterCACert_MissingKey(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			TLS: openbaov1alpha1.TLSConfig{Enabled: true},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1-tls-ca"},
		Data:       map[string][]byte{},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	_, err := LoadClusterCACert(ctx, c, cluster)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), `trust bundle key "ca.crt" missing`))
}
