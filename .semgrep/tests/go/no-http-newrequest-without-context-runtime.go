package semgreptest

import (
	"context"
	"net/http"
)

func withoutContext() (*http.Request, error) {
	// ruleid: no-http-newrequest-without-context-runtime
	return http.NewRequest(http.MethodGet, "https://example.com", nil)
}

func withContext(ctx context.Context) (*http.Request, error) {
	// ok: no-http-newrequest-without-context-runtime
	return http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", nil)
}
