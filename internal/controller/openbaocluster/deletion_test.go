package openbaocluster

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
)

func TestOrphanRetentionSecrets(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, openbaov1alpha1.AddToScheme(scheme))

	clusterUID := types.UID("test-cluster-uid")
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			UID:       clusterUID,
		},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Version:        "2.4.4",
			Image:          "openbao/openbao:2.4.4",
			Replicas:       3,
			DeletionPolicy: openbaov1alpha1.DeletionPolicyRetain,
		},
	}

	// Build OwnerReference that would be set by the controller
	ownerRef := metav1.OwnerReference{
		APIVersion: "openbao.org/v1alpha1",
		Kind:       "OpenBaoCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: ptrBool(true),
	}

	tests := []struct {
		name                   string
		existingSecrets        []*corev1.Secret
		expectOrphanedSecrets  []string
		expectRemainingSecrets []string
	}{
		{
			name: "orphans unseal-key and root-token secrets",
			existingSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-cluster" + constants.SuffixUnsealKey,
						Namespace:       "default",
						OwnerReferences: []metav1.OwnerReference{ownerRef},
					},
					Data: map[string][]byte{"key": []byte("unseal-key-data")},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-cluster" + constants.SuffixRootToken,
						Namespace:       "default",
						OwnerReferences: []metav1.OwnerReference{ownerRef},
					},
					Data: map[string][]byte{"token": []byte("root-token-data")},
				},
			},
			expectOrphanedSecrets: []string{
				"test-cluster" + constants.SuffixUnsealKey,
				"test-cluster" + constants.SuffixRootToken,
			},
		},
		{
			name: "handles missing secrets gracefully",
			existingSecrets: []*corev1.Secret{
				// Only unseal-key exists, root-token is missing
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-cluster" + constants.SuffixUnsealKey,
						Namespace:       "default",
						OwnerReferences: []metav1.OwnerReference{ownerRef},
					},
					Data: map[string][]byte{"key": []byte("unseal-key-data")},
				},
			},
			expectOrphanedSecrets: []string{
				"test-cluster" + constants.SuffixUnsealKey,
			},
		},
		{
			name: "skips already orphaned secrets",
			existingSecrets: []*corev1.Secret{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:            "test-cluster" + constants.SuffixUnsealKey,
						Namespace:       "default",
						OwnerReferences: nil, // Already orphaned
					},
					Data: map[string][]byte{"key": []byte("unseal-key-data")},
				},
			},
			expectOrphanedSecrets:  []string{},
			expectRemainingSecrets: []string{"test-cluster" + constants.SuffixUnsealKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build fake client with existing secrets
			objects := make([]runtime.Object, 0, len(tt.existingSecrets)+1)
			objects = append(objects, cluster)
			for _, s := range tt.existingSecrets {
				objects = append(objects, s)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objects...).
				Build()

			reconciler := &OpenBaoClusterReconciler{
				Client: fakeClient,
				Scheme: scheme,
			}

			ctx := context.Background()
			logger := log.FromContext(ctx)

			err := reconciler.orphanRetentionSecrets(ctx, logger, cluster)
			require.NoError(t, err)

			// Verify orphaned secrets have no OwnerReferences
			for _, secretName := range tt.expectOrphanedSecrets {
				secret := &corev1.Secret{}
				err := fakeClient.Get(ctx, types.NamespacedName{
					Namespace: "default",
					Name:      secretName,
				}, secret)
				require.NoError(t, err, "expected secret %s to exist", secretName)
				assert.Empty(t, secret.OwnerReferences, "expected secret %s to have no OwnerReferences", secretName)
			}

			// Verify remaining secrets still exist
			for _, secretName := range tt.expectRemainingSecrets {
				secret := &corev1.Secret{}
				err := fakeClient.Get(ctx, types.NamespacedName{
					Namespace: "default",
					Name:      secretName,
				}, secret)
				require.NoError(t, err, "expected secret %s to exist", secretName)
			}
		})
	}
}

func TestHasOwnerReference(t *testing.T) {
	uid := types.UID("test-uid")

	tests := []struct {
		name     string
		refs     []metav1.OwnerReference
		uid      types.UID
		expected bool
	}{
		{
			name:     "returns true when UID matches",
			refs:     []metav1.OwnerReference{{UID: uid}},
			uid:      uid,
			expected: true,
		},
		{
			name:     "returns false when UID does not match",
			refs:     []metav1.OwnerReference{{UID: "other-uid"}},
			uid:      uid,
			expected: false,
		},
		{
			name:     "returns false when empty",
			refs:     nil,
			uid:      uid,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: tt.refs,
				},
			}
			assert.Equal(t, tt.expected, hasOwnerReference(secret, tt.uid))
		})
	}
}

func ptrBool(b bool) *bool {
	return &b
}
