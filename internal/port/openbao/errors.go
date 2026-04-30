package openbao

import (
	"errors"
	"fmt"
	"strings"
)

const apiErrorBodyLimit = 2048

var (
	// ErrAlreadyInitialized indicates OpenBao rejected init because the cluster is already initialized.
	ErrAlreadyInitialized = errors.New("OpenBao cluster already initialized")
	// ErrAlreadyJoined indicates a raft join request was a no-op because the node is already in the cluster.
	ErrAlreadyJoined = errors.New("OpenBao raft node already joined")
	// ErrAlreadyVoter indicates a raft promote request was a no-op because the node is already a voter.
	ErrAlreadyVoter = errors.New("OpenBao raft node already voter")
	// ErrAlreadyNonVoter indicates a raft demote request was a no-op because the node is already a non-voter.
	ErrAlreadyNonVoter = errors.New("OpenBao raft node already non-voter")
)

// APIError captures an OpenBao API failure with a machine-readable HTTP status.
type APIError struct {
	Operation    string
	StatusCode   int
	ResponseBody string
}

// Error returns a stable text form while preserving typed access to the HTTP status.
func (e *APIError) Error() string {
	if e == nil {
		return ""
	}

	operation := strings.TrimSpace(e.Operation)
	if operation == "" {
		operation = "OpenBao API request failed"
	}

	if strings.TrimSpace(e.ResponseBody) == "" {
		return fmt.Sprintf("%s with status %d", operation, e.StatusCode)
	}

	return fmt.Sprintf("%s with status %d: %s", operation, e.StatusCode, e.ResponseBody)
}

// HTTPStatusCode returns the underlying OpenBao API response status code.
func (e *APIError) HTTPStatusCode() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

// NewAPIError constructs a typed OpenBao API error and normalizes the response body.
func NewAPIError(operation string, statusCode int, responseBody []byte) *APIError {
	return &APIError{
		Operation:    strings.TrimSpace(operation),
		StatusCode:   statusCode,
		ResponseBody: normalizeAPIErrorBody(responseBody),
	}
}

func normalizeAPIErrorBody(responseBody []byte) string {
	body := strings.TrimSpace(string(responseBody))
	if body == "" {
		return ""
	}
	if len(body) > apiErrorBodyLimit {
		return body[:apiErrorBodyLimit] + "..."
	}
	return body
}

type httpStatusCoder interface {
	error
	HTTPStatusCode() int
}

// StatusCode extracts the OpenBao HTTP status code from an error chain when present.
func StatusCode(err error) (int, bool) {
	var statusErr httpStatusCoder
	if errors.As(err, &statusErr) {
		return statusErr.HTTPStatusCode(), true
	}
	return 0, false
}

// IsStatus reports whether an error chain contains an OpenBao API error with the given status code.
func IsStatus(err error, statusCode int) bool {
	got, ok := StatusCode(err)
	return ok && got == statusCode
}
