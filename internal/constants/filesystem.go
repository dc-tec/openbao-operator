package constants

// Common mount paths used by OpenBao pods and helper executables.
const (
	PathTLS    = "/etc/bao/tls"
	PathConfig = "/etc/bao/config"
	PathData   = "/bao/data"
)

// Common volume names used by OpenBao pods.
const (
	VolumeTLS    = "tls"
	VolumeConfig = "config"
	VolumeData   = "data"
)

// TLS file paths mounted into OpenBao pods and helper executables.
const (
	PathTLSCACert     = PathTLS + "/ca.crt"
	PathTLSServerCert = PathTLS + "/tls.crt"
	PathTLSServerKey  = PathTLS + "/tls.key"
)

// Backup executor mounted file paths.
const (
	PathBackupToken       = "/etc/bao/backup/token/token"           // #nosec G101 -- This is a file path constant, not a credential
	PathBackupCredentials = "/etc/bao/backup/credentials"           // #nosec G101 -- This is a file path constant, not a credential
	PathBackupJWTToken    = "/var/run/secrets/tokens/openbao-token" // #nosec G101 -- This is a file path constant, not a credential
)
