package backup

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateBackupKey(t *testing.T) {
	tests := []struct {
		name       string
		pathPrefix string
		namespace  string
		cluster    string
		timestamp  time.Time
		wantPrefix string
		wantSuffix string
	}{
		{
			name:       "with path prefix",
			pathPrefix: "prod/",
			namespace:  "security",
			cluster:    "prod-cluster",
			timestamp:  time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
			wantPrefix: "prod/security/prod-cluster/2025-01-15T03-00-00Z-",
			wantSuffix: ".snap",
		},
		{
			name:       "without path prefix",
			pathPrefix: "",
			namespace:  "default",
			cluster:    "test",
			timestamp:  time.Date(2025, 6, 20, 14, 30, 45, 0, time.UTC),
			wantPrefix: "default/test/2025-06-20T14-30-45Z-",
			wantSuffix: ".snap",
		},
		{
			name:       "path prefix with trailing slash",
			pathPrefix: "backups/",
			namespace:  "vault",
			cluster:    "main",
			timestamp:  time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			wantPrefix: "backups/vault/main/2025-12-31T23-59-59Z-",
			wantSuffix: ".snap",
		},
		{
			name:       "path prefix without trailing slash",
			pathPrefix: "backups",
			namespace:  "vault",
			cluster:    "main",
			timestamp:  time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			wantPrefix: "backups/vault/main/2025-12-31T23-59-59Z-",
			wantSuffix: ".snap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := GenerateBackupKey(tt.pathPrefix, tt.namespace, tt.cluster, tt.timestamp)
			if err != nil {
				t.Fatalf("GenerateBackupKey() error = %v", err)
			}

			if !strings.HasPrefix(key, tt.wantPrefix) {
				t.Errorf("GenerateBackupKey() = %v, want prefix %v", key, tt.wantPrefix)
			}

			if !strings.HasSuffix(key, tt.wantSuffix) {
				t.Errorf("GenerateBackupKey() = %v, want suffix %v", key, tt.wantSuffix)
			}

			// Check that the UUID portion is 8 hex characters
			uuidPart := key[len(tt.wantPrefix) : len(key)-len(tt.wantSuffix)]
			if len(uuidPart) != ShortUUIDLength {
				t.Errorf("UUID portion length = %d, want %d", len(uuidPart), ShortUUIDLength)
			}
		})
	}
}

func TestGenerateBackupKey_UniquenessWithSameTimestamp(t *testing.T) {
	timestamp := time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC)
	keys := make(map[string]bool)

	// Generate 100 keys with the same timestamp - they should all be unique
	for i := 0; i < 100; i++ {
		key, err := GenerateBackupKey("test", "ns", "cluster", timestamp)
		if err != nil {
			t.Fatalf("GenerateBackupKey() error = %v", err)
		}

		if keys[key] {
			t.Errorf("GenerateBackupKey() generated duplicate key: %s", key)
		}
		keys[key] = true
	}
}

func TestParseBackupKey(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		wantNamespace string
		wantCluster   string
		wantTimestamp time.Time
		wantUUID      string
		wantErr       bool
	}{
		{
			name:          "valid key with prefix",
			key:           "prod/security/prod-cluster/2025-01-15T03-00-00Z-a1b2c3d4.snap",
			wantNamespace: "security",
			wantCluster:   "prod-cluster",
			wantTimestamp: time.Date(2025, 1, 15, 3, 0, 0, 0, time.UTC),
			wantUUID:      "a1b2c3d4",
			wantErr:       false,
		},
		{
			name:          "valid key without prefix",
			key:           "default/test/2025-06-20T14-30-45Z-12345678.snap",
			wantNamespace: "default",
			wantCluster:   "test",
			wantTimestamp: time.Date(2025, 6, 20, 14, 30, 45, 0, time.UTC),
			wantUUID:      "12345678",
			wantErr:       false,
		},
		{
			name:    "invalid - too few parts",
			key:     "invalid/path.snap",
			wantErr: true,
		},
		{
			name:    "invalid - wrong extension",
			key:     "ns/cluster/2025-01-15T03-00-00Z-a1b2c3d4.tar",
			wantErr: true,
		},
		{
			name:    "invalid - bad timestamp",
			key:     "ns/cluster/invalid-timestamp-a1b2c3d4.snap",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace, cluster, timestamp, uuid, err := ParseBackupKey(tt.key)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBackupKey() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if namespace != tt.wantNamespace {
				t.Errorf("ParseBackupKey() namespace = %v, want %v", namespace, tt.wantNamespace)
			}
			if cluster != tt.wantCluster {
				t.Errorf("ParseBackupKey() cluster = %v, want %v", cluster, tt.wantCluster)
			}
			if !timestamp.Equal(tt.wantTimestamp) {
				t.Errorf("ParseBackupKey() timestamp = %v, want %v", timestamp, tt.wantTimestamp)
			}
			if uuid != tt.wantUUID {
				t.Errorf("ParseBackupKey() uuid = %v, want %v", uuid, tt.wantUUID)
			}
		})
	}
}

func TestGetBackupListPrefix(t *testing.T) {
	tests := []struct {
		name       string
		pathPrefix string
		namespace  string
		cluster    string
		want       string
	}{
		{
			name:       "with path prefix",
			pathPrefix: "prod/",
			namespace:  "security",
			cluster:    "prod-cluster",
			want:       "prod/security/prod-cluster/",
		},
		{
			name:       "without path prefix",
			pathPrefix: "",
			namespace:  "default",
			cluster:    "test",
			want:       "default/test/",
		},
		{
			name:       "path prefix without trailing slash",
			pathPrefix: "backups",
			namespace:  "vault",
			cluster:    "main",
			want:       "backups/vault/main/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetBackupListPrefix(tt.pathPrefix, tt.namespace, tt.cluster)
			if got != tt.want {
				t.Errorf("GetBackupListPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}
