package controller

import (
	"fmt"

	claimadmission "github.com/dc-tec/openbao-operator/internal/adapter/kube/admission/openbaoclusterclaim"
	appopenbaoclusterclaim "github.com/dc-tec/openbao-operator/internal/app/openbaoclusterclaim"
	openbaoclusterclaimcontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaim"
	openbaoclusterclaimbackuprequestcontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaimbackuprequest"
	openbaoclusterclaimrestorerequestcontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaimrestorerequest"
	openbaoclusterclaimupgraderequestcontroller "github.com/dc-tec/openbao-operator/internal/controller/openbaoclusterclaimupgraderequest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	claimAdmissionServingCertDir  = "/tmp/k8s-webhook-server/serving-certs"
	claimAdmissionServingCertFile = "tls.crt"
	claimAdmissionServingKeyFile  = "tls.key"
)

func setupClaimControllers(mgr ctrl.Manager, runtime controllerProcessRuntime) error {
	if !runtime.enableServiceClaims {
		return nil
	}

	setupClaimAdmission(mgr, runtime)

	if err := (&openbaoclusterclaimcontroller.OpenBaoClusterClaimReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		EnableServiceClaims: runtime.enableServiceClaims,
		SameClusterNetwork: appopenbaoclusterclaim.SameClusterNetworkConfig{
			APIServerCIDR:        runtime.serviceClaimsAPIServerCIDR,
			APIServerEndpointIPs: append([]string(nil), runtime.serviceClaimsAPIServerEndpointIPs...),
			DNSEndpointIPs:       append([]string(nil), runtime.serviceClaimsDNSEndpointIPs...),
		},
		SameClusterTransitUnseal: appopenbaoclusterclaim.SameClusterTransitUnsealConfig{
			Address:               runtime.serviceClaimsTransitUnsealAddress,
			KeyName:               runtime.serviceClaimsTransitUnsealKeyName,
			MountPath:             runtime.serviceClaimsTransitUnsealMountPath,
			Namespace:             runtime.serviceClaimsTransitUnsealNamespace,
			TLSCACert:             runtime.serviceClaimsTransitUnsealTLSCACert,
			TLSServerName:         runtime.serviceClaimsTransitUnsealTLSServerName,
			CredentialsSecretName: runtime.serviceClaimsTransitUnsealCredentialsSecretName,
		},
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller openbaoclusterclaim: %w", err)
	}
	if runtime.enableServiceClaims {
		if err := (&openbaoclusterclaimbackuprequestcontroller.OpenBaoClusterClaimBackupRequestReconciler{
			Client:              mgr.GetClient(),
			Scheme:              mgr.GetScheme(),
			EnableServiceClaims: runtime.enableServiceClaims,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("unable to create controller openbaoclusterclaimbackuprequest: %w", err)
		}
		if err := (&openbaoclusterclaimrestorerequestcontroller.OpenBaoClusterClaimRestoreRequestReconciler{
			Client:              mgr.GetClient(),
			Scheme:              mgr.GetScheme(),
			EnableServiceClaims: runtime.enableServiceClaims,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("unable to create controller openbaoclusterclaimrestorerequest: %w", err)
		}
		if err := (&openbaoclusterclaimupgraderequestcontroller.OpenBaoClusterClaimUpgradeRequestReconciler{
			Client:              mgr.GetClient(),
			Scheme:              mgr.GetScheme(),
			EnableServiceClaims: runtime.enableServiceClaims,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("unable to create controller openbaoclusterclaimupgraderequest: %w", err)
		}
	}

	return nil
}

func setupClaimAdmission(mgr ctrl.Manager, runtime controllerProcessRuntime) {
	if !runtime.enableServiceClaims {
		return
	}

	mgr.GetWebhookServer().Register(
		claimadmission.MutatingWebhookPath,
		&admission.Webhook{
			Handler: claimadmission.NewServiceOfferingMutator(
				mgr.GetClient(),
				mgr.GetScheme(),
				runtime.enableServiceClaims,
				runtime.operatorNamespace,
				runtime.operatorServiceAccountName,
			),
		},
	)
	setupLog.Info(
		"Registered OpenBaoClusterClaim mutating webhook",
		"path",
		claimadmission.MutatingWebhookPath,
	)
}
