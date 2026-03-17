//go:build integration
// +build integration

package openbaocluster

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

var _ = Describe("OpenBaoCluster Validation", func() {
	Context("When validating cluster resources", func() {
		ctx := context.Background()

		createMinimalCluster := func(name string) *openbaov1alpha1.OpenBaoCluster {
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

		It("blocks reconciliation when spec.profile is not set", func() {
			cluster := createMinimalCluster("test-profile-not-set")
			cluster.Spec.Profile = ""
			err := k8sClient.Update(ctx, cluster)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
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
	})
})
