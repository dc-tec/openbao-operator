package bootstrap

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func TestResolveSelfInitRefs_ResolvesConfigMapBackedRefs(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-kubernetes-auth",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/auth/kubernetes",
				AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
					Type: "kubernetes",
					ConfigFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "ConfigMap",
						Name: "kubernetes-auth-config",
					},
				},
			},
			{
				Name:      "create-app-policy",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/policies/acl/app-readwrite",
				Policy: &openbaov1alpha1.SelfInitPolicy{
					ContentFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "ConfigMap",
						Name: "app-policy",
					},
				},
			},
			{
				Name:      "enable-http-audit",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/http",
				AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
					Type: "http",
					SinkFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "ConfigMap",
						Name: "http-audit-sink",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kubernetes-auth-config",
					Namespace: cluster.Namespace,
				},
				Data: map[string]string{
					"default_role":       "operator",
					"token_reviewer_jwt": "configmap-token",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-policy",
					Namespace: cluster.Namespace,
				},
				Data: map[string]string{
					"policy.hcl": `path "secret/data/app" { capabilities = ["read"] }`,
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "http-audit-sink",
					Namespace: cluster.Namespace,
				},
				Data: map[string]string{
					"sink.json": `{"path":"http","description":"external audit","httpOptions":{"uri":"https://audit.example.test"}}`,
				},
			},
		).
		Build()
	manager := NewManager(client, testScheme, "openbao-operator-system")

	resolved, err := manager.resolveSelfInitRefs(context.Background(), cluster)
	if err != nil {
		t.Fatalf("resolveSelfInitRefs() error = %v", err)
	}

	authMethod := resolved.Spec.SelfInit.Requests[0].AuthMethod
	if authMethod == nil {
		t.Fatal("resolved auth method = nil")
	}
	if authMethod.ConfigFromRef != nil {
		t.Fatalf("auth method ConfigFromRef = %#v, want nil after resolution", authMethod.ConfigFromRef)
	}
	if authMethod.Config["default_role"] != "operator" || authMethod.Config["token_reviewer_jwt"] != "configmap-token" {
		t.Fatalf("resolved auth method config = %#v", authMethod.Config)
	}

	policy := resolved.Spec.SelfInit.Requests[1].Policy
	if policy == nil {
		t.Fatal("resolved policy = nil")
	}
	if policy.ContentFromRef != nil {
		t.Fatalf("policy ContentFromRef = %#v, want nil after resolution", policy.ContentFromRef)
	}
	if policy.Policy != `path "secret/data/app" { capabilities = ["read"] }` {
		t.Fatalf("policy content = %q", policy.Policy)
	}

	audit := resolved.Spec.SelfInit.Requests[2].AuditDevice
	if audit == nil {
		t.Fatal("resolved audit device = nil")
	}
	if audit.SinkFromRef != nil {
		t.Fatalf("audit SinkFromRef = %#v, want nil after resolution", audit.SinkFromRef)
	}
	if audit.Description != "external audit" {
		t.Fatalf("audit description = %q, want %q", audit.Description, "external audit")
	}
	if audit.HTTPOptions == nil || audit.HTTPOptions.URI != "https://audit.example.test" {
		t.Fatalf("audit HTTPOptions = %#v, want URI %q", audit.HTTPOptions, "https://audit.example.test")
	}

	if cluster.Spec.SelfInit.Requests[0].AuthMethod.ConfigFromRef == nil {
		t.Fatal("original cluster was mutated; expected auth method ConfigFromRef to remain set")
	}
}

func TestResolveSelfInitRefs_RejectsNamespacedRefs(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-kubernetes-auth",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/auth/kubernetes",
				AuthMethod: &openbaov1alpha1.SelfInitAuthMethod{
					Type: "kubernetes",
					ConfigFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind:      "Secret",
						Name:      "kubernetes-auth-config",
						Namespace: "other",
					},
				},
			},
		},
	}

	manager := NewManager(newTestClient(t), testScheme, "openbao-operator-system")

	_, err := manager.resolveSelfInitRefs(context.Background(), cluster)
	if err == nil {
		t.Fatal("resolveSelfInitRefs() error = nil, want namespace rejection")
	}
	if !strings.Contains(err.Error(), "refs must omit namespace") {
		t.Fatalf("resolveSelfInitRefs() error = %q, want namespace rejection", err.Error())
	}
}

func TestResolveSelfInitRefs_RejectsAuditPathMismatch(t *testing.T) {
	t.Parallel()

	cluster := newMinimalCluster("test-cluster", "default")
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true,
		Requests: []openbaov1alpha1.SelfInitRequest{
			{
				Name:      "enable-http-audit",
				Operation: openbaov1alpha1.SelfInitOperationUpdate,
				Path:      "sys/audit/http",
				AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{
					Type: "http",
					SinkFromRef: &openbaov1alpha1.TypedObjectReference{
						Kind: "Secret",
						Name: "http-audit-sink",
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(testScheme).
		WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "http-audit-sink",
				Namespace: cluster.Namespace,
			},
			Data: map[string][]byte{
				"sink.json": []byte(`{"path":"file","description":"wrong sink","fileOptions":{"path":"/var/log/openbao/audit.log"}}`),
			},
		}).
		Build()
	manager := NewManager(client, testScheme, "openbao-operator-system")

	_, err := manager.resolveSelfInitRefs(context.Background(), cluster)
	if err == nil {
		t.Fatal("resolveSelfInitRefs() error = nil, want audit path mismatch")
	}
	if !strings.Contains(err.Error(), `audit sink path "file" does not match request path "sys/audit/http"`) {
		t.Fatalf("resolveSelfInitRefs() error = %q, want audit path mismatch", err.Error())
	}
}
