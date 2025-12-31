package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/dc-tec/openbao-operator/internal/upgrade"
)

const (
	exitSuccess  = 0
	exitRunError = 2
)

func run(ctx context.Context) error {
	flag.Parse()

	logger := zap.New(zap.UseDevMode(true))

	cfg, err := upgrade.LoadExecutorConfig()
	if err != nil {
		return fmt.Errorf("failed to load upgrade executor config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid upgrade executor config: %w", err)
	}

	if err := upgrade.RunExecutor(ctx, logger, cfg); err != nil {
		return err
	}

	return nil
}

func main() {
	ctx := context.Background()

	if err := run(ctx); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "upgrade-executor error: %v\n", err)
		os.Exit(exitRunError)
	}

	os.Exit(exitSuccess)
}
