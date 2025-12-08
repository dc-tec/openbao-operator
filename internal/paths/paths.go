package paths

// TLSCACertPath is the default path to the operator-managed TLS CA certificate
// on OpenBao pods and helper executables.
const TLSCACertPath = "/etc/bao/tls/ca.crt"

// BackupTokenPath is the default path to the static backup token file mounted
// into the backup Job.
const BackupTokenPath = "/etc/bao/backup/token/token"

// BackupCredentialsPath is the default path to the directory containing backup
// storage credentials.
const BackupCredentialsPath = "/etc/bao/backup/credentials"

// BackupJWTTokenPath is the default path to the projected ServiceAccount token
// used for JWT authentication in the backup Job.
const BackupJWTTokenPath = "/var/run/secrets/tokens/openbao-token"
