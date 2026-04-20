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
		Spec:       openbaov1alpha1.OpenBaoClusterSpec{Service: &openbaov1alpha1.ServiceConfig{}},
	}
	got := autopilotBaseURL(cluster)
	want := "https://cluster-a-public.tenant-ns.svc:8200"
	if got != want {
		t.Fatalf("autopilotBaseURL()=%q, want %q", got, want)
	}
}

func TestAutopilotBaseURL_UsesHeadlessServiceWithoutExternalService(t *testing.T) {
	t.Parallel()
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "tenant-ns"},
	}
	got := autopilotBaseURL(cluster)
	want := "https://cluster-a.tenant-ns.svc:8200"
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

type fakeScaleDownClient struct {
	configureCalls []portopenbao.AutopilotConfig
	configureErr   error
	raftConfig     *portopenbao.RaftConfigurationResponse
	autopilotState *portopenbao.RaftAutopilotStateResponse
	readErr        error
	removeCalls    []string
	removeErr      error
	stepDownCalls  int
	stepDownErr    error
}

func (c *fakeScaleDownClient) ConfigureRaftAutopilot(_ context.Context, config portopenbao.AutopilotConfig) error {
	c.configureCalls = append(c.configureCalls, config)
	return c.configureErr
}

func (c *fakeScaleDownClient) ReadRaftConfiguration(context.Context) (*portopenbao.RaftConfigurationResponse, error) {
	if c.readErr != nil {
		return nil, c.readErr
	}
	return c.raftConfig, nil
}

func (c *fakeScaleDownClient) ReadRaftAutopilotState(context.Context) (*portopenbao.RaftAutopilotStateResponse, error) {
	if c.readErr != nil {
		return nil, c.readErr
	}
	return c.autopilotState, nil
}

func (c *fakeScaleDownClient) RemoveRaftPeer(_ context.Context, serverID string) error {
	c.removeCalls = append(c.removeCalls, serverID)
	return c.removeErr
}

func (c *fakeScaleDownClient) StepDownLeader(context.Context) error {
	c.stepDownCalls++
	return c.stepDownErr
}

type fakeScaleDownFactory struct {
	client       Client
	newWithToken int
}

func (f *fakeScaleDownFactory) NewWithJWT(context.Context, string, string, string) (Client, error) {
	return f.client, nil
}

func (f *fakeScaleDownFactory) NewWithToken(string, string) (Client, error) {
	f.newWithToken++
	return f.client, nil
}

type fakeScaleDownFactoryProvider struct {
	factory    ClientFactory
	clusterKey string
	caCert     []byte
}

func (p *fakeScaleDownFactoryProvider) FactoryFor(clusterKey string, caCert []byte) ClientFactory {
	p.clusterKey = clusterKey
	p.caCert = append([]byte(nil), caCert...)
	return p.factory
}

func TestPrepareScaleDown_RemovesFollowerAndUpdatesAutopilot(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
	}

	client := &fakeScaleDownClient{
		raftConfig: &portopenbao.RaftConfigurationResponse{
			Config: portopenbao.RaftConfiguration{
				Servers: []portopenbao.RaftServer{
					{NodeID: "cluster-0", Address: "https://cluster-0.cluster.ns.svc:8200", Leader: true, Voter: true},
					{NodeID: "cluster-1", Address: "https://cluster-1.cluster.ns.svc:8200", Voter: true},
					{NodeID: "cluster-2", Address: "https://cluster-2.cluster.ns.svc:8200", Voter: true},
				},
			},
		},
	}
	factory := &fakeScaleDownFactory{client: client}
	provider := &fakeScaleDownFactoryProvider{factory: factory}

	clientset := k8sfake.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-root-token", Namespace: "ns"},
			Data:       map[string][]byte{"token": []byte("root-token")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-tls-ca", Namespace: "ns"},
			Data:       map[string][]byte{"ca.crt": []byte("pem-data")},
		},
	)

	mgr := NewManager(clientset, provider)
	err := mgr.PrepareScaleDown(context.Background(), logr.Discard(), cluster, "cluster", 3, 2)
	if err != nil {
		t.Fatalf("PrepareScaleDown() error = %v", err)
	}

	if len(client.configureCalls) != 1 {
		t.Fatalf("expected one autopilot update, got %d", len(client.configureCalls))
	}
	if got := client.configureCalls[0].MinQuorum; got != 2 {
		t.Fatalf("autopilot min_quorum = %d, want 2", got)
	}
	if client.configureCalls[0].CleanupDeadServers {
		t.Fatalf("cleanup_dead_servers = true, want false for 2 replicas")
	}
	if len(client.removeCalls) != 1 || client.removeCalls[0] != "cluster-2" {
		t.Fatalf("removeCalls = %v, want [cluster-2]", client.removeCalls)
	}
	if client.stepDownCalls != 0 {
		t.Fatalf("stepDownCalls = %d, want 0", client.stepDownCalls)
	}
	if factory.newWithToken != 1 {
		t.Fatalf("NewWithToken() calls = %d, want 1", factory.newWithToken)
	}
	if provider.clusterKey != "ns/cluster" {
		t.Fatalf("clusterKey = %q, want ns/cluster", provider.clusterKey)
	}
	if string(provider.caCert) != "pem-data" {
		t.Fatalf("caCert = %q, want pem-data", string(provider.caCert))
	}
}

