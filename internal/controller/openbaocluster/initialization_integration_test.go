//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
)

var _ = Describe("OpenBaoCluster Initialization", func() {
	Context("When reconciling initialization state", func() {
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

		createMinimalCluster := func(name string) *openbaov1alpha1.OpenBaoCluster {
			ensureTenantNamespaceProvisioned(ctx, "default")
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Spec: openbaov1alpha1.OpenBaoClusterSpec{
					Version:  "2.4.4",
					Image:    "openbao/openbao:2.4.4",
					Replicas: 3,
					Profile:  openbaov1alpha1.ProfileDevelopment,
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
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			return cluster
		}

		AfterEach(func() {
			var clusterList openbaov1alpha1.OpenBaoClusterList
			err := k8sClient.List(ctx, &clusterList)
			Expect(err).NotTo(HaveOccurred())
			for i := range clusterList.Items {
				_ = k8sClient.Delete(ctx, &clusterList.Items[i])
			}
		})

		It("creates StatefulSet with 1 replica when cluster is not initialized", func() {
			cluster := createMinimalCluster("test-init-replicas")
			cluster.Status.Initialized = false

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			_, err := newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			_, err = newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())
			Expect(sts.Spec.Replicas).NotTo(BeNil())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		})

		It("scales StatefulSet to desired replicas after initialization", func() {
			cluster := createMinimalCluster("test-init-scale")
			cluster.Status.Initialized = false
			cluster.Spec.Replicas = 3

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler()

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			Eventually(func(g Gomega) {
				latest := &openbaov1alpha1.OpenBaoCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				}, latest)
				g.Expect(err).NotTo(HaveOccurred())

				original := latest.DeepCopy()
				latest.Status.Initialized = true
				err = k8sClient.Status().Patch(ctx, latest, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})

		It("skips initialization when cluster is already initialized", func() {
			cluster := createMinimalCluster("test-init-skip")
			original := cluster.DeepCopy()
			cluster.Status.Initialized = true
			Expect(k8sClient.Status().Patch(ctx, cluster, client.MergeFrom(original))).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler()

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			updated := &openbaov1alpha1.OpenBaoCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Status.Initialized).To(BeTrue())
		})
	})
})
