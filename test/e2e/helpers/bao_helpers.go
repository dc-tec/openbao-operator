package helpers

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

// WriteSecret writes a KV secret to the specified path in OpenBao using a temporary pod.
func WriteSecret(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	token string,
	path string,
	data map[string]string,
) error {
	if baoAddr == "" {
		return fmt.Errorf("bao address is required")
	}
	if token == "" {
		return fmt.Errorf("bao token is required")
	}
	if path == "" {
		return fmt.Errorf("secret path is required")
	}

	// efficient check if kv is enabled is hard without auth, assuming enabled or handled by test setup
	// We'll try to write directly. If mount doesn't exist, we might need to enable it first.
	// For simplicity, assume "secret/" mount exists or similar (kv-v2 typical).
	// Actually, dev mode usually has "secret/".
	// Test setup should ensure the mount exists if it's not default.

	kvPairs := make([]string, 0, len(data))
	for k, v := range data {
		kvPairs = append(kvPairs, fmt.Sprintf("%s=%s", k, v))
	}
	dataStr := strings.Join(kvPairs, " ")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "write-secret-" + randString(5),
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
					Image: clientImage,
					Env: []corev1.EnvVar{
						{Name: "BAO_ADDR", Value: baoAddr},
						{Name: "BAO_TOKEN", Value: token},
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{
						fmt.Sprintf("bao kv put %s %s", path, dataStr),
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 1*time.Minute)
	if err != nil {
		return err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return fmt.Errorf("failed to write secret, logs:\n%s", result.Logs)
	}

	return nil
}

// ReadSecret reads a specific key from a KV secret at the specified path in OpenBao.
func ReadSecret(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	token string,
	path string,
	key string,
) (string, error) {
	if baoAddr == "" {
		return "", fmt.Errorf("bao address is required")
	}
	if token == "" {
		return "", fmt.Errorf("bao token is required")
	}
	if path == "" {
		return "", fmt.Errorf("secret path is required")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "read-secret-" + randString(5),
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
					Image: clientImage,
					Env: []corev1.EnvVar{
						{Name: "BAO_ADDR", Value: baoAddr},
						{Name: "BAO_TOKEN", Value: token},
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{
						fmt.Sprintf("bao kv get -field=%s %s", key, path),
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 1*time.Minute)
	if err != nil {
		return "", err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return "", fmt.Errorf("failed to read secret, logs:\n%s", result.Logs)
	}

	return strings.TrimSpace(result.Logs), nil
}

// WriteSecretViaJWT performs a K8s JWT login and writes a secret.
func WriteSecretViaJWT(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	serviceAccountName string,
	roleName string,
	path string,
	labels map[string]string,
	data map[string]string,
) error {
	kvPairs := make([]string, 0, len(data))
	for k, v := range data {
		kvPairs = append(kvPairs, fmt.Sprintf("%s=%s", k, v))
	}
	dataStr := strings.Join(kvPairs, " ")

	// Command sequence:
	// 1. Login with JWT using the projected ServiceAccount token (with openbao-internal audience)
	// 2. Set BAO_TOKEN from the response
	// 3. Write the secret
	cmd := fmt.Sprintf(`
set -e
export BAO_ADDR=%s
export BAO_SKIP_VERIFY=true
TOKEN=$(bao write -field=token auth/jwt/login role=%s jwt=@/var/run/secrets/openbao/token)
export BAO_TOKEN=$TOKEN
bao kv put %s %s
`, baoAddr, roleName, path, dataStr)

	expirationSeconds := int64(3600)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "write-jwt-" + randString(5),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: serviceAccountName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(100)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "openbao-token",
					VolumeSource: corev1.VolumeSource{
						Projected: &corev1.ProjectedVolumeSource{
							Sources: []corev1.VolumeProjection{
								{
									ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
										Audience:          "openbao-internal",
										ExpirationSeconds: &expirationSeconds,
										Path:              "token",
									},
								},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "bao",
					Image:   clientImage,
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{cmd},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "openbao-token",
							MountPath: "/var/run/secrets/openbao",
							ReadOnly:  true,
						},
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 1*time.Minute)
	if err != nil {
		return err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return fmt.Errorf("failed to write secret via JWT, logs:\n%s", result.Logs)
	}

	return nil
}

// ReadSecretViaJWT performs a K8s JWT login and reads a secret.
func ReadSecretViaJWT(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	serviceAccountName string,
	roleName string,
	path string,
	labels map[string]string,
	key string,
) (string, error) {
	cmd := fmt.Sprintf(`
set -e
export BAO_ADDR=%s
export BAO_SKIP_VERIFY=true
TOKEN=$(bao write -field=token auth/jwt/login role=%s jwt=@/var/run/secrets/openbao/token)
export BAO_TOKEN=$TOKEN
bao kv get -field=%s %s
`, baoAddr, roleName, key, path)

	expirationSeconds := int64(3600)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "read-jwt-" + randString(5),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: serviceAccountName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(100)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "openbao-token",
					VolumeSource: corev1.VolumeSource{
						Projected: &corev1.ProjectedVolumeSource{
							Sources: []corev1.VolumeProjection{
								{
									ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
										Audience:          "openbao-internal",
										ExpirationSeconds: &expirationSeconds,
										Path:              "token",
									},
								},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "bao",
					Image:   clientImage,
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{cmd},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "openbao-token",
							MountPath: "/var/run/secrets/openbao",
							ReadOnly:  true,
						},
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 1*time.Minute)
	if err != nil {
		return "", err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return "", fmt.Errorf("failed to read secret via JWT, logs:\n%s", result.Logs)
	}

	return strings.TrimSpace(result.Logs), nil
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

func randString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// CreateE2ERequests creates SelfInit requests for e2e testing (role, policy, mount).
func CreateE2ERequests(namespace string) []openbaov1alpha1.SelfInitRequest {
	return []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "enable-kv-secrets",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/mounts/secret",
			SecretEngine: &openbaov1alpha1.SelfInitSecretEngine{
				Type:        "kv",
				Description: "KV v2 secret engine",
				Options: map[string]string{
					"version": "2",
				},
			},
		},
		{
			Name:      "create-e2e-policy",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/policies/acl/e2e-test",
			Policy: &openbaov1alpha1.SelfInitPolicy{Policy: `path "secret/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "secret/data/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "secret/metadata/*" { capabilities = ["read", "list", "delete"] }`},
		},
		{
			Name:      "create-e2e-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/e2e-test",
			Data: MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"user_claim":      "sub",
				"bound_audiences": []string{"openbao-internal"},
				"bound_subject":   fmt.Sprintf("system:serviceaccount:%s:default", namespace),
				"token_policies":  []string{"e2e-test"},
				"ttl":             "1h",
			}),
		},
	}
}

