package infra

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	sealCredsVolumeName      = "seal-creds"
	sealCredsVolumeMountPath = "/etc/bao/seal-creds"
)

type sealWiringProvider interface {
	EnvVars() []corev1.EnvVar
	VolumeMounts() []corev1.VolumeMount
	Volumes() []corev1.Volume
}

func envVarFromCredentialsSecret(cluster *openbaov1alpha1.OpenBaoCluster, envName string, secretKey string) corev1.EnvVar {
	return corev1.EnvVar{
		Name: envName,
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cluster.Spec.Unseal.CredentialsSecretRef.Name,
				},
				Key:      secretKey,
				Optional: ptr.To(true),
			},
		},
	}
}

func newSealWiringProvider(cluster *openbaov1alpha1.OpenBaoCluster) sealWiringProvider {
	if usesStaticSeal(cluster) {
		return &staticSealWiringProvider{cluster: cluster}
	}

	switch cluster.Spec.Unseal.Type {
	case "transit":
		return &transitSealWiringProvider{cluster: cluster}
	case "gcpckms":
		return &gcpCKMSSealWiringProvider{cluster: cluster}
	case "awskms":
		return &awsKMSSealWiringProvider{cluster: cluster}
	case "azurekeyvault":
		return &azureKeyVaultSealWiringProvider{cluster: cluster}
	case "kmip":
		return &kmipSealWiringProvider{cluster: cluster}
	case "ocikms":
		return &ociKMSSealWiringProvider{cluster: cluster}
	case "pkcs11":
		return &pkcs11SealWiringProvider{cluster: cluster}
	default:
		// Preserve current behavior: treat unknown non-static seal types as requiring
		// only credentials Secret wiring (if provided).
		return &credentialsSecretSealWiringProvider{cluster: cluster}
	}
}

type staticSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *staticSealWiringProvider) EnvVars() []corev1.EnvVar { return nil }

func (p *staticSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      unsealVolumeName,
			MountPath: openBaoUnsealMountPath,
			ReadOnly:  true,
		},
	}
}

func (p *staticSealWiringProvider) Volumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: unsealVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  unsealSecretName(p.cluster),
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		},
	}
}

// credentialsSecretSealWiringProvider wires an optional credentials Secret into the
// pod as a volume. Most external seal types don't require additional env wiring.
type credentialsSecretSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *credentialsSecretSealWiringProvider) EnvVars() []corev1.EnvVar { return nil }

func (p *credentialsSecretSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}
	return []corev1.VolumeMount{
		{
			Name:      sealCredsVolumeName,
			MountPath: sealCredsVolumeMountPath,
			ReadOnly:  true,
		},
	}
}

func (p *credentialsSecretSealWiringProvider) Volumes() []corev1.Volume {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}
	return []corev1.Volume{
		{
			Name: sealCredsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  p.cluster.Spec.Unseal.CredentialsSecretRef.Name,
					DefaultMode: ptr.To(secretFileMode),
				},
			},
		},
	}
}

type awsKMSSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *awsKMSSealWiringProvider) EnvVars() []corev1.EnvVar {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}

	return []corev1.EnvVar{
		envVarFromCredentialsSecret(p.cluster, "AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"),
		envVarFromCredentialsSecret(p.cluster, "AWS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY"),
		envVarFromCredentialsSecret(p.cluster, "AWS_SESSION_TOKEN", "AWS_SESSION_TOKEN"),
	}
}

func (p *awsKMSSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *awsKMSSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type azureKeyVaultSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *azureKeyVaultSealWiringProvider) EnvVars() []corev1.EnvVar {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}

	return []corev1.EnvVar{
		envVarFromCredentialsSecret(p.cluster, "AZURE_TENANT_ID", "AZURE_TENANT_ID"),
		envVarFromCredentialsSecret(p.cluster, "AZURE_CLIENT_ID", "AZURE_CLIENT_ID"),
		envVarFromCredentialsSecret(p.cluster, "AZURE_CLIENT_SECRET", "AZURE_CLIENT_SECRET"),
		envVarFromCredentialsSecret(p.cluster, "AZURE_ENVIRONMENT", "AZURE_ENVIRONMENT"),
		envVarFromCredentialsSecret(p.cluster, "AZURE_AD_RESOURCE", "AZURE_AD_RESOURCE"),
	}
}

func (p *azureKeyVaultSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *azureKeyVaultSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type kmipSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *kmipSealWiringProvider) EnvVars() []corev1.EnvVar { return nil }

func (p *kmipSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *kmipSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type ociKMSSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *ociKMSSealWiringProvider) EnvVars() []corev1.EnvVar { return nil }

func (p *ociKMSSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *ociKMSSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type pkcs11SealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *pkcs11SealWiringProvider) EnvVars() []corev1.EnvVar {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}

	return []corev1.EnvVar{
		envVarFromCredentialsSecret(p.cluster, "BAO_HSM_PIN", "BAO_HSM_PIN"),
	}
}

func (p *pkcs11SealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *pkcs11SealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type transitSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *transitSealWiringProvider) EnvVars() []corev1.EnvVar {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}

	// Read token from the mounted secret file and set as VAULT_TOKEN.
	// This allows the seal to use the "token" parameter instead of "token_file",
	// avoiding issues with trailing newlines in mounted Secret files.
	return []corev1.EnvVar{
		{
			Name: "VAULT_TOKEN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Key: "token",
					LocalObjectReference: corev1.LocalObjectReference{
						Name: p.cluster.Spec.Unseal.CredentialsSecretRef.Name,
					},
				},
			},
		},
		{
			// If the credentials Secret provides a CA bundle, surface it via VAULT_CACERT
			// so transit seal HTTP calls verify infra-bao TLS.
			Name:  "VAULT_CACERT",
			Value: sealCredsVolumeMountPath + "/ca.crt",
		},
	}
}

func (p *transitSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *transitSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}

type gcpCKMSSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *gcpCKMSSealWiringProvider) EnvVars() []corev1.EnvVar {
	if p.cluster.Spec.Unseal == nil || p.cluster.Spec.Unseal.CredentialsSecretRef == nil {
		return nil
	}

	// The credentials secret must contain a key named "credentials.json" with the
	// GCP service account JSON credentials. This will be mounted at
	// /etc/bao/seal-creds/credentials.json and referenced by the environment variable.
	return []corev1.EnvVar{
		{
			Name:  "GOOGLE_APPLICATION_CREDENTIALS",
			Value: sealCredsVolumeMountPath + "/credentials.json",
		},
	}
}

func (p *gcpCKMSSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *gcpCKMSSealWiringProvider) Volumes() []corev1.Volume {
	return (&credentialsSecretSealWiringProvider{cluster: p.cluster}).Volumes()
}
