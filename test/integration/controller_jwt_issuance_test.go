//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	adapterauth "github.com/dc-tec/openbao-operator/internal/adapter/auth"
	adapteropenbao "github.com/dc-tec/openbao-operator/internal/adapter/openbao"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func controllerJWTIssuer(t *testing.T) (*adapterauth.ControllerTokenSource, kubernetes.Interface, *corev1.Pod) {
	t.Helper()
	namespace := newTestNamespace(t)
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: namespace}}
	require.NoError(t, k8sClient.Create(ctx, sa))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "controller", Namespace: namespace}, Spec: corev1.PodSpec{
		ServiceAccountName: sa.Name, AutomountServiceAccountToken: ptr.To(false),
		Containers: []corev1.Container{{Name: "controller", Image: "example.invalid/controller:test"}},
	}}
	require.NoError(t, k8sClient.Create(ctx, pod))
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "token-issuer", Namespace: namespace}, Rules: []rbacv1.PolicyRule{{
		APIGroups: []string{""}, Resources: []string{"serviceaccounts/token"}, ResourceNames: []string{sa.Name}, Verbs: []string{"create"},
	}}}
	require.NoError(t, k8sClient.Create(ctx, role))
	require.NoError(t, k8sClient.Create(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: role.Name, Namespace: namespace},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
		Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: sa.Name, Namespace: namespace}},
	}))
	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{UserName: "system:serviceaccount:" + namespace + ":" + sa.Name}
	clientset, err := kubernetes.NewForConfig(impersonated)
	require.NoError(t, err)
	t.Setenv("POD_NAMESPACE", namespace)
	t.Setenv("OPERATOR_SERVICE_ACCOUNT_NAME", sa.Name)
	t.Setenv("POD_NAME", pod.Name)
	t.Setenv("POD_UID", string(pod.UID))
	return adapterauth.NewControllerTokenSource(clientset), clientset, pod
}

func jwtTarget(t *testing.T, name string) *openbaov1alpha1.OpenBaoCluster {
	t.Helper()
	cluster := newMinimalClusterObj(newTestNamespace(t), name)
	cluster.Spec.ControllerJWTMode = openbaov1alpha1.ControllerJWTModeTarget
	cluster.Spec.SelfInit = &openbaov1alpha1.SelfInitConfig{
		Enabled: true, OIDC: &openbaov1alpha1.SelfInitOIDCConfig{Enabled: true},
		Requests: []openbaov1alpha1.SelfInitRequest{{
			Name: "audit", Operation: openbaov1alpha1.SelfInitOperationUpdate, Path: "sys/audit/stdout",
			AuditDevice: &openbaov1alpha1.SelfInitAuditDevice{Type: "file", FileOptions: &openbaov1alpha1.FileAuditOptions{FilePath: "stdout"}},
		}},
	}
	require.NoError(t, k8sClient.Create(ctx, cluster))
	return cluster
}

func TestControllerJWT_TokenRequestRBACAndAudience(t *testing.T) {
	source, clientset, pod := controllerJWTIssuer(t)
	target := jwtTarget(t, "target")
	var token string
	require.Eventually(t, func() bool { var err error; token, err = source.Token(ctx, target); return err == nil }, 5*time.Second, 50*time.Millisecond)
	admin, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)
	audience, err := portauth.ControllerJWTAudience(target, "")
	require.NoError(t, err)
	for _, test := range []struct {
		audience string
		valid    bool
	}{{audience, true}, {"urn:openbao:controller:another", false}, {"openbao-internal", false}} {
		review, err := admin.AuthenticationV1().TokenReviews().Create(ctx, &authenticationv1.TokenReview{Spec: authenticationv1.TokenReviewSpec{Token: token, Audiences: []string{test.audience}}}, metav1.CreateOptions{})
		require.NoError(t, err)
		require.Equal(t, test.valid, review.Status.Authenticated)
	}
	for _, test := range []struct{ namespace, name string }{{pod.Namespace, "another"}, {"default", "controller"}} {
		_, err := clientset.CoreV1().ServiceAccounts(test.namespace).CreateToken(ctx, test.name, &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{Audiences: []string{audience}}}, metav1.CreateOptions{})
		require.True(t, apierrors.IsForbidden(err), "unexpected error: %v", err)
	}
}

