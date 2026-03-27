package infra

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func kmipSecretFileRefs(cfg *openbaov1alpha1.KMIPSealConfig) ([]secretFileRef, error) {
	if cfg == nil {
		return nil, nil
	}

	return secretFileRefsForPaths([]struct {
		fieldName string
		path      string
	}{
		{fieldName: "certificate", path: cfg.ClientCert},
		{fieldName: "key", path: cfg.ClientKey},
		{fieldName: "caCert", path: cfg.CACert},
	}, "spec.unseal.kmip")
}

func secretFileRefsForPaths(candidates []struct {
	fieldName string
	path      string
}, fieldPrefix string) ([]secretFileRef, error) {
	out := make([]secretFileRef, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}

		key, ok := mountedSealCredentialsKey(candidate.path)
		if !ok {
			if strings.HasPrefix(strings.TrimSpace(candidate.path), sealCredsVolumeMountPath) {
				return nil, fmt.Errorf("%s.%s must reference a file under %s", fieldPrefix, candidate.fieldName, sealCredsVolumeMountPath)
			}
			continue
		}

		out = append(out, secretFileRef{
			fieldName: candidate.fieldName,
			key:       key,
		})
	}

	return out, nil
}

func secretDataForFileRefs(
	secretData map[string][]byte,
	fileRefs []secretFileRef,
	cluster *openbaov1alpha1.OpenBaoCluster,
	secretName string,
	secretDescription string,
) (map[string][]byte, error) {
	out := make(map[string][]byte, len(fileRefs))
	for _, ref := range fileRefs {
		data, ok := secretData[ref.key]
		if !ok || len(strings.TrimSpace(string(data))) == 0 {
			return nil, fmt.Errorf(
				"%s Secret %s/%s is missing required key %q",
				secretDescription,
				cluster.Namespace,
				secretName,
				ref.key,
			)
		}
		out[ref.fieldName] = data
	}
	return out, nil
}

func requireSecretKeys(secretData map[string][]byte, namespace, secretName, description string, keys ...string) error {
	missing := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(strings.TrimSpace(string(secretData[key]))) == 0 {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%s Secret %s/%s is missing required keys %s", description, namespace, secretName, strings.Join(missing, ", "))
}

func requireSecretKeysTogether(secretData map[string][]byte, namespace, secretName, description, firstKey, secondKey string) error {
	first := strings.TrimSpace(string(secretData[firstKey]))
	second := strings.TrimSpace(string(secretData[secondKey]))
	switch {
	case first != "" && second != "":
		return nil
	case first == "" && second == "":
		return fmt.Errorf("%s Secret %s/%s must contain both %s and %s", description, namespace, secretName, firstKey, secondKey)
	default:
		return fmt.Errorf("%s Secret %s/%s must contain both %s and %s", description, namespace, secretName, firstKey, secondKey)
	}
}

func parseOCIKMSDefaultProfile(data []byte) (map[string]string, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("config is empty")
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	inDefaultProfile := false
	sawDefaultProfile := false
	fields := map[string]string{}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			profile := strings.TrimSpace(line[1 : len(line)-1])
			inDefaultProfile = strings.EqualFold(profile, "DEFAULT")
			if inDefaultProfile {
				sawDefaultProfile = true
			}
			continue
		}

		if !inDefaultProfile {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line %q in profile [DEFAULT]", line)
		}
		fields[strings.TrimSpace(strings.ToLower(key))] = strings.TrimSpace(value)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !sawDefaultProfile {
		return nil, fmt.Errorf("missing profile [DEFAULT]")
	}

	return fields, nil
}

func (m *Manager) credentialsSecret(ctx context.Context, cluster *openbaov1alpha1.OpenBaoCluster, provider string) (*corev1.Secret, error) {
	if cluster == nil || cluster.Spec.Unseal == nil || cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil, fmt.Errorf("%s unseal requires spec.unseal.credentialsSecretRef for Secret-backed credentials", provider)
	}

	var secret corev1.Secret
	secretName := types.NamespacedName{Namespace: cluster.Namespace, Name: cluster.Spec.Unseal.CredentialsSecretRef.Name}
	if err := m.reader.Get(ctx, secretName, &secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%s credentials Secret %s/%s not found", provider, cluster.Namespace, cluster.Spec.Unseal.CredentialsSecretRef.Name)
		}
		return nil, fmt.Errorf("failed to read %s credentials Secret %s/%s: %w", provider, cluster.Namespace, cluster.Spec.Unseal.CredentialsSecretRef.Name, err)
	}

	return &secret, nil
}
