//go:build integration
// +build integration

package openbaocluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

var _ = Describe("OpenBaoCluster OIDC Bootstrap", func() {
	Context("When hostile OIDC discovery blocks self-init bootstrap", func() {
		ctx := context.Background()

		newReconciler := func(discover portauth.DiscoverConfigFunc) *testCompositeReconciler {
			parent := &OpenBaoClusterReconciler{
				Client: k8sClient,
				ControllerRuntime: ControllerRuntime{
					APIReader:  k8sClient,
					Scheme:     k8sClient.Scheme(),
					RestConfig: cfg,
				},
				OIDCRuntime: OIDCRuntime{
					DiscoverOIDCConfig: discover,
					OIDCStatusCode:     portauth.DiscoveryStatusCode,
				},
				ImageVerificationRuntime: ImageVerificationRuntime{
					ImageVerifier: security.NewImageVerifier(logr.Discard(), k8sClient, nil),
				},
			}
			return &testCompositeReconciler{parent: parent}
		}

		createOIDCBootstrapCluster := func(name string) *openbaov1alpha1.OpenBaoCluster {
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
					SelfInit: &openbaov1alpha1.SelfInitConfig{
						Enabled: true,
						OIDC: &openbaov1alpha1.SelfInitOIDCConfig{
							Enabled: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			return cluster
		}

		assertFailClosedBootstrapError := func(cluster *openbaov1alpha1.OpenBaoCluster) {
			updated := &openbaov1alpha1.OpenBaoCluster{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, updated)
			Expect(err).NotTo(HaveOccurred())

			Expect(updated.Status.Workload).NotTo(BeNil())
			Expect(updated.Status.Workload.LastError).NotTo(BeNil())
			Expect(updated.Status.Workload.LastError.Reason).To(Equal(ReasonOIDCBootstrapConfigurationInvalid))

			degraded := meta.FindStatusCondition(updated.Status.Conditions, string(openbaov1alpha1.ConditionDegraded))
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
			Expect(degraded.Reason).To(Equal(ReasonOIDCBootstrapConfigurationInvalid))

			sts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			}, sts)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected StatefulSet creation to stay blocked")
		}

		AfterEach(func() {
			var clusterList openbaov1alpha1.OpenBaoClusterList
			err := k8sClient.List(ctx, &clusterList)
			Expect(err).NotTo(HaveOccurred())
			for i := range clusterList.Items {
				_ = k8sClient.Delete(ctx, &clusterList.Items[i])
			}
		})

		It("fails closed when discovery returns no issuer", func() {
			cluster := createOIDCBootstrapCluster("test-oidc-discovery-empty-issuer")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler(func(ctx context.Context, cfg *rest.Config, baseURL string) (*portauth.OIDCConfig, error) {
				return &portauth.OIDCConfig{JWKSURL: "https://issuer.example/keys"}, nil
			})

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			assertFailClosedBootstrapError(cluster)
		})

		It("fails closed when JWKS discovery content is malformed", func() {
			cluster := createOIDCBootstrapCluster("test-oidc-malformed-jwks")
			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cluster.Name,
					Namespace: cluster.Namespace,
				},
			}

			reconciler := newReconciler(func(ctx context.Context, cfg *rest.Config, baseURL string) (*portauth.OIDCConfig, error) {
				return nil, fmt.Errorf(
					"failed to fetch JWKS keys: %w",
					fmt.Errorf("%w: failed to parse jwks document: %w", portauth.ErrDiscoveryContentInvalid, malformedOIDCDiscoveryJSON()),
				)
			})

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).NotTo(HaveOccurred())

			assertFailClosedBootstrapError(cluster)
		})
	})
})

func malformedOIDCDiscoveryJSON() error {
	var payload map[string]any
	return json.Unmarshal([]byte("{"), &payload)
}
