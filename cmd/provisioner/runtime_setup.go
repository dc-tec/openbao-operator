package provisioner

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	provisionercontroller "github.com/dc-tec/openbao-operator/internal/controller/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/admission"
)

type provisionerProcessRuntime struct {
	provisioner       appprovisioner.Provisioner
	operatorNamespace string
	admissionTracker  *admission.Tracker
}

func buildProvisionerProcessRuntime(mgr ctrl.Manager, cfg runConfig) (provisionerProcessRuntime, error) {
	provisionerRuntime, err := appprovisioner.NewProvisioner(appprovisioner.ProvisionerDependencies{
		Client: mgr.GetClient(),
		Logger: setupLog.WithName("provisioner"),
	})
	if err != nil {
		return provisionerProcessRuntime{}, fmt.Errorf("unable to create provisioner runtime: %w", err)
	}

	return provisionerProcessRuntime{
		provisioner:       provisionerRuntime,
		operatorNamespace: operatorNamespaceFromEnv(),
		admissionTracker:  initializeAdmissionTracker(mgr, cfg),
	}, nil
}

func setupControllers(mgr ctrl.Manager, runtime provisionerProcessRuntime) error {
	if err := (&provisionercontroller.NamespaceProvisionerReconciler{
		Client:            mgr.GetClient(),
		APIReader:         mgr.GetAPIReader(),
		Recorder:          mgr.GetEventRecorder("namespace-provisioner"),
		Provisioner:       runtime.provisioner,
		OperatorNamespace: runtime.operatorNamespace,
		AdmissionTracker:  runtime.admissionTracker,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller NamespaceProvisioner: %w", err)
	}

	if err := (&provisionercontroller.TenantSecretsRBACReconciler{
		Client:           mgr.GetClient(),
		AdmissionTracker: runtime.admissionTracker,
		APIReader:        mgr.GetAPIReader(),
		Recorder:         mgr.GetEventRecorder("namespace-provisioner-tenant-secrets"),
		Provisioner:      runtime.provisioner,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("unable to create controller TenantSecretsRBAC: %w", err)
	}

	return nil
}

func addManagerHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	return nil
}
