package helpers

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/constants"
	openbaolabels "github.com/dc-tec/openbao-operator/internal/openbao"
)

const (
	jwtAutopilotVerifyAttempts    = 30
	jwtAutopilotVerifySleepSecond = 3
)

// executePod creates a pod to execute a command with standard environment variables.
func executePod(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	env []corev1.EnvVar,
	cmd string,
) (string, error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "exec-" + randString(5),
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
					Name:    "bao",
					Image:   clientImage,
					Env:     env,
					Command: []string{"/bin/sh", "-ec"},
					Args:    []string{cmd},
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
		return "", fmt.Errorf("pod failed, logs:\n%s", result.Logs)
	}
	return result.Logs, nil
}

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

	kvPairs := make([]string, 0, len(data))
	for k, v := range data {
		kvPairs = append(kvPairs, fmt.Sprintf("%s=%s", k, v))
	}
	dataStr := strings.Join(kvPairs, " ")

	env := []corev1.EnvVar{
		{Name: "BAO_ADDR", Value: baoAddr},
		{Name: "BAO_TOKEN", Value: token},
		{Name: "BAO_SKIP_VERIFY", Value: "true"},
	}
	cmd := fmt.Sprintf("bao kv put %s %s", path, dataStr)

	_, err := executePod(ctx, restCfg, c, namespace, clientImage, env, cmd)
	return err
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

	env := []corev1.EnvVar{
		{Name: "BAO_ADDR", Value: baoAddr},
		{Name: "BAO_TOKEN", Value: token},
		{Name: "BAO_SKIP_VERIFY", Value: "true"},
	}
	cmd := fmt.Sprintf("bao kv get -field=%s %s", key, path)

	logs, err := executePod(ctx, restCfg, c, namespace, clientImage, env, cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(logs), nil
}

// ResolveActiveOpenBaoAddress returns a deterministic in-cluster address that targets the active,
// unsealed OpenBao pod when possible.
//
// Many E2E tests used the headless Service address (<cluster>.<ns>.svc...), which can resolve to
// *any* pod (including not-yet-ready or sealed pods) and introduce flakes on slower CI runners.
func ResolveActiveOpenBaoAddress(ctx context.Context, c client.Client, namespace, clusterName string) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("context is required")
	}
	if c == nil {
		return "", fmt.Errorf("kubernetes client is required")
	}
	if namespace == "" {
		return "", fmt.Errorf("namespace is required")
	}
	if clusterName == "" {
		return "", fmt.Errorf("cluster name is required")
	}

	var pods corev1.PodList
	if err := c.List(ctx, &pods,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{
			constants.LabelAppName:        constants.LabelValueAppNameOpenBao,
			constants.LabelAppManagedBy:   constants.LabelValueAppManagedByOpenBaoOperator,
			constants.LabelOpenBaoCluster: clusterName,
		}),
	); err != nil {
		return "", fmt.Errorf("failed to list OpenBao pods (cluster=%s): %w", clusterName, err)
	}

	type candidate struct {
		name        string
		sealedKnown bool
		sealedValue bool
		activeKnown bool
		activeValue bool
		deleting    bool
	}

	candidates := make([]candidate, 0, len(pods.Items))
	for i := range pods.Items {
		pod := &pods.Items[i]
		active, activePresent, activeErr := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelActive)
		if activeErr != nil {
			continue
		}
		if !activePresent || !active {
			continue
		}

		sealed, sealedPresent, sealedErr := openbaolabels.ParseBoolLabel(pod.Labels, openbaolabels.LabelSealed)
		if sealedErr != nil {
			continue
		}

		candidates = append(candidates, candidate{
			name:        pod.Name,
			sealedKnown: sealedPresent,
			sealedValue: sealed,
			activeKnown: activePresent,
			activeValue: active,
			deleting:    pod.DeletionTimestamp != nil,
		})
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no active OpenBao pod found (cluster=%s)", clusterName)
	}

	// Prefer active + unsealed candidates when the sealed label is present.
	unsealed := make([]candidate, 0, len(candidates))
	unknown := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.deleting {
			continue
		}
		if c.sealedKnown {
			if !c.sealedValue {
				unsealed = append(unsealed, c)
			}
			continue
		}
		unknown = append(unknown, c)
	}

	pick := func(list []candidate) (string, bool) {
		if len(list) == 0 {
			return "", false
		}
		sort.Slice(list, func(i, j int) bool { return list[i].name < list[j].name })
		return list[0].name, true
	}

	podName, ok := pick(unsealed)
	if !ok {
		podName, ok = pick(unknown)
	}
	if !ok {
		return "", fmt.Errorf("no usable active OpenBao pod found (cluster=%s)", clusterName)
	}

	// Use the headless service name (<clusterName>) for stable in-cluster pod DNS:
	// https://<podName>.<clusterName>.<namespace>.svc:8200
	return fmt.Sprintf("https://%s.%s.%s.svc:8200", podName, clusterName, namespace), nil
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
TOKEN=$(bao write -field=token auth/jwt-operator/login role=%s jwt=@/var/run/secrets/openbao/token)
export BAO_TOKEN=$TOKEN
bao kv put %s %s
`, baoAddr, roleName, path, dataStr)

	if _, err := executeJWTPod(ctx, restCfg, c, namespace, clientImage, serviceAccountName, labels, cmd); err != nil {
		return fmt.Errorf("failed to write secret via JWT: %w", err)
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
TOKEN=$(bao write -field=token auth/jwt-operator/login role=%s jwt=@/var/run/secrets/openbao/token)
export BAO_TOKEN=$TOKEN
bao kv get -field=%s %s
`, baoAddr, roleName, key, path)

	logs, err := executeJWTPod(ctx, restCfg, c, namespace, clientImage, serviceAccountName, labels, cmd)
	if err != nil {
		return "", fmt.Errorf("failed to read secret via JWT: %w", err)
	}

	return strings.TrimSpace(logs), nil
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

