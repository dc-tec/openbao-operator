package deletionops

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

func TestHandleDefaultsRetentionSecrets(t *testing.T) {
	t.Parallel()

	cluster := newCleanupTestCluster("retain-defaults")
	cluster.UID = types.UID("retain-defaults-uid")
	cluster.Spec.DeletionPolicy = openbaov1alpha1.DeletionPolicyRetain

	ownerRef := metav1.OwnerReference{
		APIVersion: openbaov1alpha1.GroupVersion.String(),
		Kind:       "OpenBaoCluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
		Controller: ptrBool(true),
	}
	unsealSecret := newRetentionTestSecret(cluster, cluster.Name+constants.SuffixUnsealKey, ownerRef)
	rootTokenSecret := newRetentionTestSecret(cluster, cluster.Name+constants.SuffixRootToken, ownerRef)
	kubeClient := newCleanupTestClient(t, cluster, unsealSecret, rootTokenSecret)

	if err := Handle(context.Background(), logr.Discard(), Dependencies{Client: kubeClient}, cluster); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	for _, secretName := range []string{unsealSecret.Name, rootTokenSecret.Name} {
		secret := &corev1.Secret{}
		if err := kubeClient.Get(context.Background(), client.ObjectKey{Namespace: cluster.Namespace, Name: secretName}, secret); err != nil {
			t.Fatalf("Get(%s) error = %v", secretName, err)
		}
		if len(secret.OwnerReferences) != 0 {
			t.Fatalf("secret %s ownerReferences = %#v, want empty", secretName, secret.OwnerReferences)
		}
	}
}

func newRetentionTestSecret(cluster *openbaov1alpha1.OpenBaoCluster, name string, ownerRef metav1.OwnerReference) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       cluster.Namespace,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Data: map[string][]byte{"value": []byte("redacted")},
	}
}