// CreateAutopilotVerificationRequests creates SelfInit requests needed for autopilot verification via JWT.
// This includes the autopilot-read policy and JWT role for test-verifier.
func CreateAutopilotVerificationRequests(namespace string) []openbaov1alpha1.SelfInitRequest {
	return []openbaov1alpha1.SelfInitRequest{
		{
			Name:      "create-autopilot-read-policy",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "sys/policies/acl/autopilot-read",
			Policy: &openbaov1alpha1.SelfInitPolicy{
				Policy: `path "sys/storage/raft/autopilot/configuration" { capabilities = ["read"] }
path "sys/health" { capabilities = ["read"] }`,
			},
		},
		{
			Name:      "create-jwt-auth-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt/role/test-verifier",
			Data: MustJSON(map[string]interface{}{
				"role_type":       "jwt",
				"user_claim":      "sub",
				"bound_audiences": []string{"openbao-internal"},
				// Allow default service account in test namespace
				"bound_subject":  fmt.Sprintf("system:serviceaccount:%s:default", namespace),
				"token_policies": []string{"autopilot-read"},
				"ttl":            "1h",
			}),
		},
	}
}

// CreateHardenedProfileRequests creates the Requests for the Hardened Profile test.
// It includes autopilot verification requests plus any additional hardened-specific requests.
func CreateHardenedProfileRequests(namespace string) []openbaov1alpha1.SelfInitRequest {
	// Reuse autopilot verification requests to avoid duplication
	return CreateAutopilotVerificationRequests(namespace)
}

