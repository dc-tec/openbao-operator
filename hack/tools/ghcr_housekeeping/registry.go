package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	defaultRegistryBaseURL  = "https://ghcr.io"
	defaultRegistryTokenURL = "https://ghcr.io/token"
	registryService         = "ghcr.io"
	manifestAcceptHeader    = "application/vnd.oci.image.index.v1+json, " +
		"application/vnd.oci.image.manifest.v1+json, " +
		"application/vnd.docker.distribution.manifest.list.v2+json, " +
		"application/vnd.docker.distribution.manifest.v2+json"
)

type githubContainerRegistryClient struct {
	baseURL      string
	tokenURL     string
	githubToken  string
	registryUser string
	httpClient   *http.Client
	tokens       map[string]string
}

func (c *githubContainerRegistryClient) ManifestReferences(
	ctx context.Context,
	owner, pkg, digest string,
) ([]manifestReference, error) {
	if !strings.HasPrefix(digest, "sha256:") {
		return nil, fmt.Errorf("unsupported manifest digest %q", digest)
	}

	repository := strings.TrimSpace(owner) + "/" + strings.TrimSpace(pkg)
	token, err := c.repositoryToken(ctx, repository)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf(
		"%s/v2/%s/%s/manifests/%s",
		strings.TrimRight(c.baseURL, "/"),
		url.PathEscape(strings.TrimSpace(owner)),
		url.PathEscape(strings.TrimSpace(pkg)),
		url.PathEscape(digest),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}
	req.Header.Set("Accept", manifestAcceptHeader)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch manifest failed (%d): %s", resp.StatusCode, extractAPIErrorMessage(resp))
	}

	var manifest struct {
		Manifests []struct {
			Digest    string `json:"digest"`
			MediaType string `json:"mediaType"`
		} `json:"manifests"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	references := make([]manifestReference, 0, len(manifest.Manifests))
	for _, item := range manifest.Manifests {
		digest := strings.TrimSpace(item.Digest)
		if digest == "" {
			return nil, errors.New("manifest contains an empty child digest")
		}
		references = append(references, manifestReference{
			Digest:    digest,
			MediaType: strings.TrimSpace(item.MediaType),
		})
	}
	return references, nil
}

func (c *githubContainerRegistryClient) repositoryToken(ctx context.Context, repository string) (string, error) {
	if c.tokens == nil {
		c.tokens = make(map[string]string)
	}
	if token := c.tokens[repository]; token != "" {
		return token, nil
	}

	endpoint, err := url.Parse(c.tokenURL)
	if err != nil {
		return "", fmt.Errorf("parse registry token URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("service", registryService)
	query.Set("scope", "repository:"+repository+":pull")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create registry token request: %w", err)
	}
	if strings.TrimSpace(c.githubToken) != "" {
		req.SetBasicAuth(c.registryUser, c.githubToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request registry token: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("request registry token failed (%d): %s", resp.StatusCode, extractAPIErrorMessage(resp))
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode registry token: %w", err)
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		token = strings.TrimSpace(payload.AccessToken)
	}
	if token == "" {
		return "", errors.New("registry token response did not include a token")
	}

	c.tokens[repository] = token
	return token, nil
}
