package resourcelock

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	testEnv *envtest.Environment
	cfg     *rest.Config
	decoder admission.Decoder
)

func TestMain(m *testing.M) {
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	// Reuse the same binary discovery logic as controller envtests so that
	// tests can be run from IDEs or without Makefile targets.
	if dir := getFirstFoundEnvTestBinaryDir(); dir != "" {
		testEnv.BinaryAssetsDirectory = dir
	}

	// Start the envtest control plane to validate that our decoder and webhook
	// interactions work against a real API server configuration. The tests in
	// this package do not create real resources but rely on the scheme and
	// admission decoder provided by controller-runtime.
	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		logf.Log.Error(err, "failed to start envtest for resource lock webhook")
		os.Exit(1)
	}

	decoder = admission.NewDecoder(scheme.Scheme)

	code := m.Run()

	if stopErr := testEnv.Stop(); stopErr != nil {
		logf.Log.Error(stopErr, "failed to stop envtest for resource lock webhook")
		os.Exit(1)
	}

	os.Exit(code)
}

// getFirstFoundEnvTestBinaryDir locates the first envtest binary directory.
// It mirrors the logic used in controller envtests so that the same
// setup-envtest tooling can be reused.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "failed to read envtest binary directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}
