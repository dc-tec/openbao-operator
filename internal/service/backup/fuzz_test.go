package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/port/blobstore"
)

func FuzzGenerateAndParseBackupKey(f *testing.F) {
	f.Add("backups", "default", "cluster-a", "pre-upgrade", int64(1_700_000_000))
	f.Add("", "ns", "cluster", "", int64(1_701_000_000))

	f.Fuzz(func(t *testing.T, pathPrefix, namespace, cluster, filenamePrefix string, unixTs int64) {
		ts := time.Unix(unixTs, 0).UTC()
		namespace = sanitizeBackupSegment(namespace, "default")
		cluster = sanitizeBackupSegment(cluster, "cluster")
		filenamePrefix = sanitizeBackupSegment(filenamePrefix, "")
		pathPrefix = sanitizeBackupPrefix(pathPrefix)

		key, err := GenerateBackupKey(pathPrefix, namespace, cluster, filenamePrefix, ts)
		if err != nil {
			t.Fatalf("GenerateBackupKey() error = %v", err)
		}

		gotNamespace, gotCluster, gotTimestamp, gotUUID, err := ParseBackupKey(key)
		if err != nil {
			t.Fatalf("ParseBackupKey() error = %v", err)
		}
		if gotNamespace != namespace {
			t.Fatalf("namespace mismatch: got %q want %q", gotNamespace, namespace)
		}
		if gotCluster != cluster {
			t.Fatalf("cluster mismatch: got %q want %q", gotCluster, cluster)
		}
		if !gotTimestamp.Equal(ts) {
			t.Fatalf("timestamp mismatch: got %v want %v", gotTimestamp, ts)
		}
		if len(gotUUID) != ShortUUIDLength {
			t.Fatalf("unexpected UUID length %d", len(gotUUID))
		}
	})
}

func FuzzParseRetentionMaxAge(f *testing.F) {
	f.Add("")
	f.Add("168h")
	f.Add("bad")
	f.Add("-1h")

	f.Fuzz(func(t *testing.T, raw string) {
		d, err := ParseRetentionMaxAge(strings.TrimSpace(raw))
		if strings.TrimSpace(raw) == "" {
			if err != nil || d != 0 {
				t.Fatalf("empty maxAge should return zero duration without error")
			}
			return
		}
		if err == nil && d <= 0 {
			t.Fatalf("successful ParseRetentionMaxAge() must return positive duration")
		}
	})
}

func FuzzApplyRetention(f *testing.F) {
	f.Add(int32(1), int64(24), false, "default/cluster/", "valid")
	f.Add(int32(0), int64(0), true, "prefix/", "invalid")

	f.Fuzz(func(t *testing.T, maxCount int32, maxAgeHours int64, deleteFails bool, prefix, keyKind string) {
		prefix = sanitizeBackupPrefix(prefix)
		if prefix == "" {
			prefix = "default/cluster"
		}

		now := time.Unix(1_700_000_000, 0).UTC()
		objects := make([]blobstore.ObjectInfo, 0, 4)
		for i := 0; i < 4; i++ {
			key := sanitizeBackupSegment(keyKind, "backup")
			if key == "valid" || key == "backup" {
				generated, err := GenerateBackupKey(prefix, "default", "cluster", "", now.Add(-time.Duration(i)*time.Hour))
				if err != nil {
					t.Fatalf("GenerateBackupKey() error = %v", err)
				}
				key = generated
			}
			objects = append(objects, blobstore.ObjectInfo{
				Key:          key,
				LastModified: now.Add(-time.Duration(i) * time.Hour),
			})
		}

		store := &fuzzBlobStore{
			objects:        objects,
			deleteBatchErr: deleteFails,
		}

		policy := RetentionPolicy{
			MaxCount: clampBackupCount(maxCount),
		}
		if maxAgeHours > 0 {
			policy.MaxAge = time.Duration(maxAgeHours%240) * time.Hour
		}

		result, err := ApplyRetention(context.Background(), logr.Discard(), store, prefix, policy)
		if policy.MaxCount == 0 && policy.MaxAge == 0 {
			if err != nil {
				t.Fatalf("ApplyRetention() unexpected error: %v", err)
			}
			return
		}
		if err != nil {
			t.Fatalf("ApplyRetention() error = %v", err)
		}
		if result.TotalBackups < 0 || result.DeletedByAge < 0 || result.DeletedByCount < 0 {
			t.Fatalf("retention result contains negative counts")
		}
		if result.TotalBackups != len(objects) {
			t.Fatalf("unexpected total backups %d", result.TotalBackups)
		}
	})
}