// VerifyRaftAutopilotViaJWT performs a K8s JWT login and verifies Raft Autopilot configuration.
func VerifyRaftAutopilotViaJWT(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	serviceAccountName string,
	labels map[string]string,
) error {
	// Command sequence:
	// 1. Login with JWT using the projected ServiceAccount token
	// 2. Set BAO_TOKEN
	// 3. Read autopilot config and verify cleanup_dead_servers is true
	// retry loop is handled inside the pod script to handle network latency
	cmd := fmt.Sprintf(`
set -e
export BAO_ADDR=%s
export BAO_SKIP_VERIFY=true

# Retry loop for login and verification
for i in $(seq 1 10); do
	echo "Attempt $i/10..."
	TOKEN=$(bao write -field=token auth/jwt/login role=test-verifier jwt=@/var/run/secrets/openbao/token 2>/dev/null || true)
	if [ -n "$TOKEN" ]; then
		export BAO_TOKEN=$TOKEN
		if bao read sys/storage/raft/autopilot/configuration 2>&1 | grep -q "cleanup_dead_servers.*true"; then
			echo "AUTOPILOT_CONFIGURED"
			exit 0
		fi
		echo "Autopilot config not yet propagated or incorrect"
	else
		echo "Login failed"
	fi
	sleep 2
done

echo "Failed to verify autopilot config after 10 attempts"
exit 1
`, baoAddr)

	expirationSeconds := int64(3600)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verify-autopilot-" + randString(5),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: serviceAccountName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(100)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "openbao-token",
					VolumeSource: corev1.VolumeSource{
						Projected: &corev1.ProjectedVolumeSource{
							Sources: []corev1.VolumeProjection{
								{
									ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
										Audience:          "openbao-internal",
										ExpirationSeconds: &expirationSeconds,
										Path:              "token",
									},
								},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "bao",
					Image:   clientImage,
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{cmd},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "openbao-token",
							MountPath: "/var/run/secrets/openbao",
							ReadOnly:  true,
						},
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 2*time.Minute)
	if err != nil {
		return err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return fmt.Errorf("failed to verify autopilot via JWT, logs:\n%s", result.Logs)
	}

	return nil
}

// VerifyRaftAutopilotMinQuorumViaJWT performs a K8s JWT login and verifies Raft Autopilot min_quorum value.
func VerifyRaftAutopilotMinQuorumViaJWT(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	baoAddr string,
	serviceAccountName string,
	labels map[string]string,
	expectedMinQuorum int,
) error {
	// Command sequence:
	// 1. Login with JWT using the projected ServiceAccount token
	// 2. Set BAO_TOKEN
	// 3. Read autopilot config and verify min_quorum matches expected value
	// retry loop is handled inside the pod script to handle network latency
	cmd := fmt.Sprintf(`
set -e
export BAO_ADDR=%s
export BAO_SKIP_VERIFY=true

# Retry loop for login and verification
for i in $(seq 1 10); do
	echo "Attempt $i/10..."
	TOKEN=$(bao write -field=token auth/jwt/login role=test-verifier jwt=@/var/run/secrets/openbao/token 2>&1 || echo "")
	if [ -n "$TOKEN" ] && [ "$TOKEN" != "null" ]; then
		export BAO_TOKEN=$TOKEN
		# Read config and check for min_quorum value
		bao read sys/storage/raft/autopilot/configuration
		# Use grep to find the min_quorum line and extract the number
		if bao read sys/storage/raft/autopilot/configuration 2>&1 | grep -q "min_quorum.*%d"; then
			echo "MIN_QUORUM_VERIFIED"
			exit 0
		fi
		echo "Autopilot config not yet propagated or incorrect"
	else
		echo "Login failed (token empty or null)"
	fi
	sleep 2
done

echo "Failed to verify min_quorum after 10 attempts"
exit 1
`, baoAddr, expectedMinQuorum)

	expirationSeconds := int64(3600)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "verify-autopilot-minquorum-" + randString(5),
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: serviceAccountName,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(100)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "openbao-token",
					VolumeSource: corev1.VolumeSource{
						Projected: &corev1.ProjectedVolumeSource{
							Sources: []corev1.VolumeProjection{
								{
									ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
										Audience:          "openbao-internal",
										ExpirationSeconds: &expirationSeconds,
										Path:              "token",
									},
								},
							},
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "bao",
					Image:   clientImage,
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{cmd},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "openbao-token",
							MountPath: "/var/run/secrets/openbao",
							ReadOnly:  true,
						},
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 2*time.Minute)
	if err != nil {
		return err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)

	if result.Phase != corev1.PodSucceeded {
		return fmt.Errorf("failed to verify min_quorum via JWT, logs:\n%s", result.Logs)
	}

	return nil
}
