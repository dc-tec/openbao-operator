package blobstore

// Secret key names expected in storage credentials Secrets.
const (
	SecretKeyAccessKeyID     = "accessKeyId"
	SecretKeySecretAccessKey = "secretAccessKey"
	SecretKeySessionToken    = "sessionToken"
	SecretKeyRegion          = "region"
	SecretKeyCACert          = "caCert"
)

// Credentials is the shared contract for object-storage credentials.
// It is intentionally placed in a port package so helper adapters can
// decode credentials without importing concrete storage implementations.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	CACert          []byte
}
