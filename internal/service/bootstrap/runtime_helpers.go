package bootstrap

import (
	"path"
	"strings"
)

const sealCredsVolumeMountPath = "/etc/bao/seal-creds" // #nosec G101 -- False positive: path, not a secret

func mountedSealCredentialsKey(filePath string) (string, bool) {
	cleanPath := path.Clean(strings.TrimSpace(filePath))
	if cleanPath == "." || cleanPath == "" {
		return "", false
	}

	cleanMount := path.Clean(sealCredsVolumeMountPath)
	if cleanPath == cleanMount || !strings.HasPrefix(cleanPath, cleanMount+"/") {
		return "", false
	}

	rel := strings.TrimPrefix(cleanPath, cleanMount+"/")
	if rel == "" || strings.Contains(rel, "/") {
		return "", false
	}

	return rel, true
}
