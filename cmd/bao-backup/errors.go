package main

import (
	"errors"
	"fmt"
)

var (
	errConfigCategory       = errors.New("backup configuration error")
	errAuthCategory         = errors.New("backup authentication error")
	errLeaderCategory       = errors.New("backup leader discovery error")
	errSnapshotCategory     = errors.New("backup snapshot error")
	errStorageCategory      = errors.New("backup storage error")
	errVerificationCategory = errors.New("backup verification error")
)

type categorizedError struct {
	category error
	err      error
}

func (e *categorizedError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *categorizedError) Unwrap() []error {
	if e == nil {
		return nil
	}
	return []error{e.category, e.err}
}

func categorize(category error, err error) error {
	if err == nil {
		return nil
	}
	return &categorizedError{
		category: category,
		err:      err,
	}
}

func categorizef(category error, format string, args ...any) error {
	return categorize(category, fmt.Errorf(format, args...))
}
