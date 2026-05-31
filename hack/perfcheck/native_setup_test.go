package main

import "testing"

func TestNativeImageSelection(t *testing.T) {
	t.Parallel()

	opts := defaultOptions("verify")

	tests := []struct {
		name             string
		scenario         string
		wantBuildTarget  string
		avoidBuildTarget string
		wantImage        string
		avoidImage       string
	}{
		{
			name:             "backup uses backup executor only",
			scenario:         "backup",
			wantBuildTarget:  "docker-build-backup",
			avoidBuildTarget: "docker-build-upgrade",
			wantImage:        opts.BackupExecutorImage,
			avoidImage:       opts.UpgradeExecutorImage,
		},
		{
			name:             "restore uses backup executor only",
			scenario:         "restore",
			wantBuildTarget:  "docker-build-backup",
			avoidBuildTarget: "docker-build-upgrade",
			wantImage:        opts.BackupExecutorImage,
			avoidImage:       opts.UpgradeExecutorImage,
		},
		{
			name:             "rolling upgrade uses upgrade executor only",
			scenario:         "rolling-upgrade",
			wantBuildTarget:  "docker-build-upgrade",
			avoidBuildTarget: "docker-build-backup",
			wantImage:        opts.UpgradeExecutorImage,
			avoidImage:       opts.BackupExecutorImage,
		},
		{
			name:             "lifecycle skips workflow executors",
			scenario:         "lifecycle-convergence",
			avoidBuildTarget: "docker-build-backup",
			avoidImage:       opts.BackupExecutorImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scenario := scenarioSpec{Name: tt.scenario}
			buildTargets := map[string]struct{}{}
			for _, build := range nativeImageBuilds(opts, scenario) {
				buildTargets[build.target] = struct{}{}
			}
			if tt.wantBuildTarget != "" {
				if _, ok := buildTargets[tt.wantBuildTarget]; !ok {
					t.Fatalf("missing build target %q in %v", tt.wantBuildTarget, buildTargets)
				}
			}
			if tt.avoidBuildTarget != "" {
				if _, ok := buildTargets[tt.avoidBuildTarget]; ok {
					t.Fatalf("unexpected build target %q in %v", tt.avoidBuildTarget, buildTargets)
				}
			}

			imageSet := stringSet(nativeImages(opts, scenario))
			if tt.wantImage != "" {
				if _, ok := imageSet[tt.wantImage]; !ok {
					t.Fatalf("missing image %q in %v", tt.wantImage, imageSet)
				}
			}
			if tt.avoidImage != "" {
				if _, ok := imageSet[tt.avoidImage]; ok {
					t.Fatalf("unexpected image %q in %v", tt.avoidImage, imageSet)
				}
			}
		})
	}
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}
