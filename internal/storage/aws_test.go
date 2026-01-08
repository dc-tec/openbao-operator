package storage

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gocloud.dev/blob/memblob"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return scheme
}()

func newTestClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	builder := fake.NewClientBuilder().WithScheme(testScheme)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return builder.Build()
}

func TestLoadCredentials_NilSecretRef(t *testing.T) {
	ctx := context.Background()
	k8sClient := newTestClient(t)

	creds, err := LoadCredentials(ctx, k8sClient, nil, "default")
	if err != nil {
		t.Fatalf("LoadCredentials() with nil secretRef should not error, got: %v", err)
	}

	if creds != nil {
		t.Error("LoadCredentials() with nil secretRef should return nil")
	}
}

func TestLoadCredentials_ValidCredentials(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			SecretKeyAccessKeyID:     []byte("test-access-key"),
			SecretKeySecretAccessKey: []byte("test-secret-key"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	creds, err := LoadCredentials(ctx, k8sClient, secretRef, "default")
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	if creds == nil {
		t.Fatal("LoadCredentials() should return credentials")
	}

	if creds.AccessKeyID != "test-access-key" {
		t.Errorf("LoadCredentials() AccessKeyID = %v, want test-access-key", creds.AccessKeyID)
	}

	if creds.SecretAccessKey != "test-secret-key" {
		t.Errorf("LoadCredentials() SecretAccessKey = %v, want test-secret-key", creds.SecretAccessKey)
	}
}

func TestLoadCredentials_WithOptionalFields(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			SecretKeyAccessKeyID:     []byte("test-access-key"),
			SecretKeySecretAccessKey: []byte("test-secret-key"),
			SecretKeySessionToken:    []byte("test-session-token"),
			SecretKeyRegion:          []byte("us-west-2"),
			SecretKeyCACert:          []byte("test-ca-cert"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	creds, err := LoadCredentials(ctx, k8sClient, secretRef, "default")
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}

	if creds.SessionToken != "test-session-token" {
		t.Errorf("LoadCredentials() SessionToken = %v, want test-session-token", creds.SessionToken)
	}

	if creds.Region != "us-west-2" {
		t.Errorf("LoadCredentials() Region = %v, want us-west-2", creds.Region)
	}

	if string(creds.CACert) != "test-ca-cert" {
		t.Errorf("LoadCredentials() CACert = %v, want test-ca-cert", string(creds.CACert))
	}
}

func TestLoadCredentials_NamespaceRequired(t *testing.T) {
	// This test verifies that the namespace parameter is required and used correctly.
	// The Secret must be in the specified namespace - cross-namespace access is not allowed.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "other-ns",
		},
		Data: map[string][]byte{
			SecretKeyAccessKeyID:     []byte("test-access-key"),
			SecretKeySecretAccessKey: []byte("test-secret-key"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	// Looking in "default" namespace should fail because secret is in "other-ns"
	_, err := LoadCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Fatal("LoadCredentials() should fail when secret is in different namespace")
	}

	// Looking in "other-ns" namespace should succeed
	creds, err := LoadCredentials(ctx, k8sClient, secretRef, "other-ns")
	if err != nil {
		t.Fatalf("LoadCredentials() error = %v", err)
	}
	if creds == nil {
		t.Fatal("LoadCredentials() should return credentials")
	}
}

func TestLoadCredentials_MissingSecret(t *testing.T) {
	ctx := context.Background()
	k8sClient := newTestClient(t)
	secretRef := &corev1.LocalObjectReference{
		Name: "missing-secret",
	}

	creds, err := LoadCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Error("LoadCredentials() with missing secret should return error")
	}

	if creds != nil {
		t.Error("LoadCredentials() with missing secret should return nil credentials")
	}
}

func TestLoadCredentials_MissingAccessKeyID(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			SecretKeySecretAccessKey: []byte("test-secret-key"),
			// Missing AccessKeyID
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	// When only one key is present, LoadCredentials should return an error
	// Both keys must be present or both must be missing (for workload identity)
	_, err := LoadCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Fatal("LoadCredentials() with missing AccessKeyID should error when only SecretAccessKey is present")
	}

	expectedErr := "must contain both accessKeyId and secretAccessKey, or neither"
	if err.Error() == "" {
		t.Errorf("LoadCredentials() error message is empty")
	}
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("LoadCredentials() error = %v, want error containing %q", err, expectedErr)
	}
}

