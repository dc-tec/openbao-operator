package opslifecycle

import (
	"os"
	"time"
)

var (
	requeueShort    = 5 * time.Second
	requeueStandard = 1 * time.Minute
)

func init() {
	if val := os.Getenv("OPENBAO_REQUEUE_STANDARD"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			requeueStandard = d
		}
	}
}

// RetryClass classifies reconcile retries for long-running operations.
type RetryClass string

const (
	RetryClassLockContention RetryClass = "lock-contention"
	RetryClassProgressPoll   RetryClass = "progress-poll"
	RetryClassStandard       RetryClass = "standard"
)

// RequeueDelay maps retry intent to the default delay.
func RequeueDelay(class RetryClass) time.Duration {
	switch class {
	case RetryClassLockContention, RetryClassProgressPoll:
		return requeueShort
	case RetryClassStandard:
		return requeueStandard
	default:
		return requeueStandard
	}
}
