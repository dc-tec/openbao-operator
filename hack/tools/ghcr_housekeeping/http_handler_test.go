package main

import (
	"fmt"
	"sync"
	"testing"
)

type httpHandlerErrors struct {
	mu       sync.Mutex
	messages []string
}

func newHTTPHandlerErrors(t *testing.T) *httpHandlerErrors {
	t.Helper()

	errors := &httpHandlerErrors{}
	t.Cleanup(func() {
		errors.mu.Lock()
		defer errors.mu.Unlock()

		for _, message := range errors.messages {
			t.Errorf("HTTP handler: %s", message)
		}
	})
	return errors
}

func (e *httpHandlerErrors) Errorf(format string, args ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = append(e.messages, fmt.Sprintf(format, args...))
}
