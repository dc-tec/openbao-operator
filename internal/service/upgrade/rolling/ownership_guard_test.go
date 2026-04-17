package rolling

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAdminOpsStatusPatchFallbackCallsitesAreRemoved(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	dir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q) error = %v", dir, err)
	}

	mergePatchCallsites := map[string]bool{}
	statusUpdateFallbackCallsites := map[string]bool{}
	statusSubresourceUpdateCallsites := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", name, err)
		}
		if strings.Contains(string(content), "PatchOpenBaoClusterStatusMerge(") {
			mergePatchCallsites[name] = true
		}
		if strings.Contains(string(content), "UpdateOpenBaoClusterStatus(") {
			statusUpdateFallbackCallsites[name] = true
		}
		if strings.Contains(string(content), "Status().Update(") {
			statusSubresourceUpdateCallsites[name] = true
		}
	}

	if len(mergePatchCallsites) != 0 {
		t.Fatalf("merge patch callsites = %#v, want none", mergePatchCallsites)
	}
	if len(statusUpdateFallbackCallsites) != 0 {
		t.Fatalf("status update fallback callsites = %#v, want none", statusUpdateFallbackCallsites)
	}
	if len(statusSubresourceUpdateCallsites) != 0 {
		t.Fatalf("status subresource update callsites = %#v, want none", statusSubresourceUpdateCallsites)
	}
}
