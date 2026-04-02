package raft

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	operatorerrors "github.com/dc-tec/openbao-operator/internal/platform/errors"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestAutopilotBaseURL(t *testing.T) {
	t.Parallel()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "tenant-ns"},
	}
	got := autopilotBaseURL(cluster)
	want := "https://cluster-a-public.tenant-ns.svc:8200"
	if got != want {
		t.Fatalf("autopilotBaseURL()=%q, want %q", got, want)
	}
}

func TestHandleJWTAuthError(t *testing.T) {
	t.Parallel()

	mgr := &Manager{}
	cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"}}

	tests := []struct {
		name        string
		initialized bool
		err         error
		wantPerm    bool
		wantText    string
	}{
		{
			name:        "404 maps to prerequisite missing guidance",
			initialized: false,
			err: fmt.Errorf("failed to authenticate using JWT Auth: %w", &portopenbao.APIError{
				Operation:    "JWT auth request failed",
				StatusCode:   http.StatusNotFound,
				ResponseBody: `{"errors":["no handler for route"]}`,
			}),
			wantPerm: true,
			wantText: "Enable JWT auth",
		},
		{
			name:        "400 maps to role guidance",
			initialized: false,
			err: fmt.Errorf("failed to authenticate using JWT Auth: %w", &portopenbao.APIError{
				Operation:    "JWT auth request failed",
				StatusCode:   http.StatusBadRequest,
				ResponseBody: `{"errors":["invalid JWT token"]}`,
			}),
			wantPerm: true,
			wantText: "Ensure JWT role",
		},
		{
			name:        "initialized cluster uses manual guidance",
			initialized: true,
			err: fmt.Errorf("failed to authenticate using JWT Auth: %w", &portopenbao.APIError{
				Operation:    "JWT auth request failed",
				StatusCode:   http.StatusBadRequest,
				ResponseBody: `{"errors":["invalid JWT token"]}`,
			}),
			wantPerm: true,
			wantText: "Manually configure JWT role",
		},
		{
			name:        "other error remains generic",
			initialized: false,
			err:         errors.New("connection refused"),
			wantPerm:    false,
			wantText:    "failed to create authenticated OpenBao client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cluster.DeepCopy()
			c.Status.Initialized = tt.initialized
			err := mgr.handleJWTAuthError(c, tt.err)
			if tt.wantPerm && !operatorerrors.IsPermanent(err) {
				t.Fatalf("expected permanent error, got %v", err)
			}
			if !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("error=%q, expected to contain %q", err.Error(), tt.wantText)
			}
		})
	}
}

func TestGetTLSCACert(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "tenant-ns"}}

	t.Run("success", func(t *testing.T) {
		clientset := k8sfake.NewClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a-tls-ca", Namespace: "tenant-ns"},
			Data:       map[string][]byte{"ca.crt": []byte("pem-data")},
		})
		mgr := NewManager(clientset, nil)

		ca, err := mgr.getTLSCACert(context.Background(), cluster)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(ca) != "pem-data" {
			t.Fatalf("ca=%q, want pem-data", string(ca))
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		mgr := NewManager(k8sfake.NewClientset(), nil)
		_, err := mgr.getTLSCACert(context.Background(), cluster)
		if err == nil || !strings.Contains(err.Error(), "failed to get TLS CA Secret") {
			t.Fatalf("expected missing secret error, got %v", err)
		}
	})

	t.Run("missing ca key", func(t *testing.T) {
		clientset := k8sfake.NewClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a-tls-ca", Namespace: "tenant-ns"},
		})
		mgr := NewManager(clientset, nil)
		_, err := mgr.getTLSCACert(context.Background(), cluster)
		if err == nil || !strings.Contains(err.Error(), "missing 'ca.crt' key") {
			t.Fatalf("expected missing key error, got %v", err)
		}
	})

	t.Run("forbidden maps to transient kubernetes api", func(t *testing.T) {
		clientset := k8sfake.NewClientset()
		clientset.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "", Resource: "secrets"}, "cluster-a-tls-ca", errors.New("forbidden"))
		})
		mgr := NewManager(clientset, nil)
		_, err := mgr.getTLSCACert(context.Background(), cluster)
		if err == nil {
			t.Fatalf("expected forbidden error")
		}
		if !operatorerrors.IsTransientKubernetesAPI(err) {
			t.Fatalf("expected transient kubernetes api classification, got %v", err)
		}
	})
}

func TestGetJWTToken_MissingProjectedVolume(t *testing.T) {
	t.Parallel()

	if _, err := os.Stat("/var/run/secrets/tokens/openbao-token"); err == nil {
		t.Skip("projected token path exists on this environment; missing-file assertion not deterministic")
	}

	mgr := &Manager{}
	_, err := mgr.getJWTToken(logr.Discard())
	if err == nil || !strings.Contains(err.Error(), "failed to read JWT token from projected volume") {
		t.Fatalf("expected projected token read error, got %v", err)
	}
}

func TestReconcileAutopilotConfig_EarlyBranches(t *testing.T) {
	t.Parallel()

	t.Run("cluster not initialized is no-op", func(t *testing.T) {
		mgr := NewManager(k8sfake.NewClientset(), nil)
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
			Spec:       openbaov1alpha1.OpenBaoClusterSpec{Replicas: 3},
			Status:     openbaov1alpha1.OpenBaoClusterStatus{Initialized: false},
		}
		if err := mgr.ReconcileAutopilotConfig(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("expected nil error for uninitialized cluster, got %v", err)
		}
	})

	t.Run("missing root token secret is skipped", func(t *testing.T) {
		mgr := NewManager(k8sfake.NewClientset(), nil)
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
			Spec:       openbaov1alpha1.OpenBaoClusterSpec{Replicas: 3},
			Status:     openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
		}
		if err := mgr.ReconcileAutopilotConfig(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("expected nil error for missing root token secret, got %v", err)
		}
	})

	t.Run("root token secret without token is skipped", func(t *testing.T) {
		clientset := k8sfake.NewClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cluster-root-token", Namespace: "ns"}})
		mgr := NewManager(clientset, nil)
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
			Spec:       openbaov1alpha1.OpenBaoClusterSpec{Replicas: 3},
			Status:     openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
		}
		if err := mgr.ReconcileAutopilotConfig(context.Background(), logr.Discard(), cluster); err != nil {
			t.Fatalf("expected nil error when root token is empty, got %v", err)
		}
	})

	t.Run("missing tls secret after root token returns error", func(t *testing.T) {
		clientset := k8sfake.NewClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-root-token", Namespace: "ns"},
			Data:       map[string][]byte{"token": []byte("root-token")},
		})
		mgr := NewManager(clientset, nil)
		cluster := &openbaov1alpha1.OpenBaoCluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
			Spec:       openbaov1alpha1.OpenBaoClusterSpec{Replicas: 3},
			Status:     openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
		}
		err := mgr.ReconcileAutopilotConfig(context.Background(), logr.Discard(), cluster)
		if err == nil || !strings.Contains(err.Error(), "failed to create authenticated OpenBao client") {
			t.Fatalf("expected authenticated client creation error, got %v", err)
		}
	})
}
