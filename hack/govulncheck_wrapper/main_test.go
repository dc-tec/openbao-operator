package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseOSVIDsFromJSON(t *testing.T) {
	in := strings.NewReader(`{"config":{}}
{"finding":{"osv":"GO-2026-4349"}}
{"finding":{"osv":"GO-2026-4349"}}
{"finding":{"osv":"GO-2026-4348"}}`)
	ids, err := parseOSVIDsFromJSON(in)
	if err != nil {
		t.Fatalf("parseOSVIDsFromJSON: %v", err)
	}
	if strings.Join(ids, ",") != "GO-2026-4348,GO-2026-4349" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestRun_FiltersTracesToUnignored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses a bash script")
	}

	dir := t.TempDir()
	ignorePath := filepath.Join(dir, ".govulnignore")
	if err := os.WriteFile(ignorePath, []byte("GO-2026-4349\n"), 0644); err != nil {
		t.Fatalf("write ignore: %v", err)
	}

	// Fake govulncheck:
	// -json: outputs 2 vulns, exits 3
	// -show=traces: outputs 2 vuln blocks + summary, exits 3
	script := filepath.Join(dir, "govulncheck")
	scriptBody := `#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-format=json" ]]; then
  cat <<'EOF'
{"finding":{"osv":"GO-2026-4348"}}
{"finding":{"osv":"GO-2026-4349"}}
EOF
  exit 3
fi
if [[ "${1:-}" == "-show=traces" ]]; then
  cat <<'EOF'
=== Symbol Results ===

Vulnerability #1: GO-2026-4348
  Unignored block

Vulnerability #2: GO-2026-4349
  Ignored block

Your code is affected by 2 vulnerabilities from 1 module.
EOF
  exit 3
fi
echo "unexpected args: $*" >&2
exit 2
`
	if err := os.WriteFile(script, []byte(scriptBody), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := run(&out, &errOut, script, ignorePath, false, []string{"./..."})
	if code != 1 {
		t.Fatalf("code=%d; err=%q; out=%q", code, errOut.String(), out.String())
	}
	if !strings.Contains(errOut.String(), "GO-2026-4348") {
		t.Fatalf("expected unignored summary in stderr, got %q", errOut.String())
	}
	if strings.Contains(out.String(), "GO-2026-4349") {
		t.Fatalf("output should not include ignored vuln, got %q", out.String())
	}
	if !strings.Contains(out.String(), "GO-2026-4348") {
		t.Fatalf("output should include unignored vuln, got %q", out.String())
	}
	if strings.Contains(out.String(), "Your code is affected by") {
		t.Fatalf("output should not include govulncheck summary, got %q", out.String())
	}
}
