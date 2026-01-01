package openbao

import (
	"context"
	"fmt"
	"io"
	"net/http"

	operatorerrors "github.com/dc-tec/openbao-operator/internal/errors"
)

func (c *Client) newRequest(ctx context.Context, method string, path string, body io.Reader) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
}

func (c *Client) doRequest(req *http.Request, httpClient *http.Client, op string) (*http.Response, error) {
	if httpClient == nil {
		httpClient = c.httpClient
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		wrapped := fmt.Errorf("%s: %w", op, err)
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
		return nil, nil, fmt.Errorf("%s: failed to read response body: %w", op, err)
	}

	return resp, body, nil
}
