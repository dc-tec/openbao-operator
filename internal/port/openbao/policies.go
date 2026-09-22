package openbao

import "context"

// PolicyWriter updates ACL policies using independently granted credentials.
type PolicyWriter interface {
	WriteACLPolicy(context.Context, string, string) error
}
