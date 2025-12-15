package helpers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os/exec"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InfraBaoConfig defines how to run the in-cluster "infra-bao" instance used
// as a mock external dependency (e.g., Transit auto-unseal).
// Infra-bao always runs in production mode with TLS enabled (never dev mode).
type InfraBaoConfig struct {
	Namespace string
	Name      string
	Image     string
	RootToken string
}

// EnsureInfraBao creates (or reuses) a production-mode OpenBao pod + service with TLS.
// The service is reachable at https://<name>.<namespace>.svc:8200.
// Infra-bao always runs in production mode with TLS (never dev mode).
func EnsureInfraBao(ctx context.Context, c client.Client, cfg InfraBaoConfig) error {
	if c == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if cfg.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("name is required")
	}
	if cfg.Image == "" {
		return fmt.Errorf("image is required")
	}
	if cfg.RootToken == "" {
		return fmt.Errorf("root token is required")
	}

	// Always generate TLS certificates and configure production mode (never dev mode)
	var tlsCertSecret *corev1.Secret
	var tlsCASecret *corev1.Secret
	var configMap *corev1.ConfigMap
	var unsealKeySecret *corev1.Secret
	{
		// Generate CA certificate
		caCertPEM, caKeyPEM, err := generateInfraBaoCA(cfg.Name)
		if err != nil {
			return fmt.Errorf("failed to generate CA for infra-bao: %w", err)
		}

		// Generate server certificate
		serverCertPEM, serverKeyPEM, err := generateInfraBaoServerCert(cfg.Namespace, cfg.Name, caCertPEM, caKeyPEM)
		if err != nil {
			return fmt.Errorf("failed to generate server certificate for infra-bao: %w", err)
		}

		// Create CA secret
		tlsCASecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.Name + "-tls-ca",
				Namespace: cfg.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"ca.crt": caCertPEM,
				"ca.key": caKeyPEM,
			},
		}
		err = c.Create(ctx, tlsCASecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create infra-bao CA Secret %s/%s: %w", cfg.Namespace, tlsCASecret.Name, err)
		}

		// Create server certificate secret
		tlsCertSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.Name + "-tls-server",
				Namespace: cfg.Namespace,
			},
			Type: corev1.SecretTypeTLS,
			Data: map[string][]byte{
				"tls.crt": serverCertPEM,
				"tls.key": serverKeyPEM,
				"ca.crt":  caCertPEM,
			},
		}
		err = c.Create(ctx, tlsCertSecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create infra-bao TLS server Secret %s/%s: %w", cfg.Namespace, tlsCertSecret.Name, err)
		}

		// Generate static unseal key for infra-bao
		staticKey, keyErr := generateUnsealKey()
		if keyErr != nil {
			return fmt.Errorf("failed to generate static unseal key: %w", keyErr)
		}

		// Create static unseal key secret
		unsealKeySecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.Name + "-unseal-key",
				Namespace: cfg.Namespace,
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"key": staticKey,
			},
		}
		err = c.Create(ctx, unsealKeySecret)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create infra-bao unseal key Secret %s/%s: %w", cfg.Namespace, unsealKeySecret.Name, err)
		}

		// Create ConfigMap with OpenBao config
		// Always use production mode (never dev mode) with static seal for auto-initialization
		// Static seal allows OpenBao to auto-initialize and auto-unseal without manual intervention
		configContent := `ui = true
disable_mlock = true

storage "file" {
  path = "/bao/data"
}

seal "static" {
  current_key = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_cert_file = "/etc/bao/tls/tls.crt"
  tls_key_file  = "/etc/bao/tls/tls.key"
  tls_client_ca_file = "/etc/bao/tls/ca.crt"
}
`

		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfg.Name + "-config",
				Namespace: cfg.Namespace,
			},
			Data: map[string]string{
				"config.hcl": configContent,
			},
		}
		err = c.Create(ctx, configMap)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create infra-bao ConfigMap %s/%s: %w", cfg.Namespace, configMap.Name, err)
		}
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": cfg.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Port:     8200,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}
	err := c.Create(ctx, svc)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create infra-bao Service %s/%s: %w", cfg.Namespace, cfg.Name, err)
	}

	// Always use production mode with TLS (never dev mode)
	podSpec := corev1.PodSpec{
		RestartPolicy: corev1.RestartPolicyAlways,
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
				Name:  "openbao",
				Image: cfg.Image,
				Ports: []corev1.ContainerPort{
					{ContainerPort: 8200, Name: "http"},
				},
				Command: []string{"bao", "server", "-config=/etc/bao/config/config.hcl"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "config",
						MountPath: "/etc/bao/config",
						ReadOnly:  true,
					},
					{
						Name:      "tls",
						MountPath: "/etc/bao/tls",
						ReadOnly:  true,
					},
					{
						Name:      "unseal",
						MountPath: "/etc/bao/unseal",
						ReadOnly:  true,
					},
					{
						Name:      "data",
						MountPath: "/bao/data",
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
		Volumes: []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: configMap.Name,
						},
					},
				},
			},
			{
				Name: "tls",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: tlsCertSecret.Name,
					},
				},
			},
			{
				Name: "unseal",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName:  unsealKeySecret.Name,
						DefaultMode: ptr.To(int32(0440)), // Match operator's secretFileMode
					},
				},
			},
			{
				Name: "data",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"app": cfg.Name,
			},
		},
		Spec: podSpec,
	}

	err = c.Create(ctx, pod)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create infra-bao Pod %s/%s: %w", cfg.Namespace, cfg.Name, err)
	}

	// Wait for pod to be running
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		current := &corev1.Pod{}
		if err := c.Get(ctx, types.NamespacedName{Name: cfg.Name, Namespace: cfg.Namespace}, current); err != nil {
			return fmt.Errorf("failed to get infra-bao Pod %s/%s: %w", cfg.Namespace, cfg.Name, err)
		}

		if current.Status.Phase == corev1.PodRunning {
			break
		}

		time.Sleep(1 * time.Second)
	}

	// Wait for OpenBao to be ready to accept connections (always HTTPS in production mode)
	infraAddr := fmt.Sprintf("https://%s.%s.svc:8200", cfg.Name, cfg.Namespace)

	readinessDeadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for time.Now().Before(readinessDeadline) {
		if err := checkInfraBaoReadinessLocal(ctx, cfg.Namespace, cfg.Name); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
		time.Sleep(2 * time.Second)
	}

	if lastErr != nil {
		return fmt.Errorf("timed out waiting for infra-bao %s/%s to be ready: %w", cfg.Namespace, cfg.Name, lastErr)
	}

	// Initialize infra-bao if not already initialized
	// This ensures infra-bao is ready for use in tests (e.g., Transit auto-unseal, ACME CA)
	if err := initializeInfraBao(ctx, c, infraAddr, cfg); err != nil {
		return fmt.Errorf("failed to initialize infra-bao %s/%s: %w", cfg.Namespace, cfg.Name, err)
	}

	return nil
}

