package upgrade

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReadCACertSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1"},
		Data: map[string][]byte{
			"ca.crt": []byte("cert"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	got, err := ReadCACertSecret(ctx, c, types.NamespacedName{Namespace: "ns1", Name: "s1"})
	require.NoError(t, err)
	require.Equal(t, []byte("cert"), got)
}

func TestReadCACertSecret_MissingKey(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	ctx := context.Background()

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "s1"},
		Data:       map[string][]byte{},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	_, err := ReadCACertSecret(ctx, c, types.NamespacedName{Namespace: "ns1", Name: "s1"})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCACertMissing))
}
