package main

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	matrixBoolTrue  = "true"
	matrixBoolFalse = "false"
)

func testVersionPolicy() versionPolicy {
	return versionPolicy{
		OpenBao: openBaoVersionPolicy{
			DefaultImage: "ghcr.io/openbao/openbao:2.5.3",
		},
		StorageEmulators: storageEmulatorVersionPolicy{
			RustFSImage:  "docker.io/rustfs/rustfs@sha256:test-rustfs",
			FakeGCSImage: "docker.io/fsouza/fake-gcs-server@sha256:test-gcs",
			AzuriteImage: "mcr.microsoft.com/azure-storage/azurite@sha256:test-azurite",
		},
		Kubernetes: kubernetesVersionPolicy{
			Primary:       "1.35.1",
			Compatibility: []string{"1.34.3"},
			ReleaseGate:   []string{"1.34.3", "1.35.1"},
			NextCandidate: "1.36.0",
		},
	}
}

func testParallelismPolicy() parallelismPolicy {
	return parallelismPolicy{
		DefaultNodes: 1,
		MaxNodes:     4,
	}
}

func TestBuildGithubMatrixPreservesLaneConfiguration(t *testing.T) {
	t.Parallel()

	matrix, err := buildGithubMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:                       "backup-restore",
				Name:                     "Backup & Restore",
				LabelFilter:              "((dr || restore) && !openshift)",
				PRLabelFilter:            "((dr && e2e-anchor) && !openshift)",
				PRScope:                  "backup",
				TimeoutMinutes:           45,
				E2ETimeout:               "40m",
				LoadBackupExecutorImage:  true,
				LoadUpgradeExecutorImage: false,
				PreloadStorageEmulators:  []string{"rustfs", "fake-gcs", "azurite"},
			},
		},
	})
	if err != nil {
		t.Fatalf("buildGithubMatrix() error = %v", err)
	}
	if got := len(matrix.Include); got != 1 {
		t.Fatalf("matrix rows = %d, want 1", got)
	}

	row := matrix.Include[0]
	if row.ID != "backup-restore" {
		t.Fatalf("id = %q, want backup-restore", row.ID)
	}
	if row.OpenBaoImage != "ghcr.io/openbao/openbao:2.5.3" {
		t.Fatalf("openbao image = %q, want central default", row.OpenBaoImage)
	}
	if row.KindNodeImage != "kindest/node:v1.35.1" {
		t.Fatalf("kind node image = %q, want central primary", row.KindNodeImage)
	}
	if row.PRLabelFilter != "((dr && e2e-anchor) && !openshift)" {
		t.Fatalf("pr label filter = %q, want PR-optimized filter", row.PRLabelFilter)
	}
	if row.LoadBackupExecutorImage != matrixBoolTrue {
		t.Fatalf("load backup image = %q, want true", row.LoadBackupExecutorImage)
	}
	if row.LoadUpgradeExecutorImage != matrixBoolFalse {
		t.Fatalf("load upgrade image = %q, want false", row.LoadUpgradeExecutorImage)
	}
	if row.PreloadStorageEmulators != matrixBoolTrue {
		t.Fatalf("preload storage emulators = %q, want true", row.PreloadStorageEmulators)
	}
	if row.PreloadRustFSImage != matrixBoolTrue ||
		row.PreloadFakeGCSImage != matrixBoolTrue ||
		row.PreloadAzuriteImage != matrixBoolTrue {
		t.Fatalf(
			"storage emulator image preload flags = %q/%q/%q, want all true",
			row.PreloadRustFSImage,
			row.PreloadFakeGCSImage,
			row.PreloadAzuriteImage,
		)
	}
	if row.RustFSImage != "docker.io/rustfs/rustfs@sha256:test-rustfs" {
		t.Fatalf("rustfs image = %q, want central storage emulator image", row.RustFSImage)
	}
	if row.FakeGCSImage != "docker.io/fsouza/fake-gcs-server@sha256:test-gcs" {
		t.Fatalf("fake gcs image = %q, want central storage emulator image", row.FakeGCSImage)
	}
	if row.AzuriteImage != "mcr.microsoft.com/azure-storage/azurite@sha256:test-azurite" {
		t.Fatalf("azurite image = %q, want central storage emulator image", row.AzuriteImage)
	}
	if row.TimeoutMinutes != 45 {
		t.Fatalf("timeout minutes = %d, want 45", row.TimeoutMinutes)
	}
	if row.ParallelNodes != 1 {
		t.Fatalf("parallel nodes = %d, want default 1", row.ParallelNodes)
	}
}