// CleanupInfraBao best-effort deletes the infra-bao resources created by EnsureInfraBao.
// It is safe to call even if resources were partially created or already removed.
func CleanupInfraBao(ctx context.Context, c client.Client, cfg InfraBaoConfig) {
	// Order: pod -> service -> secrets/configmap
	_ = c.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name, Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name, Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name + "-tls-server", Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name + "-tls-ca", Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name + "-unseal-key", Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name + "-root-token", Namespace: cfg.Namespace}})
	_ = c.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cfg.Name + "-config", Namespace: cfg.Namespace}})
}

// checkInfraBaoReadinessLocal checks readiness from inside the pod via kubectl exec.
// We accept exit code 0 (unsealed) or 2 (sealed/uninitialized) as "responsive".
func checkInfraBaoReadinessLocal(ctx context.Context, namespace, name string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "exec",
		"-n", namespace,
		name,
		"--",
		"sh", "-c",
		"BAO_ADDR=https://127.0.0.1:8200 bao status -tls-skip-verify",
	)

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 2 {
			return nil
		}
		return fmt.Errorf("readiness check failed: %w", err)
	}

	return nil
}

// initializeInfraBao initializes infra-bao if it's not already initialized.
// It uses the static auto-unseal configuration, so no secret_shares or secret_threshold
// are needed. The root token is stored in a Secret for later use.
func initializeInfraBao(ctx context.Context, c client.Client, infraAddr string, cfg InfraBaoConfig) error {
	// Initialize infra-bao using bao operator init command inside the pod
	// This is simpler and more reliable than HTTP API calls
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	// Run 'bao operator init' inside the pod
	// For static seal, we don't need to pass secret_shares or secret_threshold
	// Set BAO_ADDR to use HTTPS with the local address (since we're inside the pod)
	// Use -tls-skip-verify since we're using self-signed certs in tests
	cmd := exec.CommandContext(initCtx, "kubectl", "exec",
		"-n", cfg.Namespace,
		cfg.Name,
		"--",
		"sh", "-c",
		"BAO_ADDR=https://127.0.0.1:8200 bao operator init -format=json -tls-skip-verify",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		// Check if already initialized (command returns error for this)
		if strings.Contains(outputStr, "already initialized") ||
			strings.Contains(outputStr, "Vault is already initialized") ||
			strings.Contains(outputStr, "OpenBao is already initialized") {
			return nil
		}
		return fmt.Errorf("bao operator init failed: %w, output: %s", err, outputStr)
	}

	// Parse JSON output to extract root token
	var initResult struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(output, &initResult); err != nil {
		return fmt.Errorf("failed to parse bao operator init output (output: %s): %w", string(output), err)
	}

	if initResult.RootToken == "" {
		return fmt.Errorf("bao operator init output missing root_token (output: %s)", string(output))
	}

	// Store root token in a Secret for later use
	// The Secret name matches the pattern used by tests: <name>-root-token
	secretName := cfg.Name + "-root-token"
	rootTokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cfg.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte(initResult.RootToken),
		},
	}

	err = c.Create(ctx, rootTokenSecret)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create root token Secret %s/%s: %w", cfg.Namespace, secretName, err)
	}

	// If secret already exists, update it with the new token
	if apierrors.IsAlreadyExists(err) {
		existingSecret := &corev1.Secret{}
		if getErr := c.Get(ctx, types.NamespacedName{Name: secretName, Namespace: cfg.Namespace}, existingSecret); getErr != nil {
			return fmt.Errorf("failed to get existing root token Secret: %w", getErr)
		}
		existingSecret.Data["token"] = []byte(initResult.RootToken)
		if updateErr := c.Update(ctx, existingSecret); updateErr != nil {
			return fmt.Errorf("failed to update root token Secret: %w", updateErr)
		}
	}

	return nil
}

