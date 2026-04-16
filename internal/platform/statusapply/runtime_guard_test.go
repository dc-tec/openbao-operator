package statusapply

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeCallsitesUseAdminOpsMutateApplyGateway(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	internalRoot := filepath.Join(repoRoot, "internal")
	statusApplyDir := filepath.Join(internalRoot, "platform", "statusapply")
	directApplyPattern := regexp.MustCompile(`\bApplyOpenBaoClusterAdminOpsStatus\(`)

	directApplyCallsites := map[string]bool{}
	err := filepath.WalkDir(internalRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Clean(path) == statusApplyDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if directApplyPattern.Match(content) {
			relPath, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				relPath = path
			}
			directApplyCallsites[relPath] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(%q) error = %v", internalRoot, err)
	}

	if len(directApplyCallsites) != 0 {
		t.Fatalf("direct runtime ApplyOpenBaoClusterAdminOpsStatus callsites = %#v, want none", directApplyCallsites)
	}
}
