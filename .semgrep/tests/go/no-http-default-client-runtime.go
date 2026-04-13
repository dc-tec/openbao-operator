package semgreptest

import "net/http"

func useDefaultClient() *http.Client {
	// ruleid: no-http-default-client-runtime
	return http.DefaultClient
}

func useDefaultGet() {
	// ruleid: no-http-default-client-runtime
	_, _ = http.Get("https://example.com")
}

func explicitClient() *http.Client {
	// ok: no-http-default-client-runtime
	return &http.Client{Timeout: 5}
}
