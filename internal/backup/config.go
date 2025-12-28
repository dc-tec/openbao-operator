package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openbao/operator/internal/constants"
	"github.com/openbao/operator/internal/storage"
)

// ExecutorConfig holds the backup executor configuration.
type ExecutorConfig struct {
	// Cluster information
	ClusterNamespace string
	ClusterName      string
	ClusterReplicas  int32

	// Storage configuration
	BackupEndpoint       string
	BackupBucket         string
	BackupPathPrefix     string
	BackupFilenamePrefix string // Added to support pre-upgrade prefixes
	BackupUsePathStyle   bool
	BackupRegion         string

	// Authentication
	AuthMethod   string
	JWTAuthRole  string
	OpenBaoToken string
	JWTToken     string

	// TLS
	TLSCACert []byte

	// Storage credentials
	StorageCredentials *storage.Credentials

	// S3 upload configuration
	PartSize    int64
	Concurrency int32
}

// Validate validates the configuration and returns an error if invalid.
func (c *ExecutorConfig) Validate() error {
	if c.ClusterNamespace == "" {
		return fmt.Errorf("cluster namespace is required")
	}
	if c.ClusterName == "" {
		return fmt.Errorf("cluster name is required")
	}
	if c.ClusterReplicas <= 0 {
		return fmt.Errorf("cluster replicas must be greater than 0")
	}
	if c.BackupEndpoint == "" {
		return fmt.Errorf("backup endpoint is required")
	}
	if c.BackupBucket == "" {
		return fmt.Errorf("backup bucket is required")
	}
	if len(c.TLSCACert) == 0 {
		return fmt.Errorf("TLS CA certificate is required")
	}
	switch c.AuthMethod {
	case constants.BackupAuthMethodJWT:
		if c.JWTAuthRole == "" {
			return fmt.Errorf("JWT auth role is required when using JWT authentication")
		}
		if c.JWTToken == "" {
			return fmt.Errorf("JWT token is required when using JWT authentication")
		}
	case constants.BackupAuthMethodToken:
		if c.OpenBaoToken == "" {
			return fmt.Errorf("OpenBao token is required when using token authentication")
		}
	default:
		return fmt.Errorf("invalid auth method: %q", c.AuthMethod)
	}
	return nil
}

// LoadExecutorConfig loads configuration from environment variables and mounted files.
func LoadExecutorConfig() (*ExecutorConfig, error) {
	cfg := &ExecutorConfig{}

	if err := loadClusterConfig(cfg); err != nil {
		return nil, err
	}
	if err := loadStorageConfig(cfg); err != nil {
		return nil, err
	}
	if err := loadTLSConfig(cfg); err != nil {
		return nil, err
	}
	if err := loadAuthConfig(cfg); err != nil {
		return nil, err
	}
	if err := loadUploadConfig(cfg); err != nil {
		return nil, err
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadClusterConfig loads cluster namespace, name, and replicas from environment variables.
func loadClusterConfig(cfg *ExecutorConfig) error {
	cfg.ClusterNamespace = strings.TrimSpace(os.Getenv(constants.EnvClusterNamespace))
	if cfg.ClusterNamespace == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterNamespace)
	}

	cfg.ClusterName = strings.TrimSpace(os.Getenv(constants.EnvClusterName))
	if cfg.ClusterName == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterName)
	}

	replicasStr := strings.TrimSpace(os.Getenv(constants.EnvClusterReplicas))
	if replicasStr == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvClusterReplicas)
	}
	replicas, err := strconv.ParseInt(replicasStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid CLUSTER_REPLICAS value %q: %w", replicasStr, err)
	}
	cfg.ClusterReplicas = int32(replicas)
	return nil
}

