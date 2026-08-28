package cluster

import (
	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	suffixTLSCA     = "-tls-ca"
	suffixTLSServer = "-tls-server"
	suffixRootToken = "-root-token"
	suffixUnsealKey = "-unseal-key"
)

// SecretPermission describes access requirements for a named Secret.
type SecretPermission struct {
	Name       string // Secret name
	Permission string // "read" or "write"
}

// Permission constants.
const (
	PermissionRead  = "read"
	PermissionWrite = "write"
)

// GetRequiredSecretPermissions returns all Secrets the given cluster needs access to.
// This encapsulates the business logic for determining secret permissions based
// on TLS mode, unseal type, and configured secret references.
//
// Writer permissions are granted for operator-owned secrets that the operator
// must create/update. Reader permissions are granted for user-provided secrets
// that the operator only needs to read.
func GetRequiredSecretPermissions(c *openbaov1alpha1.OpenBaoCluster) []SecretPermission {
	if c == nil || c.Name == "" {
		return nil
	}

	var perms []SecretPermission

	// TLS secrets: writer if OperatorManaged, reader otherwise
	tlsCA := c.Name + suffixTLSCA
	tlsServer := c.Name + suffixTLSServer

	if c.Spec.TLS.Mode == "" || c.Spec.TLS.Mode == openbaov1alpha1.TLSModeOperatorManaged {
		perms = append(perms,
			SecretPermission{Name: tlsCA, Permission: PermissionWrite},
			SecretPermission{Name: tlsServer, Permission: PermissionWrite},
		)
	} else {
		perms = append(perms,
			SecretPermission{Name: tlsCA, Permission: PermissionRead},
			SecretPermission{Name: tlsServer, Permission: PermissionRead},
		)
	}

	// Retention secrets use reserved names. Always grant name-scoped writer
	// access so retained deletion can distinguish an absent Secret from an
	// authorization failure and can preserve a Secret created by an earlier
	// cluster configuration.
	perms = append(perms,
		SecretPermission{
			Name:       c.Name + suffixRootToken,
			Permission: PermissionWrite,
		},
		SecretPermission{
			Name:       c.Name + suffixUnsealKey,
			Permission: PermissionWrite,
		},
	)

	// Referenced secrets from spec (read-only)
	if c.Spec.Unseal != nil && c.Spec.Unseal.CredentialsSecretRef != nil {
		perms = append(perms, SecretPermission{
			Name:       c.Spec.Unseal.CredentialsSecretRef.Name,
			Permission: PermissionRead,
		})
	}

	if c.Spec.Backup != nil {
		if c.Spec.Backup.Target.CredentialsSecretRef != nil {
			perms = append(perms, SecretPermission{
				Name:       c.Spec.Backup.Target.CredentialsSecretRef.Name,
				Permission: PermissionRead,
			})
		}
		if c.Spec.Backup.TokenSecretRef != nil {
			perms = append(perms, SecretPermission{
				Name:       c.Spec.Backup.TokenSecretRef.Name,
				Permission: PermissionRead,
			})
		}
	}

	if c.Spec.ImageVerification != nil && c.Spec.ImageVerification.Enabled {
		for _, secretRef := range c.Spec.ImageVerification.ImagePullSecrets {
			perms = append(perms, SecretPermission{
				Name:       secretRef.Name,
				Permission: PermissionRead,
			})
		}
	}

	if c.Spec.OperatorImageVerification != nil && c.Spec.OperatorImageVerification.Enabled {
		for _, secretRef := range c.Spec.OperatorImageVerification.ImagePullSecrets {
			perms = append(perms, SecretPermission{
				Name:       secretRef.Name,
				Permission: PermissionRead,
			})
		}
	}

	return perms
}
