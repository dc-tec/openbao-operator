package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portauth "github.com/dc-tec/openbao-operator/internal/port/auth"
)

const controllerJWTRefreshMargin = 60 * time.Second

// ControllerTokenSource issues Pod-bound JWTs for individual targets. Shared
// mode reads the projected file on every call so kubelet rotation takes effect.
type ControllerTokenSource struct {
	clientset kubernetes.Interface
	tokens    *cache.Expiring
	requests  singleflight.Group
	readFile  func(string) ([]byte, error)
}

// NewControllerTokenSource creates a process-local credential cache.
func NewControllerTokenSource(clientset kubernetes.Interface) *ControllerTokenSource {
	return &ControllerTokenSource{clientset: clientset, tokens: cache.NewExpiring(), readFile: os.ReadFile}
}

// Token returns only the credential selected by the cluster. Issuance failures
// in Target mode are returned without reading the shared projected credential.
func (s *ControllerTokenSource) Token(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster) (string, error) {
	audience, err := portauth.ControllerJWTAudience(cluster, OpenBaoJWTAudience())
	if err != nil {
		return "", err
	}
	if cluster.Spec.ControllerJWTMode != openbaov1alpha1.ControllerJWTModeTarget {
		data, err := s.readFile(constants.PathOperatorJWTToken)
		if err != nil {
			return "", fmt.Errorf("read projected OpenBao JWT: %w", err)
		}
		defer clear(data)
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("projected OpenBao JWT is empty")
		}
		return token, nil
	}
	namespace := strings.TrimSpace(os.Getenv("POD_NAMESPACE"))
	serviceAccount := strings.TrimSpace(os.Getenv("OPERATOR_SERVICE_ACCOUNT_NAME"))
	podName := strings.TrimSpace(os.Getenv("POD_NAME"))
	podUID := strings.TrimSpace(os.Getenv("POD_UID"))
	if namespace == "" || serviceAccount == "" || podName == "" || podUID == "" {
		return "", fmt.Errorf("target-specific JWT requires POD_NAMESPACE, OPERATOR_SERVICE_ACCOUNT_NAME, POD_NAME and POD_UID")
	}
	key := namespace + "/" + serviceAccount + "/" + podUID + "/" + audience
	value, err, _ := s.requests.Do(key, func() (any, error) {
		if token, ok := s.tokens.Get(key); ok {
			return token, nil
		}
		return s.issue(ctx, key, namespace, serviceAccount, podName, podUID, audience)
	})
	if err != nil {
		return "", err
	}
	token, ok := value.(string)
	if !ok || token == "" {
		return "", fmt.Errorf("controller JWT issuance returned an empty credential")
	}
	return token, nil
}

func (s *ControllerTokenSource) issue(ctx context.Context, key, namespace, serviceAccount, podName, podUID, audience string) (string, error) {
	if s.clientset == nil {
		return "", fmt.Errorf("target-specific JWT issuance requires a Kubernetes client")
	}
	request := &authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{
		Audiences: []string{audience}, ExpirationSeconds: ptr.To(int64(600)),
		BoundObjectRef: &authenticationv1.BoundObjectReference{APIVersion: "v1", Kind: "Pod", Name: podName, UID: types.UID(podUID)},
	}}
	response, err := s.clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, serviceAccount, request, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("request target-specific controller JWT: %w", err)
	}
	ttl := time.Until(response.Status.ExpirationTimestamp.Time) - controllerJWTRefreshMargin
	if response.Status.Token == "" || ttl <= 0 {
		return "", fmt.Errorf("TokenRequest returned an empty or expiring controller JWT")
	}
	s.tokens.Set(key, response.Status.Token, ttl)
	return response.Status.Token, nil
}