// Run against two disposable OpenBao dev servers created by the test.
// Docker is required only when OPENBAO_JWT_TEST_IMAGE selects this lane.
func TestControllerJWT_OpenBaoCrossTargetReplay(t *testing.T) {
	image := os.Getenv("OPENBAO_JWT_TEST_IMAGE")
	if image == "" {
		t.Skip("run make test-controller-jwt-openbao to enable the Docker-backed replay test")
	}
	root := uuid.NewString()
	addresses := []string{startControllerJWTTestServer(t, image, root), startControllerJWTTestServer(t, image, root)}
	source, _, pod := controllerJWTIssuer(t)
	targets := []*openbaov1alpha1.OpenBaoCluster{jwtTarget(t, "target-a"), jwtTarget(t, "target-b")}
	cert, err := os.ReadFile(filepath.Join(testEnv.ControlPlane.APIServer.CertDir, "sa-signer.crt"))
	require.NoError(t, err)
	tokens := make([]string, 2)
	for i, target := range targets {
		require.Eventually(t, func() bool { var err error; tokens[i], err = source.Token(ctx, target); return err == nil }, 5*time.Second, 50*time.Millisecond)
		audience, err := portauth.ControllerJWTAudience(target, "")
		require.NoError(t, err)
		writeJWTTestAPI(t, addresses[i], root, "sys/auth/jwt-operator", map[string]any{"type": "jwt"})
		writeJWTTestAPI(t, addresses[i], root, "auth/jwt-operator/config", map[string]any{"jwt_validation_pubkeys": []string{string(cert)}, "bound_issuer": cfg.Host})
		writeJWTTestAPI(t, addresses[i], root, "sys/policies/acl/jwt-test", map[string]any{"policy": `path "sys/policies/acl/jwt-test" { capabilities = ["read"] }`})
		writeJWTTestAPI(t, addresses[i], root, "auth/jwt-operator/role/openbao-operator", map[string]any{
			"role_type": "jwt", "user_claim": "sub", "bound_audiences": []string{audience},
			"bound_subject": "system:serviceaccount:" + pod.Namespace + ":controller", "token_policies": []string{"jwt-test"}, "token_no_default_policy": true,
		})
	}
	admin, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err)
	legacy, err := admin.CoreV1().ServiceAccounts(pod.Namespace).CreateToken(ctx, "controller", &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{Audiences: []string{"openbao-internal"}, ExpirationSeconds: ptr.To(int64(600))}}, metav1.CreateOptions{})
	require.NoError(t, err)
	for _, strategy := range []string{portopenbao.JWTAuthStrategyInline, portopenbao.JWTAuthStrategyStandard} {
		t.Run(strategy, func(t *testing.T) {
			manager := adapteropenbao.NewClientManager(portopenbao.ClientConfig{JWTAuthStrategy: strategy})
			defer manager.Close()
			for i, address := range addresses {
				factory := manager.FactoryFor(string(targets[i].UID), nil)
				for _, test := range []struct {
					token string
					valid bool
				}{{tokens[i], true}, {tokens[1-i], false}, {legacy.Status.Token, false}} {
					client, err := factory.NewWithJWT(ctx, address, portauth.RoleNameOperator, test.token)
					if err == nil {
						_, err = client.ReadACLPolicy(ctx, "jwt-test")
					}
					if test.valid {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
						require.True(t, portopenbao.IsStatus(err, 400) || portopenbao.IsStatus(err, 403), "unexpected error: %v", err)
					}
				}
			}
		})
	}
}

func writeJWTTestAPI(t *testing.T, address, root, path string, payload map[string]any) {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address+"/v1/"+path, bytes.NewReader(data))
	require.NoError(t, err)
	request.Header.Set("X-Vault-Token", root)
	request.Header.Set("Content-Type", "application/json")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	response, err := httpClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.True(t, response.StatusCode >= 200 && response.StatusCode < 300, "configure %s: %d %s", path, response.StatusCode, body)
}

func startControllerJWTTestServer(t *testing.T, image, root string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", "run", "--rm", "-d", "-p", "127.0.0.1::8200",
		"-e", "BAO_DEV_ROOT_TOKEN_ID", "-e", "SKIP_SETCAP=true", image,
		"server", "-dev", "-dev-listen-address=0.0.0.0:8200")
	command.Env = append(os.Environ(), "BAO_DEV_ROOT_TOKEN_ID="+root)
	output, err := command.Output()
	require.NoError(t, err)
	id := strings.TrimSpace(string(output))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "docker", "rm", "-f", id).Run(); err != nil {
			t.Errorf("remove JWT test container: %v", err)
		}
	})
	output, err = exec.CommandContext(ctx, "docker", "port", id, "8200/tcp").Output()
	require.NoError(t, err)
	address := "http://" + strings.TrimSpace(string(output))
	httpClient := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		response, err := httpClient.Get(address + "/v1/sys/health")
		if err != nil {
			return false
		}
		defer response.Body.Close()
		return response.StatusCode == http.StatusOK
	}, 30*time.Second, 200*time.Millisecond)
	return address
}
