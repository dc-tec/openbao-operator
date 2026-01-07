package storage

import (
	"context"
	"strings"
	"testing"

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

func TestNewS3ClientFromCredentials_WithCredentials(t *testing.T) {
	ctx := context.Background()
	creds := &Credentials{
		AccessKeyID:     "test-access-key",
		SecretAccessKey: "test-secret-key",
		SessionToken:    "test-session-token",
		Region:          "us-west-2",
		// CACert is optional - omit it for this test to avoid PEM parsing issues
	}

	s3Client, err := NewS3ClientFromCredentials(ctx, "https://s3.amazonaws.com", "test-bucket", creds, false, 0, 0)
	if err != nil {
		t.Fatalf("NewS3ClientFromCredentials() error = %v", err)
	}

	if s3Client == nil {
		t.Fatal("NewS3ClientFromCredentials() should return client")
	}
}

func TestNewS3ClientFromCredentials_NilCredentials(t *testing.T) {
	ctx := context.Background()

	// When creds is nil, region is required but empty, so this should error
	// The S3 client requires a region even when using default credential chain
	_, err := NewS3ClientFromCredentials(ctx, "https://s3.amazonaws.com", "test-bucket", nil, false, 0, 0)
	if err == nil {
		t.Fatal("NewS3ClientFromCredentials() with nil creds should error because region is required")
	}

	expectedErr := "region is required"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("NewS3ClientFromCredentials() error = %v, want error containing %q", err, expectedErr)
	}
}