//nolint:unparam
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
			Policy: &openbaov1alpha1.SelfInitPolicy{
				Policy: `path "secret/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "secret/data/*" { capabilities = ["create", "read", "update", "delete", "list"] }
path "secret/metadata/*" { capabilities = ["read", "list", "delete"] }`},
		},
		{
			Name:      "create-e2e-role",
			Operation: openbaov1alpha1.SelfInitOperationUpdate,
			Path:      "auth/jwt-operator/role/e2e-test",
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
			Path:      "auth/jwt-operator/role/test-verifier",
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

// executeJWTPod creates a pod to execute a command with a projected service account token for JWT auth.
func executeJWTPod(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace string,
	clientImage string,
	serviceAccountName string,
	labels map[string]string,
	cmd string,
) (string, error) {
	expirationSeconds := int64(3600)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "jwt-exec-" + randString(5),
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

	result, err := RunPodUntilCompletion(ctx, restCfg, c, pod, 4*time.Minute)
	if err != nil {
		return "", err
	}
	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name) // cleanup immediately

	if result.Phase != corev1.PodSucceeded {
		return "", fmt.Errorf("pod failed, logs:\n%s", result.Logs)
	}
	return result.Logs, nil
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
	for i in $(seq 1 %d); do
		echo "Attempt $i/%d..."
		set +e
		LOGIN_OUTPUT=$(bao write -field=token auth/jwt-operator/login \
			role=test-verifier jwt=@/var/run/secrets/openbao/token 2>&1)
		LOGIN_EXIT=$?
		set -e
		if [ $LOGIN_EXIT -eq 0 ] && [ -n "$LOGIN_OUTPUT" ] && [ "$LOGIN_OUTPUT" != "null" ]; then
			export BAO_TOKEN=$LOGIN_OUTPUT
			if ! bao status >/dev/null 2>&1; then
				echo "OpenBao not ready yet"
				sleep %d
				continue
			fi
			set +e
			CLEANUP_DEAD_SERVERS=$(bao read -field=cleanup_dead_servers sys/storage/raft/autopilot/configuration 2>&1 | tr -d '\r\n')
			READ_EXIT=$?
			set -e
			if [ $READ_EXIT -eq 0 ] && [ "$CLEANUP_DEAD_SERVERS" = "true" ]; then
				echo "AUTOPILOT_CONFIGURED"
				exit 0
			fi
			echo "Autopilot config not yet propagated or incorrect"
			echo "cleanup_dead_servers read output: $CLEANUP_DEAD_SERVERS"
		else
			echo "Login failed"
			echo "Login command output: $LOGIN_OUTPUT"
		fi
		sleep %d
	done

	echo "Failed to verify autopilot config after %d attempts"
	exit 1
`, baoAddr, jwtAutopilotVerifyAttempts, jwtAutopilotVerifyAttempts, jwtAutopilotVerifySleepSecond, jwtAutopilotVerifySleepSecond, jwtAutopilotVerifyAttempts)

	if _, err := executeJWTPod(ctx, restCfg, c, namespace, clientImage, serviceAccountName, labels, cmd); err != nil {
		return fmt.Errorf("failed to verify autopilot via JWT: %w", err)
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
	for i in $(seq 1 %d); do
		echo "Attempt $i/%d..."
		set +e
		LOGIN_OUTPUT=$(bao write -field=token auth/jwt-operator/login \
			role=test-verifier jwt=@/var/run/secrets/openbao/token 2>&1)
		LOGIN_EXIT=$?
		set -e
		if [ $LOGIN_EXIT -eq 0 ] && [ -n "$LOGIN_OUTPUT" ] && [ "$LOGIN_OUTPUT" != "null" ]; then
			export BAO_TOKEN=$LOGIN_OUTPUT
			if ! bao status >/dev/null 2>&1; then
				echo "OpenBao not ready yet"
				sleep %d
				continue
			fi
			set +e
			MIN_QUORUM=$(bao read -field=min_quorum sys/storage/raft/autopilot/configuration 2>&1 | tr -d '\r\n')
			READ_EXIT=$?
			set -e
			if [ $READ_EXIT -eq 0 ] && [ "$MIN_QUORUM" = "%d" ]; then
				echo "MIN_QUORUM_VERIFIED"
				exit 0
			fi
			echo "Autopilot config not yet propagated or incorrect"
			echo "min_quorum read output: $MIN_QUORUM"
		else
			echo "Login failed"
			echo "Login command output: $LOGIN_OUTPUT"
		fi
		sleep %d
	done

echo "Failed to verify min_quorum after %d attempts"
exit 1
`, baoAddr, jwtAutopilotVerifyAttempts, jwtAutopilotVerifyAttempts, jwtAutopilotVerifySleepSecond, expectedMinQuorum, jwtAutopilotVerifySleepSecond, jwtAutopilotVerifyAttempts)

	if _, err := executeJWTPod(ctx, restCfg, c, namespace, clientImage, serviceAccountName, labels, cmd); err != nil {
		return fmt.Errorf("failed to verify min_quorum via JWT: %w", err)
	}

	return nil
}