func FuzzBuildJob(f *testing.F) {
	f.Add("test-cluster", "default", "backup-job", "scheduled", "verified:image@sha256:abc", "kubernetes", true, false, false)
	f.Add("cluster-b", "tenant", "pre-upgrade-job", "pre-upgrade", "", "openshift", false, true, true)

	f.Fuzz(func(t *testing.T, name, namespace, jobName, jobTypeRaw, digest, platform string, useBackup, appArmorEnabled, roleARN bool) {
		t.Setenv(constants.EnvOperatorVersion, "1.0.0")

		cluster := newTestClusterWithBackup(sanitizeBackupSegment(name, "cluster"), sanitizeBackupSegment(namespace, "default"))
		if !useBackup {
			cluster.Spec.Backup = nil
		}
		if cluster.Spec.Backup != nil {
			cluster.Spec.Backup.Image = sanitizeBackupText(digest, "openbao-backup:0.1.0")
			if roleARN {
				cluster.Spec.Backup.Target.RoleARN = "arn:aws:iam::123456789012:role/test"
			}
		}
		cluster.Spec.WorkloadHardening = &openbaov1alpha1.WorkloadHardeningConfig{
			AppArmorEnabled: appArmorEnabled,
		}

		jobType := JobTypeScheduled
		if strings.Contains(strings.ToLower(jobTypeRaw), "pre") {
			jobType = JobTypePreUpgrade
		}

		job, err := BuildJob(cluster, JobOptions{
			JobName:                sanitizeBackupSegment(jobName, "backup-job"),
			JobType:                jobType,
			VerifiedExecutorDigest: sanitizeBackupText(digest, ""),
			Platform:               sanitizeBackupText(platform, ""),
		})
		if !useBackup {
			if err == nil {
				t.Fatalf("expected BuildJob() to fail without backup config")
			}
			return
		}
		if err != nil {
			t.Fatalf("BuildJob() error = %v", err)
		}
		if job == nil || len(job.Spec.Template.Spec.Containers) != 1 {
			t.Fatalf("expected a single-container backup job")
		}
	})
}

type fuzzBlobStore struct {
	objects        []blobstore.ObjectInfo
	deleteBatchErr bool
}

func (f *fuzzBlobStore) Upload(context.Context, string, io.Reader) error { return nil }
func (f *fuzzBlobStore) Download(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (f *fuzzBlobStore) Delete(context.Context, string) error { return nil }
func (f *fuzzBlobStore) DeleteBatch(context.Context, []string) error {
	if f.deleteBatchErr {
		return errors.New("delete batch failed")
	}
	return nil
}
func (f *fuzzBlobStore) List(context.Context, string) ([]blobstore.ObjectInfo, error) {
	return append([]blobstore.ObjectInfo(nil), f.objects...), nil
}
func (f *fuzzBlobStore) Head(context.Context, string) (*blobstore.ObjectInfo, error) { return nil, nil }
func (f *fuzzBlobStore) Close() error                                                { return nil }

func sanitizeBackupSegment(input, fallback string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(input) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func sanitizeBackupPrefix(input string) string {
	parts := strings.Split(input, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		segment := sanitizeBackupSegment(part, "")
		if segment != "" {
			clean = append(clean, segment)
		}
		if len(clean) >= 3 {
			break
		}
	}
	return strings.Join(clean, "/")
}

func sanitizeBackupText(input, fallback string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(input, "\x00", ""))
	if trimmed == "" {
		return fallback
	}
	if len(trimmed) > 128 {
		return trimmed[:128]
	}
	return trimmed
}

func clampBackupCount(value int32) int32 {
	if value < 0 {
		return 0
	}
	return value % 6
}
