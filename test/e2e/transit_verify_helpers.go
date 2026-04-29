//go:build e2e
// +build e2e

package e2e

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func newTransitEncryptVerifyPod(name, namespace, image, address, token, transitKey string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(100)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "bao",
					Image: image,
					Env: []corev1.EnvVar{
						{Name: "BAO_ADDR", Value: address},
						{Name: "BAO_TOKEN", Value: token},
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{
						fmt.Sprintf(
							"bao write -format=json transit/encrypt/%s plaintext=$(echo -n 'test' | base64) >/dev/null && echo 'ok'",
							transitKey,
						),
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
						RunAsNonRoot: ptr.To(true),
					},
				},
			},
		},
	}
}
