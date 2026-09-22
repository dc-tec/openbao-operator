package backup

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

func TestParseRetentionMaxAge(t *testing.T) {
	tests := []struct {
		name    string
		maxAge  string
		want    time.Duration
		wantErr bool
	}{
		{
			name:    "empty string",
			maxAge:  "",
			want:    0,
			wantErr: false,
		},
		{
			name:    "7 days",
			maxAge:  "168h",
			want:    168 * time.Hour,
			wantErr: false,
		},
		{
			name:    "30 days",
			maxAge:  "720h",
			want:    720 * time.Hour,
			wantErr: false,
		},
		{
			name:    "1 hour",
			maxAge:  "1h",
			want:    1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "complex duration",
			maxAge:  "24h30m",
			want:    24*time.Hour + 30*time.Minute,
			wantErr: false,
		},
		{
			name:    "invalid format",
			maxAge:  "7 days",
			wantErr: true,
		},
		{
			name:    "negative duration",
			maxAge:  "-24h",
			wantErr: true,
		},
		{
			name:    "zero duration",
			maxAge:  "0s",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRetentionMaxAge(tt.maxAge)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRetentionMaxAge() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if got != tt.want {
				t.Errorf("ParseRetentionMaxAge() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetentionPolicy_NoPolicy(t *testing.T) {
	policy := RetentionPolicy{
		MaxCount: 0,
		MaxAge:   0,
	}

	// With no policy, nothing should be marked for deletion
	if policy.MaxCount != 0 || policy.MaxAge != 0 {
		t.Error("Expected empty policy to have zero values")
	}
}

func TestRetentionPolicy_MaxCount(t *testing.T) {
	policy := RetentionPolicy{
		MaxCount: 5,
		MaxAge:   0,
	}

	if policy.MaxCount != 5 {
		t.Errorf("Expected MaxCount = 5, got %d", policy.MaxCount)
	}
	if policy.MaxAge != 0 {
		t.Errorf("Expected MaxAge = 0, got %v", policy.MaxAge)
	}
}

func TestRetentionPolicy_MaxAge(t *testing.T) {
	policy := RetentionPolicy{
		MaxCount: 0,
		MaxAge:   168 * time.Hour, // 7 days
	}

	if policy.MaxAge != 168*time.Hour {
		t.Errorf("Expected MaxAge = 168h, got %v", policy.MaxAge)
	}
	if policy.MaxCount != 0 {
		t.Errorf("Expected MaxCount = 0, got %d", policy.MaxCount)
	}
}

func TestRetentionPolicy_Combined(t *testing.T) {
	policy := RetentionPolicy{
		MaxCount: 10,
		MaxAge:   720 * time.Hour, // 30 days
	}

	if policy.MaxCount != 10 {
		t.Errorf("Expected MaxCount = 10, got %d", policy.MaxCount)
	}
	if policy.MaxAge != 720*time.Hour {
		t.Errorf("Expected MaxAge = 720h, got %v", policy.MaxAge)
	}
}

// TestRetentionResult verifies the result struct fields
func TestRetentionResult(t *testing.T) {
	result := RetentionResult{
		TotalBackups:   15,
		DeletedByCount: 5,
		DeletedByAge:   3,
		Errors:         nil,
	}

	if result.TotalBackups != 15 {
		t.Errorf("Expected TotalBackups = 15, got %d", result.TotalBackups)
	}

	totalDeleted := result.DeletedByCount + result.DeletedByAge
	if totalDeleted != 8 {
		t.Errorf("Expected total deleted = 8, got %d", totalDeleted)
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d", len(result.Errors))
	}
}

func TestApplyRetention_NeverDeletesNewestOrProtectedBackup(t *testing.T) {
	now := time.Now().UTC()
	key := func(age time.Duration, id string) string {
		generated, err := GenerateBackupKey("backups", "default", "cluster", "", now.Add(-age))
		if err != nil {
			t.Fatalf("GenerateBackupKey() error = %v", err)
		}
		return generated[:len(generated)-len("00000000.snap")] + id + ".snap"
	}
	newest := key(48*time.Hour, "cccccccc")
	protected := key(72*time.Hour, "bbbbbbbb")
	oldest := key(96*time.Hour, "aaaaaaaa")

	tests := []struct {
		name        string
		policy      RetentionPolicy
		wantDeleted []string
	}{
		{
			name:        "every backup older than max age keeps the newest",
			policy:      RetentionPolicy{MaxAge: 24 * time.Hour},
			wantDeleted: []string{protected, oldest},
		},
		{
			name:        "protected key survives age and count",
			policy:      RetentionPolicy{MaxCount: 1, MaxAge: 24 * time.Hour, ProtectedKey: protected},
			wantDeleted: []string{oldest},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeBlobStore{objects: []blobstore.ObjectInfo{
				{Key: oldest}, {Key: newest}, {Key: protected},
			}}
			if _, err := ApplyRetention(context.Background(), logr.Discard(), store, "backups/default/cluster/", tt.policy); err != nil {
				t.Fatalf("ApplyRetention() error = %v", err)
			}
			slices.Sort(store.deleted)
			want := slices.Clone(tt.wantDeleted)
			slices.Sort(want)
			if !slices.Equal(store.deleted, want) {
				t.Fatalf("deleted = %v, want %v", store.deleted, want)
			}
		})
	}
}

func TestApplyRetention_OnlyManagesScheduledBackups(t *testing.T) {
	prefix := GetBackupListPrefix("backups", "default", "cluster")
	scheduled := func(ts, id string) string { return prefix + ts + "-" + id + ".snap" }
	oldScheduled := scheduled("2025-01-01T03-00-00Z", "aaaaaaaa")
	newScheduled := scheduled("2025-01-03T03-00-00Z", "cccccccc")
	untouched := []string{
		prefix + "pre-upgrade-2024-12-01T03-00-00Z-dddddddd.snap",
		prefix + "post-promotion-2024-12-01T03-00-00Z-eeeeeeee.snap",
		prefix + "nested/2024-12-01T03-00-00Z-ffffffff.snap",
		prefix + "operator-notes.txt",
	}

	objects := make([]blobstore.ObjectInfo, 0, 2+len(untouched))
	objects = append(objects, blobstore.ObjectInfo{Key: oldScheduled}, blobstore.ObjectInfo{Key: newScheduled})
	for _, key := range untouched {
		objects = append(objects, blobstore.ObjectInfo{Key: key, LastModified: time.Unix(0, 0)})
	}
	store := &fakeBlobStore{objects: objects}

	result, err := ApplyRetention(context.Background(), logr.Discard(), store, prefix, RetentionPolicy{MaxCount: 1, MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}
	if !slices.Equal(store.deleted, []string{oldScheduled}) {
		t.Fatalf("deleted = %v, want only %s", store.deleted, oldScheduled)
	}
	if result.TotalBackups != 2 || result.SkippedObjects != len(untouched) {
		t.Fatalf("result = %+v, want 2 scheduled backups and %d skipped objects", result, len(untouched))
	}
}