// loadStorageConfig loads backup storage endpoint, bucket, and path configuration.
func loadStorageConfig(cfg *ExecutorConfig) error {
	cfg.BackupEndpoint = strings.TrimSpace(os.Getenv(constants.EnvBackupEndpoint))
	if cfg.BackupEndpoint == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvBackupEndpoint)
	}

	cfg.BackupBucket = strings.TrimSpace(os.Getenv(constants.EnvBackupBucket))
	if cfg.BackupBucket == "" {
		return fmt.Errorf("%s environment variable is required", constants.EnvBackupBucket)
	}

	cfg.BackupPathPrefix = strings.TrimSpace(os.Getenv(constants.EnvBackupPathPrefix))
	cfg.BackupFilenamePrefix = strings.TrimSpace(os.Getenv(constants.EnvBackupFilenamePrefix))

	cfg.BackupRegion = strings.TrimSpace(os.Getenv(constants.EnvBackupRegion))
	if cfg.BackupRegion == "" {
		cfg.BackupRegion = constants.DefaultS3Region
	}

	usePathStyleStr := strings.TrimSpace(os.Getenv(constants.EnvBackupUsePathStyle))
	if usePathStyleStr != "" {
		usePathStyle, err := strconv.ParseBool(usePathStyleStr)
		if err != nil {
			return fmt.Errorf("invalid BACKUP_USE_PATH_STYLE value %q: %w", usePathStyleStr, err)
		}
		cfg.BackupUsePathStyle = usePathStyle
	}

	// Load storage credentials
	credsPath := constants.PathBackupCredentials
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvBackupCredentialsPath)); envPath != "" {
		credsPath = envPath
	}

	creds, err := loadStorageCredentials(credsPath)
	if err != nil {
		return fmt.Errorf("failed to load storage credentials: %w", err)
	}
	cfg.StorageCredentials = creds
	return nil
}

// loadTLSConfig loads the TLS CA certificate from a file.
func loadTLSConfig(cfg *ExecutorConfig) error {
	caCertPath := constants.PathTLSCACert
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvTLSCAPath)); envPath != "" {
		caCertPath = envPath
	}
	caCert, err := os.ReadFile(caCertPath) //#nosec G304 -- Path from constant or environment variable
	if err != nil {
		return fmt.Errorf("failed to read TLS CA certificate from %q: %w", caCertPath, err)
	}
	cfg.TLSCACert = caCert
	return nil
}

// loadAuthConfig loads authentication configuration (JWT or static token).
func loadAuthConfig(cfg *ExecutorConfig) error {
	cfg.AuthMethod = strings.TrimSpace(os.Getenv(constants.EnvBackupAuthMethod))

	// Try to load JWT token from projected volume
	jwtTokenPath := constants.PathBackupJWTToken
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvJWTTokenPath)); envPath != "" {
		jwtTokenPath = envPath
	}
	jwtToken, err := os.ReadFile(jwtTokenPath) //#nosec G304 -- Path from constant or environment variable
	if err == nil && len(jwtToken) > 0 {
		cfg.JWTToken = strings.TrimSpace(string(jwtToken))
		// If auth method not explicitly set, prefer JWT Auth
		if cfg.AuthMethod == "" {
			cfg.AuthMethod = constants.BackupAuthMethodJWT
		}
	}

	// Load JWT Auth role if using JWT Auth
	if cfg.AuthMethod == constants.BackupAuthMethodJWT || (cfg.AuthMethod == "" && cfg.JWTToken != "") {
		cfg.JWTAuthRole = strings.TrimSpace(os.Getenv(constants.EnvBackupJWTAuthRole))
		if cfg.JWTAuthRole == "" {
			return fmt.Errorf("BACKUP_JWT_AUTH_ROLE is required when using JWT authentication")
		}
		if cfg.JWTToken == "" {
			return fmt.Errorf("JWT token not found at %q", jwtTokenPath)
		}
		cfg.AuthMethod = constants.BackupAuthMethodJWT
		return nil
	}

	// Fall back to static token
	tokenPath := constants.PathBackupToken
	if envPath := strings.TrimSpace(os.Getenv(constants.EnvBackupTokenPath)); envPath != "" {
		tokenPath = envPath
	}
	token, err := os.ReadFile(tokenPath) //#nosec G304 -- Path from constant or environment variable
	if err != nil {
		return fmt.Errorf("failed to read OpenBao token from %q: %w", tokenPath, err)
	}
	cfg.OpenBaoToken = strings.TrimSpace(string(token))
	if cfg.OpenBaoToken == "" {
		return fmt.Errorf("OpenBao token is empty")
	}
	cfg.AuthMethod = constants.BackupAuthMethodToken
	return nil
}

