//go:build integration
// +build integration

package openbaocluster

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	security "github.com/dc-tec/openbao-operator/internal/adapter/security"
	appopenbaocluster "github.com/dc-tec/openbao-operator/internal/app/openbaocluster"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
	portsecurity "github.com/dc-tec/openbao-operator/internal/port/security"
)

type testApplicationsOptions struct {
	TLSReload    appopenbaocluster.TLSReloadSignaler
	OIDCDiscover portauth.DiscoverConfigFunc
}

func newTestOpenBaoClusterApplications(options testApplicationsOptions) *appopenbaocluster.Applications {
	var discover func(context.Context, *rest.Config) (*appopenbaocluster.OIDCConfig, error)
	if options.OIDCDiscover != nil {
		discover = func(ctx context.Context, config *rest.Config) (*appopenbaocluster.OIDCConfig, error) {
			discovered, err := options.OIDCDiscover(ctx, config, "")
			if err != nil {
				return nil, err
			}
			if discovered == nil {
				return nil, nil
			}
			return &appopenbaocluster.OIDCConfig{
				IssuerURL:          discovered.IssuerURL,
				OIDCDiscoveryURL:   discovered.OIDCDiscoveryURL,
				OIDCDiscoveryCAPEM: discovered.OIDCDiscoveryCAPEM,
				JWKSURL:            discovered.JWKSURL,
				JWKSCAPEM:          discovered.JWKSCAPEM,
				JWKSKeys:           discovered.JWKSKeys,
			}, nil
		}
	}

	imageVerifier := security.NewImageVerifier(logr.Discard(), k8sClient, nil)
	return appopenbaocluster.NewRuntimeApplications(appopenbaocluster.RuntimeApplicationsConfig{
		Kubernetes: appopenbaocluster.RuntimeKubernetesConfig{
			Client:     k8sClient,
			APIReader:  k8sClient,
			Scheme:     k8sClient.Scheme(),
			RestConfig: cfg,
		},
		OIDC: appopenbaocluster.RuntimeOIDCConfig{
			Discover: discover,
			DiscoveryStatusCode: func(err error) (int, bool) {
				if options.OIDCDiscover == nil {
					return 0, false
				}
				return portauth.DiscoveryStatusCode(err)
			},
		},
		OpenBao: appopenbaocluster.RuntimeOpenBaoConfig{
			TLSReload: options.TLSReload,
			ClientForPod: func(context.Context, *openbaov1alpha1.OpenBaoCluster, string) (portopenbao.ClusterActions, error) {
				return nil, fmt.Errorf("OpenBao client is unavailable in controller integration tests")
			},
		},
		ImageVerification: appopenbaocluster.RuntimeImageVerificationConfig{
			ImageVerifier:         imageVerifier,
			OperatorImageVerifier: imageVerifier,
			Infra: appopenbaocluster.InfraImageVerificationRuntime{
				OperatorImageVerifier: imageVerifier,
				VerifyImageFunc: func(
					ctx context.Context,
					logger logr.Logger,
					cluster *openbaov1alpha1.OpenBaoCluster,
					imageRef string,
				) (string, error) {
					if !portsecurity.IsMainImageVerificationEnabled(cluster) {
						return "", nil
					}
					return portsecurity.VerifyImageForCluster(ctx, logger, imageVerifier, cluster, imageRef)
				},
				VerifyOperatorImage:                portsecurity.VerifyOperatorImageForCluster,
				IsMainImageVerificationEnabled:     portsecurity.IsMainImageVerificationEnabled,
				IsOperatorImageVerificationEnabled: portsecurity.IsOperatorImageVerificationEnabled,
			},
		},
	})
}

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

func ensureTenantNamespaceProvisioned(ctx context.Context, namespace string) {
	key := types.NamespacedName{Namespace: namespace, Name: constants.TenantRoleBindingName}
	existing := &rbacv1.RoleBinding{}
	err := k8sClient.Get(ctx, key, existing)
	if err == nil {
		return
	}
	Expect(apierrors.IsNotFound(err)).To(BeTrue())

	Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.TenantRoleBindingName,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     constants.TenantRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "openbao-operator-controller",
				Namespace: "openbao-operator-system",
			},
		},
	})).To(Succeed())
}
