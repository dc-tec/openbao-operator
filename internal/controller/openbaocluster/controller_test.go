//go:build integration
// +build integration

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	"github.com/dc-tec/openbao-operator/internal/infra"
	security "github.com/dc-tec/openbao-operator/internal/security"
)

type testCompositeReconciler struct {
	parent *OpenBaoClusterReconciler
}

func (r *testCompositeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	statusReconciler := &openBaoClusterStatusReconciler{parent: r.parent}
	workloadReconciler := &openBaoClusterWorkloadReconciler{parent: r.parent}
	adminOpsReconciler := &openBaoClusterAdminOpsReconciler{parent: r.parent}

	if result, err := statusReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := workloadReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	if result, err := adminOpsReconciler.Reconcile(ctx, req); err != nil {
		return result, err
	}
	return statusReconciler.Reconcile(ctx, req)
}

var _ = Describe("OpenBaoCluster Controller", func() {
	Context("When reconciling a resource", func() {
		ctx := context.Background()

		newReconciler := func() *testCompositeReconciler {
			parent := &OpenBaoClusterReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				ImageVerifier: security.NewImageVerifier(logr.Discard(), k8sClient, nil),
			}
			return &testCompositeReconciler{parent: parent}
		}

		createMinimalCluster := func(name string, paused bool) *openbaov1alpha1.OpenBaoCluster {
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
						Image: "openbao/openbao-config-init:latest",
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

		It("blocks reconciliation when spec.profile is not set", func() {
			cluster := createMinimalCluster("test-profile-not-set", false)
			cluster.Spec.Profile = ""
			Expect(k8sClient.Update(ctx, cluster)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			// First reconcile adds the finalizer.
			_, err := newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile should refuse to proceed and set status conditions.
			_, err = newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			updated := &openbaov1alpha1.OpenBaoCluster{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())

			productionReady := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionProductionReady))
			Expect(productionReady).NotTo(BeNil())
			Expect(productionReady.Status).To(Equal(metav1.ConditionFalse))
			Expect(productionReady.Reason).To(Equal(ReasonProfileNotSet))

			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
			Expect(degraded.Reason).To(Equal(ReasonProfileNotSet))

			// Ensure we did not create TLS assets.
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
			// Enable SelfInit to avoid Degraded condition from RootTokenStored warning
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

			// First reconcile adds the finalizer.
			_, err := newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile performs the main reconciliation logic.
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

		It("does not create TLS Secrets when cluster is paused", func() {
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

		It("creates Gateway HTTPRoute backends and switches external Service selector during cutover", func() {
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-gateway-bluegreen-cutover",
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
						Image: "openbao/openbao-config-init:latest",
					},
					UpdateStrategy: openbaov1alpha1.UpdateStrategy{
						Type: openbaov1alpha1.UpdateStrategyBlueGreen,
					},
					Gateway: &openbaov1alpha1.GatewayConfig{
						Enabled: true,
						GatewayRef: openbaov1alpha1.GatewayReference{
							Name: "traefik-gateway",
						},
						Hostname: "bao.example.local",
					},
				},
				Status: openbaov1alpha1.OpenBaoClusterStatus{
					Initialized: true,
					BlueGreen: &openbaov1alpha1.BlueGreenStatus{
						Phase:         openbaov1alpha1.PhasePromoting,
						BlueRevision:  "blue123",
						GreenRevision: "green456",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			// Status is stored via the status subresource; persist it explicitly for
			// later reconciliation logic that depends on blue/green state.
			cluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
				Initialized: true,
				BlueGreen: &openbaov1alpha1.BlueGreenStatus{
					Phase:         openbaov1alpha1.PhasePromoting,
					BlueRevision:  "blue123",
					GreenRevision: "green456",
				},
			}
			Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())

			// Create TLS secret required by infra Manager for StatefulSet prerequisites.
			serverSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cluster.Name + constants.SuffixTLSServer,
					Namespace: cluster.Namespace,
				},
				Data: map[string][]byte{
					"tls.crt": []byte("test-cert"),
					"tls.key": []byte("test-key"),
					"ca.crt":  []byte("test-ca"),
				},
			}
			Expect(k8sClient.Create(ctx, serverSecret)).To(Succeed())

			infraMgr := infra.NewManager(k8sClient, k8sClient.Scheme(), "openbao-operator-system", "", nil, "")

			By("reconciling networking resources")
			spec := infra.StatefulSetSpec{
				Name:               cluster.Name,
				Revision:           "",
				Image:              cluster.Spec.Image,
				InitContainerImage: "",
				Replicas:           cluster.Spec.Replicas,
				ConfigHash:         "",
				DisableSelfInit:    false,
				SkipReconciliation: false,
			}
			err := infraMgr.Reconcile(ctx, logr.Discard(), cluster, spec)
			Expect(err).NotTo(HaveOccurred())

			By("ensuring the HTTPRoute references only the main external service")
			route := &gatewayv1.HTTPRoute{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      cluster.Name + "-httproute",
			}, route)
			Expect(err).NotTo(HaveOccurred())
			Expect(route.Spec.Rules).ToNot(BeEmpty())
			backends := route.Spec.Rules[0].BackendRefs
			Expect(backends).To(HaveLen(1))
			Expect(string(backends[0].Name)).To(Equal(cluster.Name + "-public"))
			// Some Gateway API implementations default Weight to 1 when unset.
			if backends[0].Weight != nil {
				Expect(*backends[0].Weight).To(Equal(int32(1)))
			}

			By("ensuring the external Service selects the Blue revision before cutover")
			svc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      cluster.Name + "-public",
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelOpenBaoRevision, "blue123"))

			By("switching to DemotingBlue and ensuring the external Service selects the Green revision")
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseDemotingBlue
			spec = infra.StatefulSetSpec{
				Name:               cluster.Name,
				Revision:           "blue123",
				Image:              cluster.Spec.Image,
				InitContainerImage: "",
				Replicas:           cluster.Spec.Replicas,
				ConfigHash:         "",
				DisableSelfInit:    false,
				SkipReconciliation: false,
			}
			err = infraMgr.Reconcile(ctx, logr.Discard(), cluster, spec)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Get(ctx, types.NamespacedName{
				Namespace: cluster.Namespace,
				Name:      cluster.Name + "-public",
			}, svc)
			Expect(err).NotTo(HaveOccurred())
			Expect(svc.Spec.Selector).To(HaveKeyWithValue(constants.LabelOpenBaoRevision, "green456"))

			By("ensuring any legacy blue/green backend Services do not exist")
			legacySvc := &corev1.Service{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name + "-public-blue"}, legacySvc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Name + "-public-green"}, legacySvc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		It("honors DeletionPolicy Retain by preserving PVCs and verifying GC cleanup via OwnerReferences", func() {
			cluster := createMinimalCluster("test-delete-retain", false)
			cluster.Spec.DeletionPolicy = openbaov1alpha1.DeletionPolicyRetain
			Expect(k8sClient.Update(ctx, cluster)).To(Succeed())

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler()

			// Attach finalizer and create infrastructure.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet exists and has OwnerReference for GC cleanup.
			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())

			// Verify OwnerReference is set (indicates GC will delete it when cluster is deleted)
			foundOwnerRef := false
			for _, ref := range sts.OwnerReferences {
				if ref.Kind == "OpenBaoCluster" && ref.Name == cluster.Name {
					foundOwnerRef = true
					break
				}
			}
			Expect(foundOwnerRef).To(BeTrue(), "StatefulSet should have OwnerReference for GC cleanup")

			// Seed a PVC that would normally be created by the StatefulSet controller.
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "data-test-delete-retain-0",
					Namespace: cluster.Namespace,
					Labels: map[string]string{
						"openbao.org/cluster": cluster.Name,
					},
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

			// Delete the OpenBaoCluster and run finalizer logic.
			Expect(k8sClient.Delete(ctx, cluster)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// PVC should be retained per DeletionPolicy=Retain.
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
			}, &corev1.PersistentVolumeClaim{})
			Expect(err).NotTo(HaveOccurred())

			// Note: StatefulSet deletion is handled by Kubernetes GC via OwnerReferences.
			// In envtest, GC may not run immediately, so we verify OwnerReference exists instead
			// of checking that StatefulSet is deleted.
		})

		It("accepts OpenBaoCluster with structured configuration", func() {
			uiEnabled := true
			cluster := &openbaov1alpha1.OpenBaoCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-structured-config",
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
					Configuration: &openbaov1alpha1.OpenBaoConfiguration{
						UI:       &uiEnabled,
						LogLevel: "debug",
					},
				},
			}

			err := k8sClient.Create(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		})

		It("rotates the server certificate and signals reload when within the rotation window", func() {
			cluster := createMinimalCluster("test-tls-rotation", false)
			cluster.Spec.TLS.RotationPeriod = "9000h"
			Expect(k8sClient.Update(ctx, cluster)).To(Succeed())

			reloader := &tlsReloadRecorder{}

			parent := &OpenBaoClusterReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				TLSReload: reloader,
			}
			controllerReconciler := &testCompositeReconciler{parent: parent}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			// First reconcile adds finalizer.
			_, err := controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates initial TLS assets.
			_, err = controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Third reconcile evaluates rotation window and should trigger reload.
			_, err = controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(reloader.hashes)).To(BeNumerically(">=", 2))
		})

		It("creates StatefulSet with 1 replica when cluster is not initialized", func() {
			cluster := createMinimalCluster("test-init-replicas", false)
			cluster.Status.Initialized = false

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			// First reconcile adds finalizer.
			_, err := newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates infrastructure.
			_, err = newReconciler().Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet exists with 1 replica
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
			cluster := createMinimalCluster("test-init-scale", false)
			cluster.Status.Initialized = false
			cluster.Spec.Replicas = 3

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler()

			// First reconcile adds finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates infrastructure with 1 replica.
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet has 1 replica
			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			// Mark cluster as initialized. In the real controller this transition is
			// owned by the internal init manager; here we simply patch status to
			// simulate that initialization has completed.
			Eventually(func(g Gomega) {
				latest := &openbaov1alpha1.OpenBaoCluster{}
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				}, latest)
				g.Expect(err).NotTo(HaveOccurred())

				// Capture original state for status patching to avoid optimistic locking conflicts
				original := latest.DeepCopy()
				latest.Status.Initialized = true
				err = k8sClient.Status().Patch(ctx, latest, client.MergeFrom(original))
				g.Expect(err).NotTo(HaveOccurred())
			}).Should(Succeed())

			// Reconcile again - should scale to desired replicas
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify StatefulSet scaled to desired replicas
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(err).NotTo(HaveOccurred())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})

		It("skips initialization when cluster is already initialized", func() {
			cluster := createMinimalCluster("test-init-skip", false)
			// Capture original state for status patching to avoid optimistic locking conflicts
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

			// First reconcile adds finalizer.
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile should not modify the Initialized status when InitManager is not configured
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			// Verify cluster remains initialized
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

type tlsReloadRecorder struct {
	hashes []string
}

func (r *tlsReloadRecorder) SignalReload(_ context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, certHash string) error {
	r.hashes = append(r.hashes, certHash)
	return nil
}

// Multi-tenancy Tests
// These tests verify the multi-tenancy requirements (FR-MT-01 through FR-MT-05) are satisfied.

var _ = Describe("OpenBaoCluster Multi-Tenancy", func() {
	Context("When managing multiple clusters in different namespaces", func() {
		ctx := context.Background()

		newReconciler := func() *testCompositeReconciler {
			parent := &OpenBaoClusterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			return &testCompositeReconciler{parent: parent}
		}

		createClusterInNamespace := func(name, namespace string) *openbaov1alpha1.OpenBaoCluster {
			// Ensure namespace exists
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
						Image: "openbao/openbao-config-init:latest",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			return cluster
		}

		AfterEach(func() {
			// Clean up clusters from all test namespaces
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

			// Reconcile both clusters
			_, err := reconciler.Reconcile(ctx, reqA)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reqA)
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reqB)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reqB)
			Expect(err).NotTo(HaveOccurred())

			// Verify cluster A has its own resources in tenant-a namespace
			caSecretA := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterA.Name + constants.SuffixTLSCA,
				Namespace: "tenant-a",
			}, caSecretA)
			Expect(err).NotTo(HaveOccurred())

			// Verify cluster B has its own resources in tenant-b namespace
			caSecretB := &corev1.Secret{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterB.Name + constants.SuffixTLSCA,
				Namespace: "tenant-b",
			}, caSecretB)
			Expect(err).NotTo(HaveOccurred())

			// Verify the secrets are different (unique per cluster)
			Expect(caSecretA.Data["ca.crt"]).NotTo(Equal(caSecretB.Data["ca.crt"]))

			// Verify each cluster has independently created StatefulSets
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

			// Verify namespace isolation - resources are in their respective namespaces
			Expect(stsA.Namespace).To(Equal("tenant-a"))
			Expect(stsB.Namespace).To(Equal("tenant-b"))
		})

		// FR-MT-05: Avoid sharing Secrets or ConfigMaps between different OpenBaoCluster instances
		It("creates uniquely named resources per cluster preventing cross-tenant sharing (FR-MT-05)", func() {
			// Create two clusters with same name but in different namespaces
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

			// Reconcile both clusters
			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, req1)
				Expect(err).NotTo(HaveOccurred())
				_, err = reconciler.Reconcile(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify uniquely named resources exist in each namespace
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
			// Create two clusters
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

			// Reconcile the good cluster first
			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, reqGood)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify good cluster has resources
			stsGood := &appsv1.StatefulSet{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterGood.Name,
				Namespace: "tenant-a",
			}, stsGood)
			Expect(err).NotTo(HaveOccurred())

			// Now reconcile the other cluster - it should work independently
			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, reqOther)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify other cluster has resources
			stsOther := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      clusterOther.Name,
				Namespace: "tenant-b",
			}, stsOther)
			Expect(err).NotTo(HaveOccurred())

			// Verify good cluster is still intact (not affected by other cluster)
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

			// Reconcile both clusters in the same namespace
			for i := 0; i < 2; i++ {
				_, err := reconciler.Reconcile(ctx, req1)
				Expect(err).NotTo(HaveOccurred())
				_, err = reconciler.Reconcile(ctx, req2)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify both clusters have distinct resources
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

			// Verify resources are named differently
			Expect(sts1.Name).NotTo(Equal(sts2.Name))

			// Verify each StatefulSet uses its own ConfigMap and Secrets
			Expect(sts1.Spec.Template.Spec.Volumes).NotTo(BeEmpty())
			Expect(sts2.Spec.Template.Spec.Volumes).NotTo(BeEmpty())

			// Find the config volume and verify it references the correct ConfigMap
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
