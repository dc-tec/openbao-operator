package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

const (
	compatibilitySmokeCoverage = "compatibility-smoke"
	matrixBoolTrue             = "true"
	matrixBoolFalse            = "false"
)

func testVersionPolicy() versionPolicy {
	return versionPolicy{
		OpenBao: openBaoVersionPolicy{
			DefaultImage: "ghcr.io/openbao/openbao:2.5.5",
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
				InstallCSIHostPath:       true,
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
	if row.OpenBaoImage != "ghcr.io/openbao/openbao:2.5.5" {
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
	if row.InstallCSIHostPath != matrixBoolTrue {
		t.Fatalf("install csi hostpath = %q, want true", row.InstallCSIHostPath)
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
	if matrix.Include[2].Coverage != compatibilitySmokeCoverage {
		t.Fatalf("coverage = %q, want %s", matrix.Include[2].Coverage, compatibilitySmokeCoverage)
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

func TestRepoManifestNightlyRoutingUsesCanonicalLaneFilters(t *testing.T) {
	t.Parallel()

	m := loadRepoManifest(t)
	lanes, errs := validateLanes(m)
	if len(errs) > 0 {
		t.Fatalf("validate lanes: %s", strings.Join(errs, "; "))
	}
	securityLane, ok := lanes["security"]
	if !ok {
		t.Fatal("security lane is not defined")
	}
	if !strings.Contains(securityLane.LabelFilter, "tenant && !lifecycle") {
		t.Fatalf("security lane label filter = %q, want tenant lifecycle exclusion", securityLane.LabelFilter)
	}

	compatVersion := firstVersion(t, m.Versions.Kubernetes.Compatibility, "compatibility")
	dailyCompat := singleNightlyRow(t, m, "daily", nightlyFilters{
		Lane:       "security",
		Kubernetes: compatVersion,
	})
	if dailyCompat.Coverage != compatibilitySmokeCoverage {
		t.Fatalf("daily security coverage = %q, want %s", dailyCompat.Coverage, compatibilitySmokeCoverage)
	}
	if strings.Contains(dailyCompat.LabelFilter, "|| tenant ||") {
		t.Fatalf(
			"daily security compatibility label filter = %q, must not select all tenant lifecycle specs",
			dailyCompat.LabelFilter,
		)
	}
	if !strings.Contains(dailyCompat.LabelFilter, "tenant && !lifecycle") {
		t.Fatalf("daily security compatibility label filter = %q, want tenant lifecycle exclusion", dailyCompat.LabelFilter)
	}

	coreCompat := singleNightlyRow(t, m, "daily", nightlyFilters{
		Lane:       "core",
		Kubernetes: compatVersion,
	})
	if coreCompat.Coverage != compatibilitySmokeCoverage {
		t.Fatalf("daily core coverage = %q, want %s", coreCompat.Coverage, compatibilitySmokeCoverage)
	}
	if !strings.Contains(coreCompat.LabelFilter, "lifecycle && smoke") ||
		!strings.Contains(coreCompat.LabelFilter, "manager && smoke") {
		t.Fatalf(
			"daily core compatibility label filter = %q, want smoke-gated lifecycle and manager routing",
			coreCompat.LabelFilter,
		)
	}

	releaseGateVersion := firstVersion(t, m.Versions.Kubernetes.ReleaseGate, "releaseGate")
	assertProfileUsesCanonicalLaneFilters(t, m, lanes, "weekly-full", "full", releaseGateVersion)
	assertProfileUsesCanonicalLaneFilters(t, m, lanes, "release-gate", "release-gate", releaseGateVersion)
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
	if !strings.Contains(out, `"install_csi_hostpath":"false"`) {
		t.Fatalf("matrix json missing csi hostpath install flag: %s", out)
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

func loadRepoManifest(t *testing.T) manifest {
	t.Helper()

	m, err := loadManifest(filepath.Join("..", "..", "..", "test", "e2e", "suites.yaml"))
	if err != nil {
		t.Fatalf("load repo manifest: %v", err)
	}
	return m
}

func singleNightlyRow(t *testing.T, m manifest, profile string, filters nightlyFilters) matrixRow {
	t.Helper()

	matrix, err := buildGithubNightlyMatrix(m, profile, filters)
	if err != nil {
		t.Fatalf("build %s nightly matrix: %v", profile, err)
	}
	if got := len(matrix.Include); got != 1 {
		t.Fatalf("%s nightly matrix rows = %d, want 1", profile, got)
	}
	return matrix.Include[0]
}

func assertProfileUsesCanonicalLaneFilters(
	t *testing.T,
	m manifest,
	lanes map[string]ciLaneConfig,
	profile string,
	coverage string,
	kubernetes string,
) {
	t.Helper()

	matrix, err := buildGithubNightlyMatrix(m, profile, nightlyFilters{Kubernetes: kubernetes})
	if err != nil {
		t.Fatalf("build %s nightly matrix: %v", profile, err)
	}
	if len(matrix.Include) == 0 {
		t.Fatalf("%s nightly matrix produced no rows", profile)
	}
	for _, row := range matrix.Include {
		if row.Coverage != coverage {
			t.Fatalf("%s row %q coverage = %q, want %q", profile, row.ID, row.Coverage, coverage)
		}
		lane, ok := lanes[row.ID]
		if !ok {
			t.Fatalf("%s row references unknown lane %q", profile, row.ID)
		}
		if row.LabelFilter != lane.LabelFilter {
			t.Fatalf(
				"%s row %q label filter = %q, want canonical lane filter %q",
				profile,
				row.ID,
				row.LabelFilter,
				lane.LabelFilter,
			)
		}
	}
}

func firstVersion(t *testing.T, versions []string, field string) string {
	t.Helper()

	if len(versions) == 0 {
		t.Fatalf("versions.kubernetes.%s must not be empty", field)
	}
	return versions[0]
}
