//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/service/infra"
)

var _ = Describe("OpenBaoCluster Networking", func() {
	Context("When reconciling gateway-backed networking", func() {
		ctx := context.Background()

		AfterEach(func() {
			var clusterList openbaov1alpha1.OpenBaoClusterList
			err := k8sClient.List(ctx, &clusterList)
			Expect(err).NotTo(HaveOccurred())
			for i := range clusterList.Items {
				_ = k8sClient.Delete(ctx, &clusterList.Items[i])
			}
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
						Image: "openbao/openbao-init:latest",
					},
					Upgrade: &openbaov1alpha1.UpgradeConfig{
						Strategy: openbaov1alpha1.UpdateStrategyBlueGreen,
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
			cluster.Status = openbaov1alpha1.OpenBaoClusterStatus{
				Initialized: true,
				BlueGreen: &openbaov1alpha1.BlueGreenStatus{
					Phase:         openbaov1alpha1.PhasePromoting,
					BlueRevision:  "blue123",
					GreenRevision: "green456",
				},
			}
			Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())

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

			By("switching to Cleanup and ensuring the external Service selects the Green revision")
			cluster.Status.BlueGreen.Phase = openbaov1alpha1.PhaseCleanup
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
	})
})
