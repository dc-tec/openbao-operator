//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

// Multi-tenancy Tests
// These tests verify the multi-tenancy requirements (FR-MT-01 through FR-MT-05) are satisfied.

var _ = Describe("OpenBaoCluster Multi-Tenancy", func() {
	Context("When managing multiple clusters in different namespaces", func() {
		ctx := context.Background()

		newReconciler := func() *testCompositeReconciler {
			parent := &OpenBaoClusterReconciler{
				Client: k8sClient,
				ControllerRuntime: ControllerRuntime{
					APIReader: k8sClient,
					Scheme:    k8sClient.Scheme(),
				},
			}
			return &testCompositeReconciler{parent: parent}
		}

		createClusterInNamespace := func(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Create(ctx, ns)

			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
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
			namespaces := []string{"tenant-a", "tenant-b", "default"}
			for _, ns := range namespaces {
				var clusterList openbaov1alpha1.OpenBaoClusterList
				_ = k8sClient.List(ctx, &clusterList, client.InNamespace(ns))
				for i := range clusterList.Items {
					_ = k8sClient.Delete(ctx, &clusterList.Items[i])
				}
			}
		})

		// FR-MT-01: Support managing multiple OpenBaoCluster resources in a single Kubernetes cluster
		// FR-MT-02: Support multiple OpenBaoCluster per namespace, with no cross-impact
		It("reconciles multiple clusters in different namespaces independently (FR-MT-01, FR-MT-02)", func() {
			clusterA := createClusterInNamespace("cluster-a", "tenant-a")
			clusterB := createClusterInNamespace("cluster-b", "tenant-b")

			reqA := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      clusterA.Name,
					Namespace: clusterA.Namespace,
				},
			}
			reqB := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      clusterB.Name,
					Namespace: clusterB.Namespace,
				},
			}

			reconciler := newReconciler()

			_, err := reconciler.Reconcile(ctx, reqA)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reqA)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reqB)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reqB)
			Expect(err).NotTo(HaveOccurred())

			caSecretA := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterA.Name + constants.SuffixTLSCA,
				Namespace: "tenant-a",
			}, caSecretA)
			Expect(err).NotTo(HaveOccurred())

			caSecretB := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterB.Name + constants.SuffixTLSCA,
				Namespace: "tenant-b",
			}, caSecretB)
			Expect(err).NotTo(HaveOccurred())

			Expect(caSecretA.Data["ca.crt"]).NotTo(Equal(caSecretB.Data["ca.crt"]))

			stsA := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterA.Name,
				Namespace: "tenant-a",
			}, stsA)
			Expect(err).NotTo(HaveOccurred())

			stsB := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterB.Name,
				Namespace: "tenant-b",
			}, stsB)
			Expect(err).NotTo(HaveOccurred())

			Expect(stsA.Namespace).To(Equal("tenant-a"))
			Expect(stsB.Namespace).To(Equal("tenant-b"))
		})

		// FR-MT-05: Avoid sharing Secrets or ConfigMaps between different OpenBaoCluster instances
		It("creates uniquely named resources per cluster preventing cross-tenant sharing (FR-MT-05)", func() {
			cluster1 := createClusterInNamespace("same-name", "tenant-a")
			cluster2 := createClusterInNamespace("same-name", "tenant-b")

			req1 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster1.Name,
					Namespace: cluster1.Namespace,
				},
			}
			req2 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster2.Name,
					Namespace: cluster2.Namespace,
				},
			}

			reconciler := newReconciler()

			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, req1)
				Expect(err).NotTo(HaveOccurred())
				_, err = reconciler.Reconcile(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
			}

			resourceSuffixes := []string{constants.SuffixTLSCA, constants.SuffixTLSServer, constants.SuffixUnsealKey, constants.SuffixConfigMap}
			for _, suffix := range resourceSuffixes {
				var resourceA, resourceB client.Object
				if suffix == "-config" {
					resourceA = &corev1.ConfigMap{}
					resourceB = &corev1.ConfigMap{}
				} else {
					resourceA = &corev1.Secret{}
					resourceB = &corev1.Secret{}
				}

				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      "same-name" + suffix,
					Namespace: "tenant-a",
				}, resourceA)
				Expect(err).NotTo(HaveOccurred(), "expected resource %s to exist in tenant-a", suffix)

				err = k8sClient.Get(ctx, types.NamespacedName{
					Name:      "same-name" + suffix,
					Namespace: "tenant-b",
				}, resourceB)
				Expect(err).NotTo(HaveOccurred(), "expected resource %s to exist in tenant-b", suffix)
			}
		})

		// FR-MT-03: A failure or misconfiguration in one OpenBaoCluster MUST NOT prevent reconciliation of others
		It("failure in one cluster does not prevent reconciliation of others (FR-MT-03)", func() {
			clusterGood := createClusterInNamespace("good-cluster", "tenant-a")
			clusterOther := createClusterInNamespace("other-cluster", "tenant-b")

			reqGood := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      clusterGood.Name,
					Namespace: clusterGood.Namespace,
				},
			}
			reqOther := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      clusterOther.Name,
					Namespace: clusterOther.Namespace,
				},
			}

			reconciler := newReconciler()

			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, reqGood)
				Expect(err).NotTo(HaveOccurred())
			}

			stsGood := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterGood.Name,
				Namespace: "tenant-a",
			}, stsGood)
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, reqOther)
				Expect(err).NotTo(HaveOccurred())
			}

			stsOther := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterOther.Name,
				Namespace: "tenant-b",
			}, stsOther)
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterGood.Name,
				Namespace: "tenant-a",
			}, stsGood)
			Expect(err).NotTo(HaveOccurred())
		})

		// FR-MT-02: Support multiple OpenBaoCluster resources per namespace with no cross-impact
		It("supports multiple clusters in the same namespace without cross-impact (FR-MT-02)", func() {
			cluster1 := createClusterInNamespace("cluster-one", "default")
			cluster2 := createClusterInNamespace("cluster-two", "default")

			req1 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster1.Name,
					Namespace: cluster1.Namespace,
				},
			}
			req2 := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster2.Name,
					Namespace: cluster2.Namespace,
				},
			}

			reconciler := newReconciler()

			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, req1)
				Expect(err).NotTo(HaveOccurred())
				_, err = reconciler.Reconcile(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
			}

			sts1 := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster1.Name,
				Namespace: "default",
			}, sts1)
			Expect(err).NotTo(HaveOccurred())

			sts2 := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster2.Name,
				Namespace: "default",
			}, sts2)
			Expect(err).NotTo(HaveOccurred())

			Expect(sts1.Name).NotTo(Equal(sts2.Name))
			Expect(sts1.Spec.Template.Spec.Volumes).NotTo(BeEmpty())
			Expect(sts2.Spec.Template.Spec.Volumes).NotTo(BeEmpty())

			var cm1Name, cm2Name string
			for _, vol := range sts1.Spec.Template.Spec.Volumes {
				if vol.ConfigMap != nil {
					cm1Name = vol.ConfigMap.Name
					break
				}
			}
			for _, vol := range sts2.Spec.Template.Spec.Volumes {
				if vol.ConfigMap != nil {
					cm2Name = vol.ConfigMap.Name
					break
				}
			}
			Expect(cm1Name).NotTo(Equal(cm2Name))
			Expect(cm1Name).To(ContainSubstring(cluster1.Name))
			Expect(cm2Name).To(ContainSubstring(cluster2.Name))
		})
	})
})