// ConfigureInfraBaoTransit enables the transit secrets engine and ensures the
// given key exists. It runs a short-lived client pod (bao CLI) and returns its logs.
func ConfigureInfraBaoTransit(ctx context.Context, restCfg *rest.Config, c client.Client, namespace string, clientImage string, infraBaoAddress string, rootToken string, keyName string) (*PodResult, error) {
	if infraBaoAddress == "" {
		return nil, fmt.Errorf("infra-bao address is required")
	}
	if keyName == "" {
		return nil, fmt.Errorf("key name is required")
	}

	// Always use the root token captured during initialization
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      "infra-bao-root-token",
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to read infra-bao root token Secret: %w", err)
	}
	tokenBytes, ok := secret.Data["token"]
	if !ok || len(tokenBytes) == 0 {
		return nil, fmt.Errorf("infra-bao root token Secret missing token data")
	}
	tokenToUse := strings.TrimSpace(string(tokenBytes))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-bao-configure-transit",
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
						{Name: "BAO_ADDR", Value: infraBaoAddress},
						{Name: "BAO_TOKEN", Value: tokenToUse},
						// Skip TLS verification for self-signed certificates in test environment
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{`
bao status >/dev/null
if ! bao secrets list -format=json | grep -q '"transit/"'; then
  bao secrets enable transit >/dev/null
fi
if ! bao read -format=json transit/keys/` + keyName + ` >/dev/null 2>&1; then
  bao write -f transit/keys/` + keyName + ` type=aes256-gcm96 >/dev/null
fi
echo "ok"
`},
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
		return nil, err
	}

	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)
	return result, nil
}

// ConfigureInfraBaoPKIACME enables the PKI secrets engine and ACME support on infra-bao.
// It is safe to call multiple times.
func ConfigureInfraBaoPKIACME(ctx context.Context, restCfg *rest.Config, c client.Client, namespace string, clientImage string, infraBaoAddress string, rootToken string, clusterPath string) (*PodResult, error) {
	if infraBaoAddress == "" {
		return nil, fmt.Errorf("infra-bao address is required")
	}
	if clusterPath == "" {
		return nil, fmt.Errorf("cluster path is required")
	}

	// Always use the root token captured during initialization
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      "infra-bao-root-token",
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to read infra-bao root token Secret: %w", err)
	}
	tokenBytes, ok := secret.Data["token"]
	if !ok || len(tokenBytes) == 0 {
		return nil, fmt.Errorf("infra-bao root token Secret missing token data")
	}
	tokenToUse := strings.TrimSpace(string(tokenBytes))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-bao-configure-pki-acme",
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
						{Name: "BAO_ADDR", Value: infraBaoAddress},
						{Name: "BAO_TOKEN", Value: tokenToUse},
						// Skip TLS verification for self-signed certificates in test environment
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{`
bao status >/dev/null
if ! bao secrets list -format=json | grep -q '"pki/"'; then
  bao secrets enable pki >/dev/null
fi
bao secrets tune -allowed-response-headers=Location -allowed-response-headers=Replay-Nonce -allowed-response-headers=Link pki/ >/dev/null
if ! bao read -format=json pki/cert/ca >/dev/null 2>&1; then
  bao write -format=json pki/root/generate/internal common_name="E2E ACME Root CA" ttl=87600h >/dev/null
fi
bao write pki/config/cluster path="` + clusterPath + `" >/dev/null
bao write pki/config/acme enabled=true >/dev/null
echo "ok"
`},
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
		return nil, err
	}

	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)
	return result, nil
}

