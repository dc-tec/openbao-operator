//go:build e2e
// +build e2e

package helpers

import (
	gomega "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"

	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

// ExpectOpenBaoPodVersion verifies that a pod contains the OpenBao container and
// reports the expected semantic version via its controller-managed label. The
// image reference itself may be canonicalized to a digest by the workload path.
func ExpectOpenBaoPodVersion(g gomegatypes.Gomega, pod corev1.Pod, expectedVersion string) {
	g.Expect(expectedVersion).NotTo(gomega.BeEmpty(), "expected version is required")

	hasOpenBaoContainer := false
	for _, container := range pod.Spec.Containers {
		if container.Name != constants.ContainerBao {
			continue
		}
		hasOpenBaoContainer = true
		g.Expect(container.Image).NotTo(gomega.BeEmpty(), "pod %s should define an OpenBao image", pod.Name)
		break
	}

	g.Expect(hasOpenBaoContainer).To(gomega.BeTrue(), "pod %s should contain the %q container", pod.Name, constants.ContainerBao)
	g.Expect(pod.Labels).To(gomega.HaveKeyWithValue(portopenbao.LabelVersion, expectedVersion),
		"pod %s should report OpenBao version %s", pod.Name, expectedVersion)
}
