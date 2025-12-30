package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun_MissingConfig(t *testing.T) {
	// Ensure run fails if config is missing (no env vars set)
	err := run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "required")
}
