package helpers

import (
	"os"
	"strings"
)

const (
	// EnvRustFSImage overrides the pinned RustFS emulator image used by E2E storage-provider tests.
	EnvRustFSImage = "E2E_RUSTFS_IMAGE"
	// EnvFakeGCSImage overrides the pinned fake-gcs-server emulator image used by E2E storage-provider tests.
	EnvFakeGCSImage = "E2E_FAKE_GCS_IMAGE"
	// EnvAzuriteImage overrides the pinned Azurite emulator image used by E2E storage-provider tests.
	EnvAzuriteImage = "E2E_AZURITE_IMAGE"

	// DefaultRustFSImage pins the RustFS multi-arch manifest that replaced the historical :latest reference.
	DefaultRustFSImage = "docker.io/rustfs/rustfs@sha256:3c2d55977829620284ece8593901bf776bcfc0fc9972784352de4dcffdb92416"
	// DefaultFakeGCSImage pins the fake-gcs-server multi-arch manifest that replaced the historical :latest reference.
	DefaultFakeGCSImage = "docker.io/fsouza/fake-gcs-server@sha256:3730da0e31f7e5186a90ec4899dc2c336104e7599df400411392ef17e684c31f"
	// DefaultAzuriteImage pins the Azurite multi-arch manifest that replaced the historical :latest reference.
	DefaultAzuriteImage = "mcr.microsoft.com/azure-storage/azurite@sha256:647c63a91102a9d8e8000aab803436e1fc85fbb285e7ce830a82ee5d6661cf37"
)

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
