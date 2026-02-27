package opslifecycle

import (
	"time"

	"github.com/dc-tec/openbao-operator/internal/constants"
)

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
		return constants.RequeueShort
	case RetryClassStandard:
		return constants.RequeueStandard
	default:
		return constants.RequeueStandard
	}
}
