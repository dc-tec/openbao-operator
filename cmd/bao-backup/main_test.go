package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStorageCredentials(t *testing.T) {
	tests := []struct {
		name          string
		setupFiles    map[string]string
		wantErr       bool
		wantAccessKey string
		wantSecretKey string
		wantRegion    string
	}{
		{
			name: "valid credentials",
			setupFiles: map[string]string{
				"accessKeyId":     "AKIAIOSFODNN7EXAMPLE",
				"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"region":          "us-west-2",
			},
			wantErr:       false,
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
			wantSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			wantRegion:    "us-west-2",
		},
		{
			name: "credentials with default region",
			setupFiles: map[string]string{
				"accessKeyId":     "AKIAIOSFODNN7EXAMPLE",
				"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			wantErr:       false,
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
			wantSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			wantRegion:    defaultRegion,
		},
		{
			name:       "missing directory returns nil",
			setupFiles: map[string]string{},
			wantErr:    false,
		},
		{
			name: "only access key (invalid)",
			setupFiles: map[string]string{
				"accessKeyId": "AKIAIOSFODNN7EXAMPLE",
			},
			wantErr: true,
		},
		{
			name: "only secret key (invalid)",
			setupFiles: map[string]string{
				"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			wantErr: true,
		},
		{
			name: "credentials with session token",
			setupFiles: map[string]string{
				"accessKeyId":     "AKIAIOSFODNN7EXAMPLE",
				"secretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken":    "temporary-token",
				"region":          "us-east-1",
			},
			wantErr:       false,
			wantAccessKey: "AKIAIOSFODNN7EXAMPLE",
			wantSecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			wantRegion:    "us-east-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// Create files
			for filename, content := range tt.setupFiles {
				filePath := filepath.Join(dir, filename)
				if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			// If no files were created, test the missing directory case
			if len(tt.setupFiles) == 0 {
				// Use a non-existent directory
				dir = filepath.Join(t.TempDir(), "nonexistent")
			}

			creds, err := loadStorageCredentials(dir)

			if (err != nil) != tt.wantErr {
				t.Errorf("loadStorageCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			// If we expect nil (missing directory), verify it
			if tt.wantAccessKey == "" && tt.wantSecretKey == "" {
				if creds != nil {
					t.Errorf("loadStorageCredentials() = %v, want nil", creds)
				}
				return
			}

			if creds == nil {
				t.Fatal("loadStorageCredentials() returned nil, expected credentials")
			}

			if creds.AccessKeyID != tt.wantAccessKey {
				t.Errorf("loadStorageCredentials().AccessKeyID = %q, want %q", creds.AccessKeyID, tt.wantAccessKey)
			}
			if creds.SecretAccessKey != tt.wantSecretKey {
				t.Errorf("loadStorageCredentials().SecretAccessKey = %q, want %q", creds.SecretAccessKey, tt.wantSecretKey)
			}
			if creds.Region != tt.wantRegion {
				t.Errorf("loadStorageCredentials().Region = %q, want %q", creds.Region, tt.wantRegion)
			}
		})
	}
}
