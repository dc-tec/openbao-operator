package openbao

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type cachedToken struct {
	token      string
	expiration time.Time
}

// ClientFactory centralizes OpenBao client construction for a shared configuration template
// (e.g., per-cluster CA bundle, client settings, timeouts).
//
// Callers remain responsible for sourcing CA material (mounted files, Kubernetes Secrets, etc.).
type ClientFactory struct {
	template ClientConfig

	mu         sync.RWMutex
	clients    map[string]*http.Client
	tokenCache map[string]cachedToken

	// clientState is the shared client state for this factory.
	clientState *clientState
}

// NewClientFactory returns a factory that creates clients based on the provided template.
// The template's BaseURL and Token are ignored; per-request values are provided to New/NewWithToken.
//
// Deprecated: This function creates a factory without shared state management.
// New code should use ClientManager.FactoryFor() for explicit state management.
func NewClientFactory(template ClientConfig) *ClientFactory {
	t := template
	t.BaseURL = ""
	t.Token = ""
	return &ClientFactory{
		template:   t,
		clients:    make(map[string]*http.Client),
		tokenCache: make(map[string]cachedToken),
	}
}

// newClientFactoryWithState is the internal constructor used by ClientManager.
// The factory will use the provided clientState.
func newClientFactoryWithState(template ClientConfig, state *clientState) *ClientFactory {
	t := template
	t.BaseURL = ""
	t.Token = ""
	return &ClientFactory{
		template:    t,
		clients:     make(map[string]*http.Client),
		tokenCache:  make(map[string]cachedToken),
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
	// Check token cache
	f.mu.RLock()
	cached, ok := f.tokenCache[role]
	f.mu.RUnlock()

	if ok && time.Now().Before(cached.expiration) {
		return cached.token, nil
	}

	client, err := f.New(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to create OpenBao client for JWT login: %w", err)
	}

	token, ttl, err := client.LoginJWT(ctx, role, jwtToken)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate using JWT Auth: %w", err)
	}

	// Cache the token with some buffer (e.g., 5 seconds or 10% of TTL)
	// OpenBao TTL is in seconds.
	if ttl > 0 {
		f.mu.Lock()
		// Expire 10 seconds early to be safe
		expiration := time.Now().Add(time.Duration(ttl)*time.Second - 10*time.Second)
		if time.Now().Before(expiration) {
			f.tokenCache[role] = cachedToken{
				token:      token,
				expiration: expiration,
			}
		}
		f.mu.Unlock()
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
