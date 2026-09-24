package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ReadACLPolicy returns the exact stored contents, or nil when the policy is missing.
func (c *Client) ReadACLPolicy(ctx context.Context, name string) (*string, error) {
	if err := c.requireAuth("ACL policy read"); err != nil {
		return nil, err
	}
	if name == "" || strings.ContainsAny(name, "/?#%") {
		return nil, fmt.Errorf("a policy name without path delimiters is required")
	}
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/sys/policies/acl/"+name, nil)
	if err != nil {
		return nil, err
	}
	if err := c.authorize(req); err != nil {
		return nil, err
	}
	status, response, err := c.doAndReadAll(req, nil, "read ACL policy")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, portopenbao.NewAPIError("read ACL policy", status, response)
	}
	var envelope struct {
		Data struct {
			Policy *string `json:"policy"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return nil, fmt.Errorf("decode ACL policy: %w", err)
	}
	if envelope.Data.Policy == nil {
		return nil, fmt.Errorf("ACL policy response is missing policy contents")
	}
	return envelope.Data.Policy, nil
}

// WriteACLPolicy sends only the policy parameter so OpenBao can restrict its
// exact value with allowed_parameters in a separate approval policy.
func (c *Client) WriteACLPolicy(ctx context.Context, name, policy string) error {
	if err := c.requireAuth("ACL policy write"); err != nil {
		return err
	}
	if name == "" || strings.ContainsAny(name, "/?#%") || strings.TrimSpace(policy) == "" {
		return fmt.Errorf("a policy name without path delimiters and nonempty contents are required")
	}
	body, err := json.Marshal(struct {
		Policy string `json:"policy"`
	}{Policy: policy})
	if err != nil {
		return fmt.Errorf("encode ACL policy: %w", err)
	}
	req, err := c.newRequest(ctx, http.MethodPut, "/v1/sys/policies/acl/"+name, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if err := c.authorize(req); err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	status, response, err := c.doAndReadAll(req, nil, "write ACL policy")
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return portopenbao.NewAPIError("write ACL policy", status, response)
	}
	return nil
}
