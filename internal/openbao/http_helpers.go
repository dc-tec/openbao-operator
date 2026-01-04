package openbao

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/dc-tec/openbao-operator/internal/constants"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

func (c *Client) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
}

func (c *Client) doRequest(req *http.Request, httpClient *http.Client, op string) (*http.Response, error) {
	if httpClient == nil {
		httpClient = c.httpClient
	}

	if c.smart != nil {
		if err := c.smart.allow(req.Context(), req); err != nil {
			return nil, err
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("%s: %w", op, err)
		if c.smart != nil {
			c.smart.after(req, false)
		}
		if operatorerrors.IsTransientConnection(err) {
			return nil, operatorerrors.WrapTransientConnection(wrapped)
		}
		return nil, wrapped
	}
	return resp, nil
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func (c *Client) doAndReadAll(req *http.Request, httpClient *http.Client, op string) (*http.Response, []byte, error) {
	resp, err := c.doRequest(req, httpClient, op)
	if err != nil {
		return nil, nil, err
	}

	defer drainAndClose(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if c.smart != nil {
			c.smart.after(req, false)
		}
		return nil, nil, fmt.Errorf("%s: failed to read response body: %w", op, err)
	}

	// The health endpoint encodes state in HTTP status codes (sealed, standby, etc.),
	// so we must not classify 429/5xx responses as overload.
	if req.URL != nil && req.URL.Path != constants.APIPathSysHealth {
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if c.smart != nil {
				c.smart.after(req, false)
			}
			return nil, nil, operatorerrors.WrapTransientRemoteOverloaded(
				fmt.Errorf("%s: OpenBao API overloaded (status %d): %s", op, resp.StatusCode, string(body)),
			)
		}
	}

	if c.smart != nil {
		c.smart.after(req, true)
	}
	return resp, body, nil
}
