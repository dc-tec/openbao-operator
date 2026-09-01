package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestContainerRegistryClientListsManifestReferencesAndCachesToken(t *testing.T) {
	t.Parallel()

	tokenRequests := 0
	manifestRequests := 0
	handlerErrors := newHTTPHandlerErrors(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			tokenRequests++
			username, password, ok := r.BasicAuth()
			if !ok || username != "github-actions[bot]" || password != "github-token" {
				handlerErrors.Errorf("unexpected registry token credentials: username=%q ok=%t", username, ok)
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			if got := r.URL.Query().Get("scope"); got != "repository:dc-tec/openbao-operator:pull" {
				handlerErrors.Errorf("scope = %q", got)
				http.Error(w, "invalid scope", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "registry-token"})
		case strings.HasPrefix(r.URL.Path, "/v2/dc-tec/openbao-operator/manifests/"):
			manifestRequests++
			if got := r.Header.Get("Authorization"); got != "Bearer registry-token" {
				handlerErrors.Errorf("authorization = %q", got)
				http.Error(w, "invalid authorization", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"schemaVersion": 2,
				"mediaType":     mediaTypeOCIImageIndex,
				"manifests": []map[string]string{
					{"digest": testDigest("b"), "mediaType": "application/vnd.oci.image.manifest.v1+json"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &githubContainerRegistryClient{
		baseURL:      server.URL,
		tokenURL:     server.URL + "/token",
		githubToken:  "github-token",
		registryUser: "github-actions[bot]",
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}

	for range 2 {
		references, err := client.ManifestReferences(
			context.Background(),
			"dc-tec",
			"openbao-operator",
			testDigest("a"),
		)
		if err != nil {
			t.Fatalf("ManifestReferences() error = %v", err)
		}
		if len(references) != 1 || references[0].Digest != testDigest("b") {
			t.Fatalf("references = %#v", references)
		}
	}
	if tokenRequests != 1 {
		t.Fatalf("token requests = %d, want 1", tokenRequests)
	}
	if manifestRequests != 2 {
		t.Fatalf("manifest requests = %d, want 2", manifestRequests)
	}
}

func TestContainerRegistryClientFailsOnMissingManifest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "registry-token"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &githubContainerRegistryClient{
		baseURL:    server.URL,
		tokenURL:   server.URL + "/token",
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	_, err := client.ManifestReferences(
		context.Background(),
		"dc-tec",
		"openbao-operator",
		testDigest("a"),
	)
	if err == nil || !strings.Contains(err.Error(), "fetch manifest failed (404)") {
		t.Fatalf("ManifestReferences() error = %v, want not found", err)
	}
}
