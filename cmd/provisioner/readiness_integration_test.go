//go:build integration

package provisioner

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	appprovisioner "github.com/dc-tec/openbao-operator/internal/app/provisioner"
	"github.com/dc-tec/openbao-operator/internal/platform/testutil/managerprobe"
)

func TestProvisionerReadinessCacheAndWatchContract(t *testing.T) {
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "true")
	config := managerprobe.Environment(t)
	gate := managerprobe.NewListGate("openbaorestores")
	t.Cleanup(gate.Release)
	config.WrapTransport = gate.Wrap
	address := managerprobe.ProbeAddress(t)
	options := newManagerOptions(metricsserver.Options{BindAddress: "0"}, address, false)
	warmup := true
	options.Controller.EnableWarmup = &warmup
	options.Controller.SkipNameValidation = &warmup
	var recording *managerprobe.RecordingCache
	options.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
		var err error
		recording, err = managerprobe.NewRecordingCache(config, opts)
		return recording, err
	}
	mgr, err := ctrl.NewManager(config, options)
	require.NoError(t, err)
	observed := &managerprobe.WarmupObserver{Manager: mgr}
	require.NoError(t, setupControllers(observed, readinessProvisionerRuntime(t, mgr)))
	require.NoError(t, addManagerHealthChecks(t.Context(), mgr))
	managerprobe.Start(t, mgr)
	select {
	case <-gate.Observed:
	case <-time.After(10 * time.Second):
		t.Fatal("cache did not request initial population")
	}
	require.Eventually(t, func() bool {
		return managerprobe.Status(address, "/healthz") == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)
	require.Equal(t, http.StatusInternalServerError, managerprobe.Status(address, "/readyz"))
	gate.Release()
	select {
	case <-mgr.Elected():
	case <-time.After(10 * time.Second):
		t.Fatal("controller warmup did not finish")
	}
	observed.Wait(t)
	recording.AssertMatchesWatches(t)
	require.Eventually(t, func() bool {
		return managerprobe.Status(address, "/readyz") == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)
}

func TestProvisionerStandbyReadinessAdmissionLifecycle(t *testing.T) {
	t.Setenv("OPENBAO_UNSAFE_ADMISSION_DISABLED", "")
	config := managerprobe.Environment(t)
	address := managerprobe.ProbeAddress(t)
	options := newManagerOptions(metricsserver.Options{BindAddress: "0"}, address, true)
	options.LeaderElectionNamespace = "default"
	skipNames := true
	options.Controller.SkipNameValidation = &skipNames
	kubeClient, err := client.New(config, client.Options{Scheme: scheme})
	require.NoError(t, err)
	managerprobe.HoldLeadership(t, kubeClient, options.LeaderElectionID)
	mgr, err := ctrl.NewManager(config, options)
	require.NoError(t, err)
	require.NoError(t, setupControllers(mgr, readinessProvisionerRuntime(t, mgr)))
	require.NoError(t, addManagerHealthChecks(t.Context(), mgr))
	managerprobe.Start(t, mgr)
	managerprobe.AdmissionLifecycle(t, mgr, address)
}

func readinessProvisionerRuntime(t *testing.T, mgr ctrl.Manager) provisionerProcessRuntime {
	t.Helper()
	provisioner, err := appprovisioner.NewProvisioner(appprovisioner.ProvisionerDependencies{
		Client: mgr.GetClient(), Logger: mgr.GetLogger(),
	})
	require.NoError(t, err)
	return provisionerProcessRuntime{provisioner: provisioner, operatorNamespace: "default"}
}
