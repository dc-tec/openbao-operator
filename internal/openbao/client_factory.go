package openbao

import (
	"context"
	"fmt"
)

// ClientFactory centralizes OpenBao client construction for a shared configuration template
// (e.g., per-cluster CA bundle, smart client settings, timeouts).
//
// Callers remain responsible for sourcing CA material (mounted files, Kubernetes Secrets, etc.).
type ClientFactory struct {
	template ClientConfig
}

// NewClientFactory returns a factory that creates clients based on the provided template.
// The template's BaseURL and Token are ignored; per-request values are provided to New/NewWithToken.
func NewClientFactory(template ClientConfig) *ClientFactory {
	t := template
	t.BaseURL = ""
	t.Token = ""
	return &ClientFactory{template: t}
}

// New constructs an unauthenticated client for the provided baseURL.
func (f *ClientFactory) New(baseURL string) (*Client, error) {
	if f == nil {
		return nil, fmt.Errorf("client factory is required")
	}

	cfg := f.template
	cfg.BaseURL = baseURL
	return NewClient(cfg)
}

// NewWithToken constructs an authenticated client for the provided baseURL and token.
func (f *ClientFactory) NewWithToken(baseURL, token string) (*Client, error) {
	if f == nil {
		return nil, fmt.Errorf("client factory is required")
	}

	cfg := f.template
	cfg.BaseURL = baseURL
	cfg.Token = token
	return NewClient(cfg)
}

// LoginJWT authenticates via JWT against the provided baseURL and returns the OpenBao client token.
func (f *ClientFactory) LoginJWT(ctx context.Context, baseURL, role, jwtToken string) (string, error) {
	client, err := f.New(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to create OpenBao client for JWT login: %w", err)
	}

	token, err := client.LoginJWT(ctx, role, jwtToken)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate using JWT Auth: %w", err)
	}

	return token, nil
}

// NewWithJWT constructs an authenticated client by performing JWT login against baseURL.
func (f *ClientFactory) NewWithJWT(ctx context.Context, baseURL, role, jwtToken string) (*Client, error) {
	token, err := f.LoginJWT(ctx, baseURL, role, jwtToken)
	if err != nil {
		return nil, err
	}

	client, err := f.NewWithToken(baseURL, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create authenticated OpenBao client: %w", err)
	}

	return client, nil
}