// loadUploadConfig loads S3 upload configuration (part size and concurrency).
func loadUploadConfig(cfg *ExecutorConfig) error {
	partSizeStr := strings.TrimSpace(os.Getenv(constants.EnvBackupPartSize))
	if partSizeStr != "" {
		partSize, err := strconv.ParseInt(partSizeStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid BACKUP_PART_SIZE value %q: %w", partSizeStr, err)
		}
		cfg.PartSize = partSize
	}

	concurrencyStr := strings.TrimSpace(os.Getenv(constants.EnvBackupConcurrency))
	if concurrencyStr != "" {
		concurrency, err := strconv.ParseInt(concurrencyStr, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid BACKUP_CONCURRENCY value %q: %w", concurrencyStr, err)
		}
		cfg.Concurrency = int32(concurrency)
	}
	return nil
}

// validatePath ensures a file path is within the base directory and doesn't contain path traversal.
func validatePath(baseDir, filePath string) (string, error) {
	// Clean the base directory and file path
	cleanBase := filepath.Clean(baseDir)
	cleanPath := filepath.Clean(filePath)

	// Ensure the path is within the base directory
	relPath, err := filepath.Rel(cleanBase, cleanPath)
	if err != nil {
		return "", fmt.Errorf("invalid path %q relative to base %q: %w", filePath, baseDir, err)
	}

	// Check for path traversal attempts
	if strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("path %q attempts to escape base directory %q", filePath, baseDir)
	}

	return cleanPath, nil
}

// loadStorageCredentials loads S3 credentials from the mounted directory.
func loadStorageCredentials(credsPath string) (*storage.Credentials, error) {
	creds := &storage.Credentials{}

	// Validate and clean the base credentials path
	cleanCredsPath, err := validatePath("/", credsPath)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials path: %w", err)
	}

	// Check if credentials directory exists
	if _, err := os.Stat(cleanCredsPath); os.IsNotExist(err) {
		// No credentials provided - will use default credential chain
		return nil, nil
	}

	// Load access key ID
	accessKeyPath := filepath.Join(cleanCredsPath, "accessKeyId")
	validatedAccessKeyPath, err := validatePath(cleanCredsPath, accessKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid access key path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedAccessKeyPath); err == nil {
		creds.AccessKeyID = strings.TrimSpace(string(data))
	}

	// Load secret access key
	secretKeyPath := filepath.Join(cleanCredsPath, "secretAccessKey")
	validatedSecretKeyPath, err := validatePath(cleanCredsPath, secretKeyPath)
	if err != nil {
		return nil, fmt.Errorf("invalid secret key path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedSecretKeyPath); err == nil {
		creds.SecretAccessKey = strings.TrimSpace(string(data))
	}

	// Validate that if one is provided, both must be provided
	if (creds.AccessKeyID != "" && creds.SecretAccessKey == "") ||
		(creds.AccessKeyID == "" && creds.SecretAccessKey != "") {
		return nil, fmt.Errorf("both accessKeyId and secretAccessKey must be provided if using static credentials")
	}

	// Load optional fields
	sessionTokenPath := filepath.Join(cleanCredsPath, "sessionToken")
	validatedSessionTokenPath, err := validatePath(cleanCredsPath, sessionTokenPath)
	if err != nil {
		return nil, fmt.Errorf("invalid session token path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedSessionTokenPath); err == nil {
		creds.SessionToken = strings.TrimSpace(string(data))
	}

	regionPath := filepath.Join(cleanCredsPath, "region")
	validatedRegionPath, err := validatePath(cleanCredsPath, regionPath)
	if err != nil {
		return nil, fmt.Errorf("invalid region path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedRegionPath); err == nil {
		creds.Region = strings.TrimSpace(string(data))
	} else {
		// Use default region if not provided
		creds.Region = constants.DefaultS3Region
	}

	caCertPath := filepath.Join(cleanCredsPath, "caCert")
	validatedCACertPath, err := validatePath(cleanCredsPath, caCertPath)
	if err != nil {
		return nil, fmt.Errorf("invalid CA cert path: %w", err)
	}
	// #nosec G304 -- Path is validated to be within base directory
	if data, err := os.ReadFile(validatedCACertPath); err == nil {
		creds.CACert = data
	}

	// If no credentials were loaded, return nil to use default credential chain
	if creds.AccessKeyID == "" && creds.SecretAccessKey == "" {
		return nil, nil
	}

	return creds, nil
}
