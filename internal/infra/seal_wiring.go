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

func newSealWiringProvider(cluster *openbaov1alpha1.OpenBaoCluster) sealWiringProvider {
	if usesStaticSeal(cluster) {
		return &staticSealWiringProvider{cluster: cluster}
	}

	switch cluster.Spec.Unseal.Type {
	case "transit":
		return &transitSealWiringProvider{cluster: cluster}
	case "gcpckms":
		return &gcpCKMSSealWiringProvider{cluster: cluster}
	case "awskms", "azurekeyvault", "kmip", "ocikms", "pkcs11":
		return &externalSealWiringProvider{cluster: cluster}
	default:
		// Preserve current behavior: treat unknown non-static seal types as "external"
		// and only wire credentials Secret (if provided).
		return &externalSealWiringProvider{cluster: cluster}
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

type externalSealWiringProvider struct {
	cluster *openbaov1alpha1.OpenBaoCluster
}

func (p *externalSealWiringProvider) EnvVars() []corev1.EnvVar { return nil }

func (p *externalSealWiringProvider) VolumeMounts() []corev1.VolumeMount {
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

func (p *externalSealWiringProvider) Volumes() []corev1.Volume {
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
	return (&externalSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *transitSealWiringProvider) Volumes() []corev1.Volume {
	return (&externalSealWiringProvider{cluster: p.cluster}).Volumes()
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
	return (&externalSealWiringProvider{cluster: p.cluster}).VolumeMounts()
}

func (p *gcpCKMSSealWiringProvider) Volumes() []corev1.Volume {
	return (&externalSealWiringProvider{cluster: p.cluster}).Volumes()
}