func TestPrepareScaleDown_StepsDownLeaderVictim(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
	}

	client := &fakeScaleDownClient{
		raftConfig: &portopenbao.RaftConfigurationResponse{
			Config: portopenbao.RaftConfiguration{
				Servers: []portopenbao.RaftServer{
					{NodeID: "cluster-0", Address: "https://cluster-0.cluster.ns.svc:8200", Voter: true},
					{NodeID: "cluster-1", Address: "https://cluster-1.cluster.ns.svc:8200", Voter: true},
					{NodeID: "cluster-2", Address: "https://cluster-2.cluster.ns.svc:8200", Leader: true, Voter: true},
				},
			},
		},
	}

	clientset := k8sfake.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-root-token", Namespace: "ns"},
			Data:       map[string][]byte{"token": []byte("root-token")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-tls-ca", Namespace: "ns"},
			Data:       map[string][]byte{"ca.crt": []byte("pem-data")},
		},
	)

	mgr := NewManager(clientset, &fakeScaleDownFactoryProvider{factory: &fakeScaleDownFactory{client: client}})
	err := mgr.PrepareScaleDown(context.Background(), logr.Discard(), cluster, "cluster", 3, 2)
	if err == nil || !strings.Contains(err.Error(), "waiting for leader step-down on cluster-2 to complete") {
		t.Fatalf("expected step-down wait error, got %v", err)
	}
	if client.stepDownCalls != 1 {
		t.Fatalf("stepDownCalls = %d, want 1", client.stepDownCalls)
	}
	if len(client.removeCalls) != 0 {
		t.Fatalf("removeCalls = %v, want none", client.removeCalls)
	}
}

func TestPrepareReadReplicaScaleDown_RemovesNonVoter(t *testing.T) {
	t.Parallel()

	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			Profile:  openbaov1alpha1.ProfileDevelopment,
			Replicas: 3,
		},
		Status: openbaov1alpha1.OpenBaoClusterStatus{Initialized: true},
	}

	client := &fakeScaleDownClient{
		raftConfig: &portopenbao.RaftConfigurationResponse{
			Config: portopenbao.RaftConfiguration{
				Servers: []portopenbao.RaftServer{
					{NodeID: "cluster-0", Address: "https://cluster-0.cluster.ns.svc:8200", Leader: true, Voter: true},
					{NodeID: "cluster-read-0", Address: "https://cluster-read-0.cluster.ns.svc:8200", Voter: false},
					{NodeID: "cluster-read-1", Address: "https://cluster-read-1.cluster.ns.svc:8200", Voter: false},
				},
			},
		},
	}

	clientset := k8sfake.NewClientset(
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-root-token", Namespace: "ns"},
			Data:       map[string][]byte{"token": []byte("root-token")},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-tls-ca", Namespace: "ns"},
			Data:       map[string][]byte{"ca.crt": []byte("pem-data")},
		},
	)

	mgr := NewManager(clientset, &fakeScaleDownFactoryProvider{factory: &fakeScaleDownFactory{client: client}})
	err := mgr.PrepareReadReplicaScaleDown(context.Background(), logr.Discard(), cluster, "cluster-read", 2, 1)
	if err != nil {
		t.Fatalf("PrepareReadReplicaScaleDown() error = %v", err)
	}
	if len(client.removeCalls) != 1 || client.removeCalls[0] != "cluster-read-1" {
		t.Fatalf("removeCalls = %v, want [cluster-read-1]", client.removeCalls)
	}
	if len(client.configureCalls) != 0 {
		t.Fatalf("configureCalls = %v, want none", client.configureCalls)
	}
	if client.stepDownCalls != 0 {
		t.Fatalf("stepDownCalls = %d, want 0", client.stepDownCalls)
	}
}

func TestWrapScaleDownPermissionError_SelfInitClusterRequiresUpdatedPolicy(t *testing.T) {
	t.Parallel()

	mgr := &Manager{}
	cluster := &openbaov1alpha1.OpenBaoCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "ns"},
		Spec: openbaov1alpha1.OpenBaoClusterSpec{
			SelfInit: &openbaov1alpha1.SelfInitConfig{
				Enabled: true,
				OIDC:    &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
			},
		},
	}

	err := mgr.wrapScaleDownPermissionError(cluster, fmt.Errorf("read raft config: %w", &portopenbao.APIError{
		Operation:    "raft configuration request failed",
		StatusCode:   http.StatusForbidden,
		ResponseBody: `{"errors":["permission denied"]}`,
	}))
	if err == nil {
		t.Fatal("expected permission error")
	}
	if !operatorerrors.IsPermanent(err) {
		t.Fatalf("expected permanent classification, got %v", err)
	}
	if !strings.Contains(err.Error(), "remove-peer") {
		t.Fatalf("expected permission guidance, got %v", err)
	}
}
