package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	selfInitRefKindConfigMap = "ConfigMap"
	selfInitRefKindSecret    = "Secret"
)

type selfInitAuditSinkRefContent struct {
	Path          string                              `json:"path,omitempty"`
	Description   string                              `json:"description,omitempty"`
	FileOptions   *openbaov1alpha1.FileAuditOptions   `json:"fileOptions,omitempty"`
	HTTPOptions   *openbaov1alpha1.HTTPAuditOptions   `json:"httpOptions,omitempty"`
	SyslogOptions *openbaov1alpha1.SyslogAuditOptions `json:"syslogOptions,omitempty"`
	SocketOptions *openbaov1alpha1.SocketAuditOptions `json:"socketOptions,omitempty"`
}

func (m *Manager) resolveSelfInitCluster(cluster *openbaov1alpha1.OpenBaoCluster) *openbaov1alpha1.OpenBaoCluster {
	if cluster == nil {
		return nil
	}
	return cluster.DeepCopy()
}

func (m *Manager) resolveSelfInitRefs(
	ctx context.Context,
	cluster *openbaov1alpha1.OpenBaoCluster,
) (*openbaov1alpha1.OpenBaoCluster, error) {
	resolved := m.resolveSelfInitCluster(cluster)
	if resolved == nil || resolved.Spec.SelfInit == nil || !resolved.Spec.SelfInit.Enabled {
		return resolved, nil
	}

	for i := range resolved.Spec.SelfInit.Requests {
		if err := m.resolveSelfInitRequest(ctx, resolved.Namespace, &resolved.Spec.SelfInit.Requests[i]); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}

func (m *Manager) resolveSelfInitRequest(
	ctx context.Context,
	namespace string,
	req *openbaov1alpha1.SelfInitRequest,
) error {
	if req == nil {
		return nil
	}
	if req.AuthMethod != nil && req.AuthMethod.ConfigFromRef != nil {
		config, err := m.resolveSelfInitStringMapRef(ctx, namespace, req.AuthMethod.ConfigFromRef, "auth-method config")
		if err != nil {
			return fmt.Errorf("resolve self-init auth-method config ref for request %q: %w", req.Name, err)
		}
		req.AuthMethod.Config = config
		req.AuthMethod.ConfigFromRef = nil
	}
	if req.Policy != nil && req.Policy.ContentFromRef != nil {
		content, err := m.resolveSelfInitSingleValueRef(ctx, namespace, req.Policy.ContentFromRef, "policy content")
		if err != nil {
			return fmt.Errorf("resolve self-init policy content ref for request %q: %w", req.Name, err)
		}
		req.Policy.Policy = content
		req.Policy.ContentFromRef = nil
	}
	if req.AuditDevice != nil && req.AuditDevice.SinkFromRef != nil {
		sink, err := m.resolveSelfInitAuditSinkRef(ctx, namespace, req.Path, req.AuditDevice.SinkFromRef)
		if err != nil {
			return fmt.Errorf("resolve self-init audit sink ref for request %q: %w", req.Name, err)
		}
		req.AuditDevice.Description = sink.Description
		req.AuditDevice.FileOptions = sink.FileOptions.DeepCopy()
		req.AuditDevice.HTTPOptions = sink.HTTPOptions.DeepCopy()
		req.AuditDevice.SyslogOptions = sink.SyslogOptions.DeepCopy()
		req.AuditDevice.SocketOptions = sink.SocketOptions.DeepCopy()
		req.AuditDevice.SinkFromRef = nil
	}

	return nil
}

func (m *Manager) resolveSelfInitAuditSinkRef(
	ctx context.Context,
	namespace string,
	requestPath string,
	ref *openbaov1alpha1.TypedObjectReference,
) (*selfInitAuditSinkRefContent, error) {
	raw, err := m.resolveSelfInitSingleValueRef(ctx, namespace, ref, "audit sink")
	if err != nil {
		return nil, err
	}

	sink := &selfInitAuditSinkRefContent{}
	if err := json.Unmarshal([]byte(raw), sink); err != nil {
		return nil, fmt.Errorf("audit sink must contain valid JSON content: %w", err)
	}

	if expectedPath, ok := selfInitAuditRequestPath(requestPath); ok && strings.TrimSpace(sink.Path) != "" {
		if strings.Trim(strings.TrimSpace(sink.Path), "/") != expectedPath {
			return nil, fmt.Errorf("audit sink path %q does not match request path %q", sink.Path, requestPath)
		}
	}

	return sink, nil
}

func selfInitAuditRequestPath(requestPath string) (string, bool) {
	const prefix = "sys/audit/"
	if !strings.HasPrefix(requestPath, prefix) {
		return "", false
	}
	return strings.Trim(strings.TrimPrefix(requestPath, prefix), "/"), true
}

func (m *Manager) resolveSelfInitStringMapRef(
	ctx context.Context,
	namespace string,
	ref *openbaov1alpha1.TypedObjectReference,
	purpose string,
) (map[string]string, error) {
	switch strings.TrimSpace(ref.Kind) {
	case selfInitRefKindConfigMap:
		configMap := &corev1.ConfigMap{}
		if err := m.getLocalSelfInitRefObject(ctx, namespace, ref, configMap, purpose); err != nil {
			return nil, err
		}
		if len(configMap.Data) == 0 {
			return nil, fmt.Errorf("%s ConfigMap must contain string data", purpose)
		}
		resolved := make(map[string]string, len(configMap.Data))
		for key, value := range configMap.Data {
			resolved[key] = value
		}
		return resolved, nil
	case selfInitRefKindSecret:
		secret := &corev1.Secret{}
		if err := m.getLocalSelfInitRefObject(ctx, namespace, ref, secret, purpose); err != nil {
			return nil, err
		}
		if len(secret.Data) == 0 {
			return nil, fmt.Errorf("%s Secret must contain data", purpose)
		}
		resolved := make(map[string]string, len(secret.Data))
		for key, value := range secret.Data {
			resolved[key] = string(value)
		}
		return resolved, nil
	default:
		return nil, fmt.Errorf("%s refs support only ConfigMap or Secret objects", purpose)
	}
}

func (m *Manager) resolveSelfInitSingleValueRef(
	ctx context.Context,
	namespace string,
	ref *openbaov1alpha1.TypedObjectReference,
	purpose string,
) (string, error) {
	switch strings.TrimSpace(ref.Kind) {
	case selfInitRefKindConfigMap:
		configMap := &corev1.ConfigMap{}
		if err := m.getLocalSelfInitRefObject(ctx, namespace, ref, configMap, purpose); err != nil {
			return "", err
		}
		if len(configMap.Data) != 1 {
			return "", fmt.Errorf("%s ConfigMap must contain exactly one string data entry", purpose)
		}
		for _, value := range configMap.Data {
			if strings.TrimSpace(value) == "" {
				return "", fmt.Errorf("%s ConfigMap content must be non-empty", purpose)
			}
			return value, nil
		}
	case selfInitRefKindSecret:
		secret := &corev1.Secret{}
		if err := m.getLocalSelfInitRefObject(ctx, namespace, ref, secret, purpose); err != nil {
			return "", err
		}
		if len(secret.Data) != 1 {
			return "", fmt.Errorf("%s Secret must contain exactly one data entry", purpose)
		}
		for _, value := range secret.Data {
			if strings.TrimSpace(string(value)) == "" {
				return "", fmt.Errorf("%s Secret content must be non-empty", purpose)
			}
			return string(value), nil
		}
	default:
		return "", fmt.Errorf("%s refs support only ConfigMap or Secret objects", purpose)
	}

	return "", fmt.Errorf("%s content could not be resolved", purpose)
}

func (m *Manager) getLocalSelfInitRefObject(
	ctx context.Context,
	namespace string,
	ref *openbaov1alpha1.TypedObjectReference,
	obj client.Object,
	purpose string,
) error {
	if ref == nil {
		return fmt.Errorf("%s ref is required", purpose)
	}
	if strings.TrimSpace(ref.Name) == "" {
		return fmt.Errorf("%s ref name is required", purpose)
	}
	if strings.TrimSpace(ref.Namespace) != "" {
		return fmt.Errorf("%s refs must omit namespace and resolve in the cluster namespace", purpose)
	}

	key := types.NamespacedName{Namespace: namespace, Name: ref.Name}
	if err := m.reader.Get(ctx, key, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%s ref %s/%s was not found", purpose, key.Namespace, key.Name)
		}
		return fmt.Errorf("load %s ref %s/%s: %w", purpose, key.Namespace, key.Name, err)
	}
	return nil
}
