package openbao

import "context"

// PolicyClient reads and updates ACL policies using independently granted credentials.
type PolicyClient interface {
	// ReadACLPolicy returns nil when the policy does not exist.
	ReadACLPolicy(context.Context, string) (*string, error)
	WriteACLPolicy(context.Context, string, string) error
}
