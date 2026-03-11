package helpers

import (
	"strings"
	"testing"
)

func TestBuildJWTCommandScriptSkipVerify(t *testing.T) {
	t.Parallel()

	script := buildJWTCommandScript(
		"https://tls-lifecycle-0.tls-lifecycle.testing.svc:8200",
		"e2e-test",
		jwtTLSValidationExport(""),
		"bao kv put secret/tls-lifecycle foo=bar",
	)

	expectedSubstrings := []string{
		`export BAO_ADDR="https://tls-lifecycle-0.tls-lifecycle.testing.svc:8200"`,
		`export BAO_CLIENT_TIMEOUT="5s"`,
		`export BAO_SKIP_VERIFY=true`,
		`for i in $(seq 1 15); do`,
		`role="e2e-test"`,
		`/bin/sh -eu /tmp/jwt-command.sh`,
		`bao kv put secret/tls-lifecycle foo=bar`,
	}

	for _, substring := range expectedSubstrings {
		if !strings.Contains(script, substring) {
			t.Fatalf("expected generated script to contain %q\nscript:\n%s", substring, script)
		}
	}
}

func TestBuildJWTCommandScriptWithCAValidation(t *testing.T) {
	t.Parallel()

	script := buildJWTCommandScript(
		"https://tls-lifecycle-0.tls-lifecycle.testing.svc:8200",
		"e2e-test",
		jwtTLSValidationExport("tls-lifecycle-tls-ca"),
		"echo first\necho second",
	)

	if !strings.Contains(script, `export BAO_CACERT=/var/run/secrets/openbao-ca/ca.crt`) {
		t.Fatalf("expected generated script to export BAO_CACERT\nscript:\n%s", script)
	}
	if strings.Contains(script, "BAO_SKIP_VERIFY") {
		t.Fatalf("expected generated CA-validated script not to disable TLS verification\nscript:\n%s", script)
	}
	if !strings.Contains(script, "echo first\necho second") {
		t.Fatalf("expected generated script to preserve the command body\nscript:\n%s", script)
	}
}

func TestExtractJWTCommandOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		logs string
		want string
	}{
		{
			name: "returns plain output unchanged",
			logs: "bar",
			want: "bar",
		},
		{
			name: "strips attempt prefix from successful first try",
			logs: "Attempt 1/15...\nbar\n",
			want: "bar",
		},
		{
			name: "returns output from last successful attempt",
			logs: strings.Join([]string{
				"Attempt 1/15...",
				"JWT login not ready yet",
				"Error writing data to auth/jwt-operator/login: context deadline exceeded",
				"Attempt 2/15...",
				"line one",
				"line two",
			}, "\n"),
			want: "line one\nline two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractJWTCommandOutput(tt.logs); got != tt.want {
				t.Fatalf("extractJWTCommandOutput() = %q, want %q", got, tt.want)
			}
		})
	}
}
