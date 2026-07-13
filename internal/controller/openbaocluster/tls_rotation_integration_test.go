//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

type tlsReloadRecorder struct {
	hashes []string
}

func (r *tlsReloadRecorder) SignalReload(_ context.Context, _ logr.Logger, _ *openbaov1alpha1.OpenBaoCluster, certHash string) error {
	r.hashes = append(r.hashes, certHash)
	return nil
}

var _ = Describe("OpenBaoCluster TLS Rotation", func() {
	Context("When reconciling certificate rotation", func() {
		ctx := context.Background()

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

		It("rotates the server certificate and signals reload when within the rotation window", func() {
			cluster := createMinimalCluster("test-tls-rotation")
			cluster.Spec.TLS.RotationPeriod = "9000h"
			Expect(k8sClient.Update(ctx, cluster)).To(Succeed())

			reloader := &tlsReloadRecorder{}
			parent := &OpenBaoClusterReconciler{
				Client:       k8sClient,
				Applications: newTestOpenBaoClusterApplications(testApplicationsOptions{TLSReload: reloader}),
			}
			controllerReconciler := &testCompositeReconciler{parent: parent}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			_, err = controllerReconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			Expect(len(reloader.hashes)).To(BeNumerically(">=", 2))
		})
	})
})