func TestLoadCredentials_PartialCredentialsError(t *testing.T) {
	tests := []struct {
		name    string
		secret  *corev1.Secret
		wantErr bool
	}{
		{
			name: "only AccessKeyID",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					SecretKeyAccessKeyID: []byte("test-access-key"),
					// Missing SecretAccessKey
				},
			},
			wantErr: true,
		},
		{
			name: "only SecretAccessKey",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					SecretKeySecretAccessKey: []byte("test-secret-key"),
					// Missing AccessKeyID
				},
			},
			wantErr: true,
		},
		{
			name: "both keys present",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{
					SecretKeyAccessKeyID:     []byte("test-access-key"),
					SecretKeySecretAccessKey: []byte("test-secret-key"),
				},
			},
			wantErr: false,
		},
		{
			name: "neither key present",
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-credentials",
					Namespace: "default",
				},
				Data: map[string][]byte{},
			},
			wantErr: false, // This is valid for workload identity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			client := newTestClient(t, tt.secret)
			secretRef := &corev1.LocalObjectReference{
				Name: "test-credentials",
			}

			creds, err := LoadCredentials(ctx, client, secretRef, "default")

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if creds == nil && (tt.secret.Data[SecretKeyAccessKeyID] != nil || tt.secret.Data[SecretKeySecretAccessKey] != nil) {
				t.Error("LoadCredentials() should return credentials when keys are present")
			}
		})
	}
}

func TestOpenS3Bucket_WithCredentials(t *testing.T) {
	ctx := context.Background()

	bucket, err := OpenS3Bucket(ctx, S3ClientConfig{
		Endpoint:        "https://s3.amazonaws.com",
		Bucket:          "test-bucket",
		Region:          "us-west-2",
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		SessionToken:    "test-session-token",
	})
	if err != nil {
		t.Fatalf("OpenS3Bucket() error = %v", err)
	}
	defer func() {
		_ = bucket.Close()
	}()

	if bucket == nil {
		t.Fatal("OpenS3Bucket() should return bucket")
	}
}

func TestOpenS3Bucket_MissingRegion(t *testing.T) {
	ctx := context.Background()

	// Region is required, so this should error
	_, err := OpenS3Bucket(ctx, S3ClientConfig{
		Endpoint: "https://s3.amazonaws.com",
		Bucket:   "test-bucket",
		// Region is missing
	})
	if err == nil {
		t.Fatal("OpenS3Bucket() should error when region is missing")
	}

	expectedErr := "region is required"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("OpenS3Bucket() error = %v, want error containing %q", err, expectedErr)
	}
}

// ============================================================================
// Bucket Tests (using memblob for in-memory testing)
// ============================================================================

// TestBucket_Upload verifies that Upload correctly stores content.
func TestBucket_Upload(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	data := []byte("test backup data")
	reader := bytes.NewReader(data)

	if err := bucket.Upload(ctx, "test-key", reader); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	downloaded, err := bucket.Download(ctx, "test-key")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer func() {
		_ = downloaded.Close()
	}()

	content, err := io.ReadAll(downloaded)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}

	if !bytes.Equal(content, data) {
		t.Errorf("Content mismatch: got %q, want %q", content, data)
	}
}

// TestBucket_Delete verifies that Delete removes objects.
func TestBucket_Delete(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	data := []byte("test data")
	if err := bucket.Upload(ctx, "delete-test", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if err := bucket.Delete(ctx, "delete-test"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	info, err := bucket.Head(ctx, "delete-test")
	if err != nil {
		t.Fatalf("Head() after delete error = %v", err)
	}
	if info != nil {
		t.Error("Object should not exist after delete")
	}
}

// TestBucket_DeleteNonExistent verifies deleting non-existent objects doesn't error.
func TestBucket_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	if err := bucket.Delete(ctx, "non-existent-key"); err != nil {
		t.Errorf("Delete() of non-existent key should not error, got: %v", err)
	}
}

// TestBucket_List verifies that List returns objects with correct prefix.
func TestBucket_List(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	testData := []struct {
		key  string
		data []byte
	}{
		{"backups/cluster1/backup1", []byte("backup1")},
		{"backups/cluster1/backup2", []byte("backup2")},
		{"backups/cluster2/backup1", []byte("backup3")},
	}

	for _, td := range testData {
		if err := bucket.Upload(ctx, td.key, bytes.NewReader(td.data)); err != nil {
			t.Fatalf("Upload() for %s error = %v", td.key, err)
		}
	}

	objects, err := bucket.List(ctx, "backups/cluster1/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("List() returned %d objects, want 2", len(objects))
	}
}

// TestBucket_Head verifies that Head returns correct metadata.
func TestBucket_Head(t *testing.T) {
	ctx := context.Background()

	bucket := NewBucket(memblob.OpenBucket(nil))
	defer func() {
		_ = bucket.Close()
	}()

	info, err := bucket.Head(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if info != nil {
		t.Error("Head() on non-existent object should return nil")
	}

	data := []byte("test data with known size")
	if err := bucket.Upload(ctx, "head-test", bytes.NewReader(data)); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	info, err = bucket.Head(ctx, "head-test")
	if err != nil {
		t.Fatalf("Head() error = %v", err)
	}
	if info == nil {
		t.Fatal("Head() should return metadata for existing object")
	}
	if info.Size != int64(len(data)) {
		t.Errorf("Head() Size = %d, want %d", info.Size, len(data))
	}
}
