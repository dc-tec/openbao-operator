//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	kubeadapter "github.com/dc-tec/openbao-operator/internal/adapter/kube"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

var _ = Describe("OpenBaoCluster GitOps contract", func() {
	ctx := context.Background()

	newReconciler := func() *testCompositeReconciler {
		parent := &OpenBaoClusterReconciler{
			Client: k8sClient,
			ControllerRuntime: ControllerRuntime{
				APIReader: k8sClient,
				Scheme:    k8sClient.Scheme(),
			},
			ImageVerificationRuntime: ImageVerificationRuntime{
				ImageVerifier: security.NewImageVerifier(logr.Discard(), k8sClient, nil),
			},
		}
		return &testCompositeReconciler{parent: parent}
	}

	desiredCluster := func(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
		return &openbaov1alpha1.OpenBaoCluster{
			TypeMeta: metav1.TypeMeta{
				APIVersion: openbaov1alpha1.GroupVersion.String(),
				Kind:       "OpenBaoCluster",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
			Spec: openbaov1alpha1.OpenBaoClusterSpec{
				Profile:  openbaov1alpha1.ProfileDevelopment,
				Version:  "2.4.4",
				Image:    "openbao/openbao:2.4.4",
				Replicas: 3,
				InitContainer: &openbaov1alpha1.InitContainerConfig{
					Image: "openbao/openbao-init:latest",
				},
				TLS: openbaov1alpha1.TLSConfig{
					Enabled:        true,
					RotationPeriod: "720h",
				},
				SelfInit: &openbaov1alpha1.SelfInitConfig{
					Enabled: true,
				},
				Storage: openbaov1alpha1.StorageConfig{
					Size: "10Gi",
				},
				DeletionPolicy: openbaov1alpha1.DeletionPolicyDeleteAll,
			},
		}
	}

	normalizeStringMap := func(in map[string]string) map[string]string {
		if in == nil {
			return map[string]string{}
		}
		out := make(map[string]string, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}

	AfterEach(func() {
		var clusterList openbaov1alpha1.OpenBaoClusterList
		err := k8sClient.List(ctx, &clusterList)
		Expect(err).NotTo(HaveOccurred())
		for i := range clusterList.Items {
			_ = k8sClient.Delete(ctx, &clusterList.Items[i])
		}
	})

	It("does not mutate user-owned spec, labels, or annotations across repeated server-side applies", func() {
		const (
			namespace = "default"
			name      = "gitops-contract"
		)
		ensureTenantNamespaceProvisioned(ctx, namespace)

		key := types.NamespacedName{Namespace: namespace, Name: name}
		req := reconcile.Request{NamespacedName: key}

		for i := 0; i < 3; i++ {
			desired := desiredCluster(name, namespace)
			applyConfig, err := kubeadapter.ToApplyConfiguration(desired, k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Apply(ctx, applyConfig, client.FieldOwner("argocd"))).To(Succeed())

			want := &openbaov1alpha1.OpenBaoCluster{}
			Expect(k8sClient.Get(ctx, key, want)).To(Succeed())

			_, err = newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			got := &openbaov1alpha1.OpenBaoCluster{}
			Expect(k8sClient.Get(ctx, key, got)).To(Succeed())
			Expect(got.Spec).To(Equal(want.Spec), "operator must not mutate spec")
			Expect(normalizeStringMap(got.Labels)).To(Equal(normalizeStringMap(want.Labels)), "operator must not add labels")
			Expect(normalizeStringMap(got.Annotations)).To(Equal(normalizeStringMap(want.Annotations)), "operator must not add annotations")
			Expect(got.Finalizers).To(ContainElement(openbaov1alpha1.OpenBaoClusterFinalizer))
		}

		secret := &corev1.Secret{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name + constants.SuffixTLSCA}, secret)).To(Succeed())
	})
})
