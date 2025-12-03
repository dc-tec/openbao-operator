package backup

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"
)

const (
	// BackupExtension is the file extension for backup snapshots.
	BackupExtension = ".snap"
	// ShortUUIDLength is the length of the random UUID suffix in hex characters.
	ShortUUIDLength = 8
)

// GenerateBackupKey generates a predictable, sortable object key for a backup.
// Format: <pathPrefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap
//
// The timestamp is RFC3339 in UTC with colons replaced by dashes for filesystem compatibility.
// The UUID is 8 hex characters from crypto/rand to prevent collisions.
func GenerateBackupKey(pathPrefix, namespace, cluster string, timestamp time.Time) (string, error) {
	// Generate short UUID
	uuid, err := generateShortUUID()
	if err != nil {
		return "", fmt.Errorf("failed to generate UUID for backup key: %w", err)
	}

	// Format timestamp: RFC3339 UTC with colons replaced by dashes
	// Example: 2025-01-15T03-00-00Z
	ts := timestamp.UTC().Format(time.RFC3339)
	ts = strings.ReplaceAll(ts, ":", "-")

	// Construct filename
	filename := fmt.Sprintf("%s-%s%s", ts, uuid, BackupExtension)

	// Build full path, handling optional pathPrefix
	var keyPath string
	if pathPrefix != "" {
		// Ensure pathPrefix doesn't have leading/trailing slashes for clean joining
		pathPrefix = strings.Trim(pathPrefix, "/")
		keyPath = path.Join(pathPrefix, namespace, cluster, filename)
	} else {
		keyPath = path.Join(namespace, cluster, filename)
	}

	return keyPath, nil
}

// ParseBackupKey extracts metadata from a backup object key.
// Returns the namespace, cluster name, timestamp, and UUID from a backup key.
func ParseBackupKey(key string) (namespace, cluster string, timestamp time.Time, uuid string, err error) {
	// Split the path
	parts := strings.Split(key, "/")
	if len(parts) < 3 {
		return "", "", time.Time{}, "", fmt.Errorf("invalid backup key format: %s", key)
	}

	// Get the last three parts: namespace/cluster/filename
	filename := parts[len(parts)-1]
	cluster = parts[len(parts)-2]
	namespace = parts[len(parts)-3]

	// Parse filename: <timestamp>-<uuid>.snap
	if !strings.HasSuffix(filename, BackupExtension) {
		return "", "", time.Time{}, "", fmt.Errorf("invalid backup filename extension: %s", filename)
	}

	// Remove extension
	base := strings.TrimSuffix(filename, BackupExtension)

	// Split by the last dash to get UUID
	lastDash := strings.LastIndex(base, "-")
	if lastDash == -1 || lastDash+1+ShortUUIDLength != len(base) {
		return "", "", time.Time{}, "", fmt.Errorf("invalid backup filename format: %s", filename)
	}

	uuid = base[lastDash+1:]
	tsStr := base[:lastDash]

	// Convert dashes back to colons in the time portion
	// The timestamp format is: 2025-01-15T03-00-00Z
	// We need to restore: 2025-01-15T03:00:00Z
	// Only convert dashes after 'T' (in the time portion)
	tIdx := strings.Index(tsStr, "T")
	if tIdx == -1 {
		return "", "", time.Time{}, "", fmt.Errorf("invalid timestamp format in backup key: %s", tsStr)
	}
	datePart := tsStr[:tIdx]
	timePart := tsStr[tIdx:]
	timePart = strings.ReplaceAll(timePart, "-", ":")
	tsStr = datePart + timePart

	timestamp, err = time.Parse(time.RFC3339, tsStr)
	if err != nil {
		return "", "", time.Time{}, "", fmt.Errorf("failed to parse timestamp from backup key: %w", err)
	}

	return namespace, cluster, timestamp, uuid, nil
}

// GetBackupListPrefix returns the object prefix for listing backups of a specific cluster.
func GetBackupListPrefix(pathPrefix, namespace, cluster string) string {
	if pathPrefix != "" {
		pathPrefix = strings.Trim(pathPrefix, "/")
		return path.Join(pathPrefix, namespace, cluster) + "/"
	}
	return path.Join(namespace, cluster) + "/"
}

// generateShortUUID generates a short random UUID using crypto/rand.
func generateShortUUID() (string, error) {
	bytes := make([]byte, ShortUUIDLength/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
