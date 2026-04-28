//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

var _ = Describe("OpenBaoCluster Reconcile", func() {
	Context("When reconciling cluster state", func() {
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

		createMinimalCluster := func(name string, paused bool) *openbaov1alpha1.OpenBaoCluster {
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
					Paused:   paused,
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

		It("adds a finalizer to new clusters", func() {
			cluster := createMinimalCluster("test-finalizer", false)

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			_, err := newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			updated := &openbaov1alpha1.OpenBaoCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated.Finalizers).To(ContainElement(openbaov1alpha1.OpenBaoClusterFinalizer))
		})

		It("reconciles an unpaused cluster and updates status conditions", func() {
			cluster := createMinimalCluster("test-unpaused", false)
			cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
			}
			Expect(k8sClient.Update(ctx, cluster)).To(Succeed())

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

			updated := &openbaov1alpha1.OpenBaoCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseInitializing))

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
			Expect(available.Reason).To(Equal("NoReplicasReady"))

			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
			Expect(degraded.Reason).To(Equal("Reconciling"))

			upgrading := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionUpgrading))
			Expect(upgrading).NotTo(BeNil())
			Expect(upgrading.Status).To(Equal(metav1.ConditionFalse))

			backingUp := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionBackingUp))
			Expect(backingUp).NotTo(BeNil())
			Expect(backingUp.Status).To(Equal(metav1.ConditionFalse))

			tlsReady := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			Expect(tlsReady).NotTo(BeNil())
			Expect(tlsReady.Status).To(Equal(metav1.ConditionTrue))
			Expect(tlsReady.Reason).To(Equal("Ready"))
		})

		It("honors spec.paused and sets paused conditions", func() {
			cluster := createMinimalCluster("test-paused", true)

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

			updated := &openbaov1alpha1.OpenBaoCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Status.Phase).To(Equal(openbaov1alpha1.ClusterPhaseInitializing))

			available := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionAvailable))
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionUnknown))
			Expect(available.Reason).To(Equal("Paused"))

			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
			Expect(degraded.Reason).To(Equal("Paused"))

			tlsReady := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionTLSReady))
			Expect(tlsReady).NotTo(BeNil())
			Expect(tlsReady.Status).To(Equal(metav1.ConditionUnknown))
			Expect(tlsReady.Reason).To(Equal("Paused"))
		})

		It("does not create workload resources when cluster is paused", func() {
			cluster := createMinimalCluster("test-paused-no-tls", true)

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

			caSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name + constants.SuffixTLSCA,
				Namespace: cluster.Namespace,
			}, caSecret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			serverSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name + constants.SuffixTLSServer,
				Namespace: cluster.Namespace,
			}, serverSecret)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			statefulSet := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, statefulSet)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			headlessService := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, headlessService)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			configMap := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name + constants.SuffixConfigMap,
				Namespace: cluster.Namespace,
			}, configMap)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("creates CA and server TLS Secrets for a new cluster", func() {
			cluster := createMinimalCluster("test-tls-secrets", false)

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

			caSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name + constants.SuffixTLSCA,
				Namespace: cluster.Namespace,
			}, caSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(caSecret.Data).To(HaveKey("ca.crt"))
			Expect(caSecret.Data).To(HaveKey("ca.key"))

			serverSecret := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name + constants.SuffixTLSServer,
				Namespace: cluster.Namespace,
			}, serverSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(serverSecret.Data).To(HaveKey("tls.crt"))
			Expect(serverSecret.Data).To(HaveKey("tls.key"))
			Expect(serverSecret.Data).To(HaveKey("ca.crt"))
		})
	})
})
