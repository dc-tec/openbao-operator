package security

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// buildKeychain constructs a keychain from ImagePullSecrets.
func (v *ImageVerifier) buildKeychain(ctx context.Context, imagePullSecrets []corev1.LocalObjectReference, namespace string) (authn.Keychain, error) {
	if len(imagePullSecrets) == 0 || v.client == nil {
		return nil, nil
	}

	type dockerConfig struct {
		Auths map[string]dockerAuthConfig `json:"auths"`
	}

	combinedConfig := dockerConfig{Auths: make(map[string]dockerAuthConfig)}
	for _, secretRef := range imagePullSecrets {
		secret := &corev1.Secret{}
		if err := v.client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      secretRef.Name,
		}, secret); err != nil {
			return nil, fmt.Errorf("failed to get ImagePullSecret %s/%s: %w", namespace, secretRef.Name, err)
		}

		if secret.Type != corev1.SecretTypeDockerConfigJson && secret.Type != corev1.SecretTypeDockercfg {
			return nil, fmt.Errorf("ImagePullSecret %s/%s has invalid type %s, expected %s or %s",
				namespace, secretRef.Name, secret.Type, corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg)
		}

		dockerConfigKey := corev1.DockerConfigKey
		if secret.Type == corev1.SecretTypeDockerConfigJson {
			dockerConfigKey = corev1.DockerConfigJsonKey
		}

		dockerConfigData, ok := secret.Data[dockerConfigKey]
		if !ok {
			return nil, fmt.Errorf("ImagePullSecret %s/%s missing key %s", namespace, secretRef.Name, dockerConfigKey)
		}

		var secretConfig dockerConfig
		if err := json.Unmarshal(dockerConfigData, &secretConfig); err != nil {
			return nil, fmt.Errorf("failed to parse docker config from ImagePullSecret %s/%s: %w", namespace, secretRef.Name, err)
		}
		for registry, authConfig := range secretConfig.Auths {
			combinedConfig.Auths[registry] = authConfig
		}
	}

	if len(combinedConfig.Auths) == 0 {
		return nil, nil
	}
	return &dockerConfigKeychain{auths: combinedConfig.Auths}, nil
}

type dockerAuthConfig struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Auth     string `json:"auth,omitempty"`
}

type dockerConfigKeychain struct {
	auths map[string]dockerAuthConfig
}

func (k *dockerConfigKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	registry := resource.RegistryStr()
	if auth, ok := k.auths[registry]; ok {
		if auth.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					return &authn.Basic{Username: parts[0], Password: parts[1]}, nil
				}
			}
		}
		if auth.Username != "" && auth.Password != "" {
			return &authn.Basic{Username: auth.Username, Password: auth.Password}, nil
		}
	}
	return authn.Anonymous, nil
}