func TestBuildGithubMatrixRejectsInvalidLane(t *testing.T) {
	t.Parallel()

	_, err := buildGithubMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:             "bad lane",
				Name:           "Bad Lane",
				LabelFilter:    "lifecycle",
				PRScope:        "sometimes",
				TimeoutMinutes: 0,
				E2ETimeout:     "",
			},
		},
	})
	if err == nil {
		t.Fatalf("buildGithubMatrix() error = nil, want validation failure")
	}
	if !strings.Contains(err.Error(), "lowercase slug") || !strings.Contains(err.Error(), "prScope") {
		t.Fatalf("error = %q, want slug and prScope messages", err)
	}
}

func TestBuildGithubMatrixSkipsNonPRMatrixLane(t *testing.T) {
	t.Parallel()

	include := false
	matrix, err := buildGithubMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:                "platform-openshift",
				Name:              "OpenShift Platform",
				LabelFilter:       "openshift",
				PRScope:           "manual",
				TimeoutMinutes:    45,
				E2ETimeout:        "40m",
				IncludeInPRMatrix: &include,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildGithubMatrix() error = %v", err)
	}
	if got := len(matrix.Include); got != 0 {
		t.Fatalf("matrix rows = %d, want 0", got)
	}
}

func TestBuildGithubNightlyMatrixExpandsLaneSetsAndRows(t *testing.T) {
	t.Parallel()

	matrix, err := buildGithubNightlyMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:             "core",
				Name:           "Core Lifecycle & Manager",
				LabelFilter:    "lifecycle",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
				ParallelNodes:  2,
			},
			{
				ID:             "security",
				Name:           "Security & Tenants",
				LabelFilter:    "security",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
			},
		},
		Nightly: nightlyConfig{
			Profiles: []nightlyProfile{
				{
					ID: "daily",
					LaneSets: []nightlyLaneSet{
						{
							Coverage:   "full",
							Kubernetes: []string{"@primary"},
							Lanes:      []string{"core", "security"},
						},
					},
					Rows: []nightlyRowConfig{
						{
							Lane:        "core",
							Kubernetes:  "@compatibility",
							Coverage:    "compatibility-smoke",
							LabelFilter: "lifecycle && smoke",
						},
					},
				},
			},
		},
	}, "daily", nightlyFilters{})
	if err != nil {
		t.Fatalf("buildGithubNightlyMatrix() error = %v", err)
	}
	if got := len(matrix.Include); got != 3 {
		t.Fatalf("matrix rows = %d, want 3", got)
	}
	if matrix.Include[0].KindNodeImage != "kindest/node:v1.35.1" {
		t.Fatalf("kind node image = %q, want primary image", matrix.Include[0].KindNodeImage)
	}
	if matrix.Include[0].ParallelNodes != 2 {
		t.Fatalf("core parallel nodes = %d, want lane override 2", matrix.Include[0].ParallelNodes)
	}
	if matrix.Include[1].ParallelNodes != 1 {
		t.Fatalf("security parallel nodes = %d, want default 1", matrix.Include[1].ParallelNodes)
	}
	if matrix.Include[2].LabelFilter != "lifecycle && smoke" {
		t.Fatalf("smoke label filter = %q, want override", matrix.Include[2].LabelFilter)
	}
	if matrix.Include[2].KindNodeImage != "kindest/node:v1.34.3" {
		t.Fatalf("smoke kind node image = %q, want compatibility image", matrix.Include[2].KindNodeImage)
	}
	if matrix.Include[2].Coverage != "compatibility-smoke" {
		t.Fatalf("coverage = %q, want compatibility-smoke", matrix.Include[2].Coverage)
	}
}

