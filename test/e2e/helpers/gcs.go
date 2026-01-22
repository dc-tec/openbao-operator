package helpers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GCSConfig defines how to run fake-gcs-server for GCS emulator testing.
type GCSConfig struct {
	Namespace string
	Name      string
	Image     string
	Port      int32
	Project   string // GCP project ID for fake-gcs-server
}

// DefaultGCSConfig returns a default fake-gcs-server configuration.
func DefaultGCSConfig() GCSConfig {
	return GCSConfig{
		Name:    "fake-gcs-server",
		Image:   "fsouza/fake-gcs-server:latest",
		Port:    4443,
		Project: "test-project",
	}
}

// EnsureFakeGCS creates (or reuses) a fake-gcs-server Deployment + Service for GCS emulator testing.
// The service is reachable at http://<name>.<namespace>.svc:<port>.
func EnsureFakeGCS(ctx context.Context, c client.Client, cfg GCSConfig) error {
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
	if cfg.Port <= 0 {
		cfg.Port = 4443
	}

	// Ensure namespace exists
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cfg.Namespace,
		},
	}
	if err := c.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create namespace %s: %w", cfg.Namespace, err)
	}

	// Create Service
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
					Name:       "http",
					Port:       cfg.Port,
					TargetPort: intstr.FromInt32(cfg.Port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
	if err := c.Create(ctx, svc); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create fake-gcs Service %s/%s: %w", cfg.Namespace, svc.Name, err)
	}

	// Create Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels: map[string]string{
				"app": cfg.Name,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": cfg.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": cfg.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  cfg.Name,
							Image: cfg.Image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: cfg.Port,
									Name:          "http",
								},
							},
							Args: []string{
								"-scheme", "http",
								"-port", fmt.Sprintf("%d", cfg.Port),
								"-backend", "memory",
								"-external-url", fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", cfg.Name, cfg.Namespace, cfg.Port),
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/storage/v1/b",
										Port: intstr.FromInt32(cfg.Port),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/storage/v1/b",
										Port: intstr.FromInt32(cfg.Port),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
								// Note: fake-gcs-server image runs as root, so we cannot set RunAsNonRoot
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := c.Create(ctx, deployment); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create fake-gcs Deployment %s/%s: %w", cfg.Namespace, cfg.Name, err)
	}

	// Wait for Deployment to be ready
	return waitForDeploymentReady(ctx, c, cfg.Namespace, cfg.Name, "fake-gcs")
}

// CreateGCSCredentialsSecret creates a Kubernetes Secret with valid GCS service account JSON credentials.
// The credentials are valid in format but are dummy credentials for use with fake-gcs-server.
func CreateGCSCredentialsSecret(ctx context.Context, c client.Client, namespace, name, projectID string) error {
	if c == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	if projectID == "" {
		projectID = "test-project"
	}

	// Generate a valid RSA private key for the dummy credentials
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode the private key in PEM format (PKCS8)
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %w", err)
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyDER,
	})

	// Create minimal but valid service account JSON
	credsJSON := map[string]interface{}{
		"type":                        "service_account",
		"project_id":                  projectID,
		"private_key_id":              "test-key-id",
		"private_key":                 string(privateKeyPEM),
		"client_email":                fmt.Sprintf("test@%s.iam.gserviceaccount.com", projectID),
		"client_id":                   "123456789",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url": fmt.Sprintf(
			"https://www.googleapis.com/robot/v1/metadata/x509/test%%40%s.iam.gserviceaccount.com",
			projectID,
		),
	}

	credsBytes, err := json.Marshal(credsJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal GCS credentials: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"credentials.json": credsBytes,
		},
	}

	if err := c.Create(ctx, secret); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create GCS credentials secret: %w", err)
	}

	return nil
}

// CreateFakeGCSBucket creates a bucket in the fake-gcs-server emulator by running a Pod inside the cluster.
// This is necessary because fake-gcs-server with -backend memory starts completely empty.
// The bucket must exist before OpenBao's backup job attempts to write snapshots.
// This function uses a Pod to make the HTTP request so it can access the cluster's internal DNS.
func CreateFakeGCSBucket(
	ctx context.Context,
	restCfg *rest.Config,
	c client.Client,
	namespace, endpoint, bucketName string,
) error {
	if restCfg == nil {
		return fmt.Errorf("rest config is required")
	}
	if c == nil {
		return fmt.Errorf("kubernetes client is required")
	}
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if bucketName == "" {
		return fmt.Errorf("bucket name is required")
	}

	// Normalize endpoint (remove trailing slash if present)
	endpoint = strings.TrimSuffix(endpoint, "/")
	url := fmt.Sprintf("%s/storage/v1/b?project=%s", endpoint, "test-project")
	payload := fmt.Sprintf(`{"name": "%s"}`, bucketName)

	// Generate unique pod name
	suffix, err := randomHexSuffix(4)
	if err != nil {
		return fmt.Errorf("failed to generate pod suffix: %w", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("create-gcs-bucket-%s", suffix),
			Namespace: namespace,
			Labels: map[string]string{
				"openbao.org/component": "gcs-bucket-create",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(1000)),
				RunAsGroup:   ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "curl",
					Image:   "curlimages/curl:8.4.0",
					Command: []string{"/bin/sh", "-ec"},
					Args: []string{fmt.Sprintf(`
set -e
# Create bucket via HTTP POST
response=$(curl -s -w "\n%%{http_code}" -X POST -H "Content-Type: application/json" -d '%s' "%s")
http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

# Accept 200 (created) or 409 (already exists)
if [ "$http_code" -eq 200 ] || [ "$http_code" -eq 409 ]; then
  echo "Bucket %s created or already exists (status: $http_code)"
  exit 0
else
  echo "Failed to create bucket, status: $http_code, response: $body" >&2
  exit 1
fi
`, payload, url, bucketName)},
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
		return fmt.Errorf("failed to run bucket creation pod: %w", err)
	}

	if result.Phase != corev1.PodSucceeded {
		return fmt.Errorf("bucket creation pod failed, phase=%s, logs:\n%s", result.Phase, result.Logs)
	}

	_ = DeletePodBestEffort(ctx, c, namespace, pod.Name)
	return nil
}
