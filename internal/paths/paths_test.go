package paths

import "testing"

func TestBackupPathsAreNonEmpty(t *testing.T) {
	if TLSCACertPath == "" {
		t.Fatal("TLSCACertPath must not be empty")
	}
	if BackupTokenPath == "" {
		t.Fatal("BackupTokenPath must not be empty")
	}
	if BackupCredentialsPath == "" {
		t.Fatal("BackupCredentialsPath must not be empty")
	}
	if BackupJWTTokenPath == "" {
		t.Fatal("BackupJWTTokenPath must not be empty")
	}
}
