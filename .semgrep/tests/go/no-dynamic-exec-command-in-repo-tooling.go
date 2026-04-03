package semgreptest

import (
	"context"
	"os/exec"
)

func dynamicExec(name string, args []string) *exec.Cmd {
	// ruleid: no-dynamic-exec-command-in-repo-tooling
	return exec.Command(name, args...)
}

func dynamicExecContext(ctx context.Context, name string, args []string) *exec.Cmd {
	// ruleid: no-dynamic-exec-command-in-repo-tooling
	return exec.CommandContext(ctx, name, args...)
}

func staticExec(args []string) *exec.Cmd {
	// ok: no-dynamic-exec-command-in-repo-tooling
	return exec.Command("gh", args...)
}

func staticExecContext(ctx context.Context, args []string) *exec.Cmd {
	// ok: no-dynamic-exec-command-in-repo-tooling
	return exec.CommandContext(ctx, "go", args...)
}
