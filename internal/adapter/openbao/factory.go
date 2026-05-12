package openbao

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ClientFactory centralizes OpenBao client construction for a shared configuration template
// (e.g., per-cluster CA bundle, client settings, timeouts).
//
// Callers remain responsible for sourcing CA material (mounted files, Kubernetes Secrets, etc.).
type ClientFactory struct {
	template portopenbao.ClientConfig

	mu        sync.RWMutex
	clients   map[string]*http.Client
	jwtTokens *jwtTokenCache

	// clientState is the shared client state for this factory.
	clientState *clientState
}

// newClientFactoryWithState is the internal constructor used by ClientManager.
// The factory will use the provided clientState.
func newClientFactoryWithState(template portopenbao.ClientConfig, state *clientState) *ClientFactory {
	t := template
	t.BaseURL = ""
	t.Token = ""
	jwtTokens := newJWTTokenCache()
	if state != nil && state.jwtTokens != nil {
		jwtTokens = state.jwtTokens
	}
	return &ClientFactory{
		template:    t,
		clients:     make(map[string]*http.Client),
		jwtTokens:   jwtTokens,
		clientState: state,
	}
}

// New constructs an unauthenticated client for the provided baseURL.
func (f *ClientFactory) New(baseURL string) (*Client, error) {
	if f == nil {
		return nil, fmt.Errorf("client factory is required")
	}

	// Check cache first
	f.mu.RLock()
	cachedClient, ok := f.clients[baseURL]
	f.mu.RUnlock()

	var httpClient *http.Client

	if ok {
		httpClient = cachedClient
	} else {
		// Create a new client (which creates a new transport and client state)
		cfg := f.template
		cfg.BaseURL = baseURL
		tempClient, err := NewClient(cfg)
		if err != nil {
			return nil, err
		}
		httpClient = tempClient.httpClient

		// Cache the http.Client
		f.mu.Lock()
		f.clients[baseURL] = httpClient
		f.mu.Unlock()
	}

	// We still return a new *Client struct because it might hold per-request state (like Token)
	// But we inject the reused http.Client
	cfg := f.template
	cfg.BaseURL = baseURL

	// Construct client with explicit client state if available
	client, err := newClientWithState(cfg, f.clientState)
	if err != nil {
		return nil, err
	}
	client.httpClient = httpClient
	return client, nil
}

// NewWithToken constructs an authenticated client for the provided baseURL and token.
func (f *ClientFactory) NewWithToken(baseURL, token string) (*Client, error) {
	if f == nil {
		return nil, fmt.Errorf("client factory is required")
	}

	client, err := f.New(baseURL)
	if err != nil {
		return nil, err
	}
	client.token = token
	return client, nil
}

// LoginJWT authenticates via JWT against the provided baseURL and returns the OpenBao client token.
func (f *ClientFactory) LoginJWT(ctx context.Context, baseURL, role, jwtToken string) (string, error) {
	if f == nil {
		return "", fmt.Errorf("client factory is required")
	}

	token, err := f.jwtTokens.getOrLogin(ctx, baseURL, role, jwtToken, func(ctx context.Context) (string, int, error) {
		client, err := f.New(baseURL)
		if err != nil {
			return "", 0, fmt.Errorf("failed to create OpenBao client for JWT login: %w", err)
		}
		token, ttl, err := client.LoginJWT(ctx, role, jwtToken)
		if err != nil {
			return "", 0, fmt.Errorf("failed to authenticate using JWT Auth: %w", err)
		}
		return token, ttl, nil
	})
	if err != nil {
		return "", err
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
