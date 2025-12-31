package main

import (
	"testing"
	"time"
)

func TestComputeDeterministicDebounceJitter(t *testing.T) {
	tests := []struct {
		name              string
		clusterName       string
		jitterRangeSecond int64
		wantZero          bool
	}{
		{
			name:              "empty cluster name yields zero jitter",
			clusterName:       "",
			jitterRangeSecond: 5,
			wantZero:          true,
		},
		{
			name:              "zero jitter range yields zero jitter",
			clusterName:       "test-cluster",
			jitterRangeSecond: 0,
			wantZero:          true,
		},
		{
			name:              "negative jitter range yields zero jitter",
			clusterName:       "test-cluster",
			jitterRangeSecond: -1,
			wantZero:          true,
		},
		{
			name:              "jitter is deterministic and within range",
			clusterName:       "test-cluster",
			jitterRangeSecond: 5,
			wantZero:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got1 := computeDeterministicDebounceJitter(tt.clusterName, tt.jitterRangeSecond)
			got2 := computeDeterministicDebounceJitter(tt.clusterName, tt.jitterRangeSecond)

			if got1 != got2 {
				t.Fatalf("jitter should be deterministic: got %v and %v", got1, got2)
			}

			if tt.wantZero {
				if got1 != 0 {
					t.Fatalf("expected zero jitter, got %v", got1)
				}
				return
			}

			if got1 < 0 {
				t.Fatalf("jitter must be non-negative, got %v", got1)
			}

			max := time.Duration(tt.jitterRangeSecond) * time.Second
			if got1 >= max {
				t.Fatalf("jitter must be < %v, got %v", max, got1)
			}
		})
	}
}