// FetchInfraBaoPKICA fetches the PKI CA certificate from infra-bao.
// This is the CA that signs ACME certificates, which is different from the TLS CA.
func FetchInfraBaoPKICA(ctx context.Context, restCfg *rest.Config, c client.Client, namespace string, clientImage string, infraBaoAddress string) ([]byte, error) {
	if infraBaoAddress == "" {
		return nil, fmt.Errorf("infra-bao address is required")
	}

	// Get the root token
	secret := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{
		Name:      "infra-bao-root-token",
		Namespace: namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to read infra-bao root token Secret: %w", err)
	}
	tokenBytes, ok := secret.Data["token"]
	if !ok || len(tokenBytes) == 0 {
		return nil, fmt.Errorf("infra-bao root token Secret missing token data")
	}
	tokenToUse := strings.TrimSpace(string(tokenBytes))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "infra-bao-fetch-pki-ca",
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
						{Name: "BAO_ADDR", Value: infraBaoAddress},
						{Name: "BAO_TOKEN", Value: tokenToUse},
						{Name: "BAO_SKIP_VERIFY", Value: "true"},
					},
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{
						`bao read -format=json pki/cert/ca`,
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
		return nil, fmt.Errorf("failed to run pod to fetch PKI CA: %w", err)
	}
	if result.Phase != corev1.PodSucceeded {
		return nil, fmt.Errorf("pod to fetch PKI CA failed, phase=%s, logs:\n%s", result.Phase, result.Logs)
	}

	// Parse JSON output to extract the CA certificate
	var pkiResponse struct {
		Data struct {
			Certificate string `json:"certificate"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Logs), &pkiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse PKI CA JSON response: %w, logs:\n%s", err, result.Logs)
	}

	pkiCA := strings.TrimSpace(pkiResponse.Data.Certificate)
	if pkiCA == "" {
		return nil, fmt.Errorf("PKI CA certificate is empty in response")
	}

	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)
	return []byte(pkiCA), nil
}

// generateInfraBaoCA generates a self-signed CA certificate for infra-bao.
func generateInfraBaoCA(name string) ([]byte, []byte, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s Infra-Bao Root CA (e2e)", name),
			Organization: []string{"OpenBao Operator E2E"},
		},
		NotBefore:             now.Add(-1 * time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// generateInfraBaoServerCert generates a server certificate for infra-bao signed by the given CA.
func generateInfraBaoServerCert(namespace string, name string, caCertPEM []byte, caKeyPEM []byte) ([]byte, []byte, error) {
	// Parse CA
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" {
		return nil, nil, fmt.Errorf("failed to decode CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil || keyBlock.Type != "EC PRIVATE KEY" {
		return nil, nil, fmt.Errorf("failed to decode CA private key PEM")
	}
	caKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA private key: %w", err)
	}

	// Generate server key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate server private key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	dnsNames := []string{
		"localhost",
		fmt.Sprintf("%s.%s.svc", name, namespace),
		fmt.Sprintf("*.%s.%s.svc", name, namespace),
	}

	ipAddresses := []net.IP{net.ParseIP("127.0.0.1")}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("%s.%s.svc", name, namespace),
			Organization: []string{"OpenBao Operator E2E"},
		},
		NotBefore:   now.Add(-1 * time.Hour),
		NotAfter:    now.AddDate(0, 0, 365),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal ECDSA private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

// generateUnsealKey generates a 32-byte random key for static unseal (same as operator uses).
// The raw bytes are written directly to the Secret so that the mounted
// file contains a 32-byte key compatible with OpenBao's static seal.
// OpenBao supports raw, base64, or hex encoding and will auto-detect the format.
func generateUnsealKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate unseal key: %w", err)
	}
	return key, nil
}
