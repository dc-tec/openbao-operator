package constants

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRequeueStandardOverride(t *testing.T) {
	// Save original value to restore after test
	original := RequeueStandard
	defer func() { RequeueStandard = original }()

	tests := []struct {
		name     string
		envVal   string
		expected time.Duration
	}{
		{
			name:     "Default",
			envVal:   "",
			expected: 1 * time.Minute,
		},
		{
			name:     "Override 10s",
			envVal:   "10s",
			expected: 10 * time.Second,
		},
		{
			name:     "Invalid Duration",
			envVal:   "invalid",
			expected: 1 * time.Minute, // Should fallback to default (restored from original)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to original value before each test to prevent state pollution
			RequeueStandard = original

			if tt.envVal != "" {
				os.Setenv("OPENBAO_REQUEUE_STANDARD", tt.envVal)
				defer os.Unsetenv("OPENBAO_REQUEUE_STANDARD")
			}

			// Re-simulate init logic
			if val := os.Getenv("OPENBAO_REQUEUE_STANDARD"); val != "" {
				if d, err := time.ParseDuration(val); err == nil {
					RequeueStandard = d
				}
			}

			assert.Equal(t, tt.expected, RequeueStandard)
		})
	}
}
