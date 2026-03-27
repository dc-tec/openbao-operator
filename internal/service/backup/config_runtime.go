package backup

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

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

// loadClientConfig loads smart client configuration from environment variables.
func loadClientConfig(cfg *ExecutorConfig) error {
	qpsStr := strings.TrimSpace(os.Getenv(constants.EnvClientQPS))
	if qpsStr != "" {
		qps, err := strconv.ParseFloat(qpsStr, 64)
		if err != nil {
			return fmt.Errorf("invalid %s value %q: %w", constants.EnvClientQPS, qpsStr, err)
		}
		cfg.RateLimitQPS = qps
	}

	burstStr := strings.TrimSpace(os.Getenv(constants.EnvClientBurst))
	if burstStr != "" {
		burst, err := strconv.ParseInt(burstStr, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid %s value %q: %w", constants.EnvClientBurst, burstStr, err)
		}
		cfg.RateLimitBurst = int(burst)
	}

	failureThresholdStr := strings.TrimSpace(os.Getenv(constants.EnvClientCircuitBreakerFailureThreshold))
	if failureThresholdStr != "" {
		failureThreshold, err := strconv.ParseInt(failureThresholdStr, 10, 32)
		if err != nil {
			return fmt.Errorf("invalid %s value %q: %w", constants.EnvClientCircuitBreakerFailureThreshold, failureThresholdStr, err)
		}
		cfg.CircuitBreakerFailureThreshold = int(failureThreshold)
	}

	cfg.CircuitBreakerOpenDuration = strings.TrimSpace(os.Getenv(constants.EnvClientCircuitBreakerOpenDuration))
	return nil
}
