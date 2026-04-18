package infra

import (
	"path"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const sealCredsVolumeMountPath = "/etc/bao/seal-creds" // #nosec G101 -- False positive: path, not a secret

func usesACMEMode(cluster *openbaov1alpha1.OpenBaoCluster) bool {
	return cluster.Spec.TLS.Enabled && cluster.Spec.TLS.Mode == openbaov1alpha1.TLSModeACME
}

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