func TestBuildGithubNightlyMatrixFiltersRows(t *testing.T) {
	t.Parallel()

	m := manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:             "core",
				Name:           "Core",
				LabelFilter:    "lifecycle",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
			},
			{
				ID:             "security",
				Name:           "Security",
				LabelFilter:    "security",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
			},
		},
		Nightly: nightlyConfig{
			Profiles: []nightlyProfile{
				{
					ID: "release-gate",
					LaneSets: []nightlyLaneSet{
						{
							Coverage:   "release-gate",
							Kubernetes: []string{"@releaseGate"},
							Lanes:      []string{"core", "security"},
						},
					},
				},
			},
		},
	}

	matrix, err := buildGithubNightlyMatrix(m, "release-gate", nightlyFilters{
		Lane:       "security",
		Kubernetes: "1.35.1",
	})
	if err != nil {
		t.Fatalf("buildGithubNightlyMatrix() error = %v", err)
	}
	if got := len(matrix.Include); got != 1 {
		t.Fatalf("matrix rows = %d, want 1", got)
	}
	row := matrix.Include[0]
	if row.ID != "security" || row.Kubernetes != "1.35.1" || row.Coverage != "release-gate" {
		t.Fatalf("filtered row = %#v, want security release-gate on 1.35.1", row)
	}

	_, err = buildGithubNightlyMatrix(m, "release-gate", nightlyFilters{Kubernetes: "1.36.0"})
	if err == nil {
		t.Fatalf("buildGithubNightlyMatrix() error = nil, want empty filter failure")
	}
	if !strings.Contains(err.Error(), "produced no rows") {
		t.Fatalf("error = %q, want empty filter message", err)
	}
}

func TestBuildGithubNightlyMatrixRejectsUnknownProfile(t *testing.T) {
	t.Parallel()

	_, err := buildGithubNightlyMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:             "core",
				Name:           "Core",
				LabelFilter:    "lifecycle",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
			},
		},
		Nightly: nightlyConfig{
			Profiles: []nightlyProfile{
				{ID: "daily"},
			},
		},
	}, "weekly-full", nightlyFilters{})
	if err == nil {
		t.Fatalf("buildGithubNightlyMatrix() error = nil, want missing profile")
	}
	if !strings.Contains(err.Error(), `profile "weekly-full"`) {
		t.Fatalf("error = %q, want missing profile message", err)
	}
}

func TestGithubMatrixJSONShape(t *testing.T) {
	t.Parallel()

	matrix, err := buildGithubMatrix(manifest{
		Version:     1,
		Versions:    testVersionPolicy(),
		Parallelism: testParallelismPolicy(),
		CILanes: []ciLaneConfig{
			{
				ID:             "core",
				Name:           "Core Lifecycle & Manager",
				LabelFilter:    "lifecycle",
				PRScope:        "always",
				TimeoutMinutes: 45,
				E2ETimeout:     "40m",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildGithubMatrix() error = %v", err)
	}

	data, err := json.Marshal(matrix)
	if err != nil {
		t.Fatalf("marshal matrix: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `"include"`) {
		t.Fatalf("matrix json missing include: %s", out)
	}
	if !strings.Contains(out, `"label_filter":"lifecycle"`) {
		t.Fatalf("matrix json missing label filter: %s", out)
	}
	if !strings.Contains(out, `"parallel_nodes":1`) {
		t.Fatalf("matrix json missing parallel nodes: %s", out)
	}
	if !strings.Contains(out, `"preload_storage_emulators":"false"`) {
		t.Fatalf("matrix json missing storage emulator preload flag: %s", out)
	}
	if !strings.Contains(out, `"preload_rustfs_image":"false"`) {
		t.Fatalf("matrix json missing rustfs preload flag: %s", out)
	}
	if !strings.Contains(out, `"rustfs_image":"docker.io/rustfs/rustfs@sha256:test-rustfs"`) {
		t.Fatalf("matrix json missing rustfs image: %s", out)
	}
}
