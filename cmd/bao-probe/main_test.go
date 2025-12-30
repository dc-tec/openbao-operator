package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun_InvalidMode(t *testing.T) {
	// flag.Parse() reads os.Args. We can't easily change os.Args in parallel tests,
	// but default mode is "" which is invalid.
	err := run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid -mode")
}
