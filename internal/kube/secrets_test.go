package kube

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

	"github.com/dc-tec/openbao-operator/internal/storage"
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

func TestLoadStorageCredentials_NilSecretRef(t *testing.T) {
	ctx := context.Background()
	k8sClient := newTestClient(t)

	creds, err := LoadStorageCredentials(ctx, k8sClient, nil, "default")
	if err != nil {
		t.Fatalf("LoadStorageCredentials() with nil secretRef should not error, got: %v", err)
	}

	if creds != nil {
		t.Error("LoadStorageCredentials() with nil secretRef should return nil")
	}
}

func TestLoadStorageCredentials_ValidCredentials(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			storage.SecretKeyAccessKeyID:     []byte("test-access-key"),
			storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	creds, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "default")
	if err != nil {
		t.Fatalf("LoadStorageCredentials() error = %v", err)
	}

	if creds == nil {
		t.Fatal("LoadStorageCredentials() should return credentials")
	}

	if creds.AccessKeyID != "test-access-key" {
		t.Errorf("LoadStorageCredentials() AccessKeyID = %v, want test-access-key", creds.AccessKeyID)
	}

	if creds.SecretAccessKey != "test-secret-key" {
		t.Errorf("LoadStorageCredentials() SecretAccessKey = %v, want test-secret-key", creds.SecretAccessKey)
	}
}

func TestLoadStorageCredentials_WithOptionalFields(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			storage.SecretKeyAccessKeyID:     []byte("test-access-key"),
			storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
			storage.SecretKeySessionToken:    []byte("test-session-token"),
			storage.SecretKeyRegion:          []byte("us-west-2"),
			storage.SecretKeyCACert:          []byte("test-ca-cert"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	creds, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "default")
	if err != nil {
		t.Fatalf("LoadStorageCredentials() error = %v", err)
	}

	if creds.SessionToken != "test-session-token" {
		t.Errorf("LoadStorageCredentials() SessionToken = %v, want test-session-token", creds.SessionToken)
	}

	if creds.Region != "us-west-2" {
		t.Errorf("LoadStorageCredentials() Region = %v, want us-west-2", creds.Region)
	}

	if string(creds.CACert) != "test-ca-cert" {
		t.Errorf("LoadStorageCredentials() CACert = %v, want test-ca-cert", string(creds.CACert))
	}
}

func TestLoadStorageCredentials_NamespaceRequired(t *testing.T) {
	// This test verifies that the namespace parameter is required and used correctly.
	// The Secret must be in the specified namespace - cross-namespace access is not allowed.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "other-ns",
		},
		Data: map[string][]byte{
			storage.SecretKeyAccessKeyID:     []byte("test-access-key"),
			storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	// Looking in "default" namespace should fail because secret is in "other-ns"
	_, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Fatal("LoadStorageCredentials() should fail when secret is in different namespace")
	}

	// Looking in "other-ns" namespace should succeed
	creds, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "other-ns")
	if err != nil {
		t.Fatalf("LoadStorageCredentials() error = %v", err)
	}
	if creds == nil {
		t.Fatal("LoadStorageCredentials() should return credentials")
	}
}

func TestLoadStorageCredentials_MissingSecret(t *testing.T) {
	ctx := context.Background()
	k8sClient := newTestClient(t)
	secretRef := &corev1.LocalObjectReference{
		Name: "missing-secret",
	}

	creds, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Error("LoadStorageCredentials() with missing secret should return error")
	}

	if creds != nil {
		t.Error("LoadStorageCredentials() with missing secret should return nil credentials")
	}
}

func TestLoadStorageCredentials_MissingAccessKeyID(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
			// Missing AccessKeyID
		},
	}

	ctx := context.Background()
	k8sClient := newTestClient(t, secret)
	secretRef := &corev1.LocalObjectReference{
		Name: "test-credentials",
	}

	// When only one key is present, LoadStorageCredentials should return an error
	// Both keys must be present or both must be missing (for workload identity)
	_, err := LoadStorageCredentials(ctx, k8sClient, secretRef, "default")
	if err == nil {
		t.Fatal("LoadStorageCredentials() with missing AccessKeyID should error when only SecretAccessKey is present")
	}

	expectedErr := "must contain both accessKeyId and secretAccessKey, or neither"
	if err.Error() == "" {
		t.Errorf("LoadStorageCredentials() error message is empty")
	}
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("LoadStorageCredentials() error = %v, want error containing %q", err, expectedErr)
	}
}

func TestLoadStorageCredentials_PartialCredentialsError(t *testing.T) {
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
					storage.SecretKeyAccessKeyID: []byte("test-access-key"),
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
					storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
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
					storage.SecretKeyAccessKeyID:     []byte("test-access-key"),
					storage.SecretKeySecretAccessKey: []byte("test-secret-key"),
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

			creds, err := LoadStorageCredentials(ctx, client, secretRef, "default")

			if (err != nil) != tt.wantErr {
				t.Errorf("LoadStorageCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if creds == nil && (tt.secret.Data[storage.SecretKeyAccessKeyID] != nil || tt.secret.Data[storage.SecretKeySecretAccessKey] != nil) {
				t.Error("LoadStorageCredentials() should return credentials when keys are present")
			}
		})
	}
}
