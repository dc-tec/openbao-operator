package workload

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	corev1 "k8s.io/api/core/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
)

const (
	openBaoLivenessProbeTimeout  = "4s"
	openBaoReadinessProbeTimeout = "10s"
	openBaoStartupProbeTimeout   = "5s"
)

var (
	// Use 127.0.0.1 instead of localhost to force IPv4, avoiding IPv6 resolution issues
	// where localhost might resolve to ::1 but OpenBao only listens on IPv4.
	openBaoProbeAddr = fmt.Sprintf("https://127.0.0.1:%d", constants.PortAPI)

	openBaoProbeCAFile = constants.PathTLSCACert
)

type probeExecActions struct {
	startup   *corev1.ExecAction
	liveness  *corev1.ExecAction
	readiness *corev1.ExecAction
}

func getInitContainerImage(cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	return ResolveInitContainerImage(cluster)
}

// getContainerImage returns the container image to use for the OpenBao container.
// If verifiedImageDigest is provided, it is used to prevent TOCTOU attacks.
// Otherwise, cluster.Spec.Image is used.
func getContainerImage(cluster *openbaov1alpha1.OpenBaoCluster, verifiedImageDigest string) string {
	if verifiedImageDigest != "" {
		return verifiedImageDigest
	}
	if cluster.Spec.Image != "" {
		return cluster.Spec.Image
	}
	// Intelligent Default based on Version
	return constants.GetOpenBaoImage(cluster.Spec.Version)
}

// getOpenBaoConfigPath returns the path to the OpenBao configuration file.
func getOpenBaoConfigPath(_ *openbaov1alpha1.OpenBaoCluster) string {
	return openBaoRenderedConfig
}

// computeConfigHash computes a SHA256 hash of the config content for change detection.
func computeConfigHash(configContent string) string {
	sum := sha256.Sum256([]byte(configContent))
	return hex.EncodeToString(sum[:])
}
