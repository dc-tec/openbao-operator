package workload

import (
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	"github.com/dc-tec/openbao-operator/internal/platform/resourceidentity"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func TestSealWiring_StaticDefault_MountsUnseal(t *testing.T) {
	cluster := newMinimalCluster("seal-static-default", "default")

	env := buildContainerEnv(cluster)
	mounts := buildContainerVolumeMounts(cluster, path.Dir(openBaoRenderedConfig))
	volumes := buildStatefulSetVolumes(cluster, StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter})

	if hasVolume(volumes, sealCredsVolumeName) {
		t.Fatalf("expected %q volume to be absent for static seal", sealCredsVolumeName)
	}
	if hasVolumeMount(mounts, sealCredsVolumeName) {
		t.Fatalf("expected %q volume mount to be absent for static seal", sealCredsVolumeName)
	}
	if hasEnvVar(env, "VAULT_TOKEN") || hasEnvVar(env, "VAULT_CACERT") || hasEnvVar(env, "GOOGLE_APPLICATION_CREDENTIALS") {
		t.Fatalf("expected no external-seal env vars for static seal")
	}

	unsealVol, ok := getVolume(volumes, unsealVolumeName)
	if !ok {
		t.Fatalf("expected %q volume to be present for static seal", unsealVolumeName)
	}
	if unsealVol.Secret == nil || unsealVol.Secret.SecretName != resourceidentity.UnsealSecretName(cluster) {
		t.Fatalf("expected %q volume to use secret %q", unsealVolumeName, resourceidentity.UnsealSecretName(cluster))
	}
	if !hasVolumeMountWithPath(mounts, unsealVolumeName, openBaoUnsealMountPath) {
		t.Fatalf("expected %q volume mount at %q for static seal", unsealVolumeName, openBaoUnsealMountPath)
	}
}

func TestSealWiring_ExternalTypes_WithCredentials_MountsSealCredsAndEnv(t *testing.T) {
	cases := []struct {
		name         string
		unsealType   string
		expectEnvVar []string
	}{
		{name: "transit", unsealType: "transit", expectEnvVar: []string{"VAULT_TOKEN"}},
		{name: "gcpckms", unsealType: "gcpckms", expectEnvVar: []string{"GOOGLE_APPLICATION_CREDENTIALS"}},
		{name: "awskms", unsealType: "awskms", expectEnvVar: []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN"}},
		{name: "azurekeyvault", unsealType: "azurekeyvault", expectEnvVar: []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_ENVIRONMENT", "AZURE_AD_RESOURCE"}},
		{name: "kmip", unsealType: "kmip"},
		{name: "ocikms", unsealType: "ocikms"},
		{name: "pkcs11", unsealType: portopenbao.SealTypePKCS11, expectEnvVar: []string{"BAO_HSM_PIN"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newMinimalCluster("seal-"+tc.name, "default")
			cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
				Type: tc.unsealType,
				CredentialsSecretRef: &corev1.LocalObjectReference{
					Name: "provider-creds",
				},
			}
			if tc.unsealType == portopenbao.SealTypePKCS11 {
				cluster.Spec.Unseal.PKCS11 = &openbaov1alpha1.PKCS11SealConfig{
					Lib:        "/usr/local/lib/libpkcs11.so",
					TokenLabel: "OpenBao",
					KeyLabel:   "bao-root-key-aes",
				}
			}

			env := buildContainerEnv(cluster)
			mounts := buildContainerVolumeMounts(cluster, path.Dir(openBaoRenderedConfig))
			volumes := buildStatefulSetVolumes(cluster, StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter})

			if hasVolume(volumes, unsealVolumeName) {
				t.Fatalf("expected %q volume to be absent for external seal type %q", unsealVolumeName, tc.unsealType)
			}
			if hasVolumeMount(mounts, unsealVolumeName) {
				t.Fatalf("expected %q volume mount to be absent for external seal type %q", unsealVolumeName, tc.unsealType)
			}

			sealCredsVol, ok := getVolume(volumes, sealCredsVolumeName)
			if !ok {
				t.Fatalf("expected %q volume to be present when credentialsSecretRef is set", sealCredsVolumeName)
			}
			if sealCredsVol.Secret == nil || sealCredsVol.Secret.SecretName != "provider-creds" {
				t.Fatalf("expected %q volume to use secret %q", sealCredsVolumeName, "provider-creds")
			}
			if !hasVolumeMountWithPath(mounts, sealCredsVolumeName, sealCredsVolumeMountPath) {
				t.Fatalf("expected %q volume mount at %q when credentialsSecretRef is set", sealCredsVolumeName, sealCredsVolumeMountPath)
			}

			for _, envName := range tc.expectEnvVar {
				if !hasEnvVar(env, envName) {
					t.Fatalf("expected env var %q for seal type %q", envName, tc.unsealType)
				}
			}

			if tc.unsealType == "transit" {
				vaultToken := findEnvVar(env, "VAULT_TOKEN")
				if vaultToken == nil || vaultToken.ValueFrom == nil || vaultToken.ValueFrom.SecretKeyRef == nil {
					t.Fatalf("expected VAULT_TOKEN to come from SecretKeyRef for transit seal")
				}
				if vaultToken.ValueFrom.SecretKeyRef.Name != "provider-creds" || vaultToken.ValueFrom.SecretKeyRef.Key != "token" {
					t.Fatalf("expected VAULT_TOKEN SecretKeyRef to be %q/%q, got %q/%q", "provider-creds", "token", vaultToken.ValueFrom.SecretKeyRef.Name, vaultToken.ValueFrom.SecretKeyRef.Key)
				}
			}
		})
	}
}

func TestSealWiring_ExternalTypes_WithoutCredentials_DoesNotMountSealCredsOrEnv(t *testing.T) {
	types := []string{"transit", "gcpckms", "awskms", "azurekeyvault", "kmip", "ocikms", portopenbao.SealTypePKCS11}

	for _, unsealType := range types {
		t.Run(unsealType, func(t *testing.T) {
			cluster := newMinimalCluster("seal-"+unsealType, "default")
			cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{Type: unsealType}

			env := buildContainerEnv(cluster)
			mounts := buildContainerVolumeMounts(cluster, path.Dir(openBaoRenderedConfig))
			volumes := buildStatefulSetVolumes(cluster, StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter})

			if hasVolume(volumes, sealCredsVolumeName) || hasVolumeMount(mounts, sealCredsVolumeName) {
				t.Fatalf("expected %q volume/mount to be absent when credentialsSecretRef is not set", sealCredsVolumeName)
			}
			if hasEnvVar(env, "VAULT_TOKEN") || hasEnvVar(env, "VAULT_CACERT") || hasEnvVar(env, "GOOGLE_APPLICATION_CREDENTIALS") {
				t.Fatalf("expected no credentials-derived env vars when credentialsSecretRef is not set")
			}
		})
	}
}

func TestSealWiring_TransitInlineToken_DoesNotInjectVaultTokenEnv(t *testing.T) {
	cluster := newMinimalCluster("seal-transit-inline-token", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "transit",
		CredentialsSecretRef: &corev1.LocalObjectReference{
			Name: "provider-creds",
		},
		Transit: &openbaov1alpha1.TransitSealConfig{
			Token: "inline-token",
		},
	}

	env := buildContainerEnv(cluster)
	if hasEnvVar(env, "VAULT_TOKEN") {
		t.Fatalf("expected VAULT_TOKEN to be absent when transit token is configured inline")
	}
}

func TestSealWiring_OCIKMSAPIKey_WithCredentials_IncludesOCIConfigEnv(t *testing.T) {
	cluster := newMinimalCluster("seal-ocikms-api-key", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: "ocikms",
		CredentialsSecretRef: &corev1.LocalObjectReference{
			Name: "provider-creds",
		},
		OCIKMS: &openbaov1alpha1.OCIKMSSealConfig{
			AuthTypeAPIKey: ptr.To(true),
		},
	}

	env := buildContainerEnv(cluster)
	configEnv := findEnvVar(env, "OCI_CONFIG_FILE")
	if configEnv == nil {
		t.Fatal("expected OCI_CONFIG_FILE env var for ocikms api-key mode")
	}
	if configEnv.Value != sealCredsVolumeMountPath+"/config" {
		t.Fatalf("OCI_CONFIG_FILE = %q, want %q", configEnv.Value, sealCredsVolumeMountPath+"/config")
	}
}

func TestSealWiring_PKCS11RuntimeEnv(t *testing.T) {
	cluster := newMinimalCluster("seal-pkcs11-runtime", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: portopenbao.SealTypePKCS11,
		CredentialsSecretRef: &corev1.LocalObjectReference{
			Name: "pkcs11-creds",
		},
		PKCS11: &openbaov1alpha1.PKCS11SealConfig{
			Lib:        "/usr/local/lib/libpkcs11.so",
			TokenLabel: "OpenBao",
			KeyLabel:   "bao-root-key-aes",
			Mechanism:  "AES_GCM",
			Runtime: &openbaov1alpha1.PKCS11RuntimeConfig{
				LibraryPath: "/usr/local/lib",
				Env: []openbaov1alpha1.PKCS11RuntimeEnvVar{
					{Name: "CRYPTOSERVER", SecretKey: "cryptoserver"},
				},
				FileEnv: []openbaov1alpha1.PKCS11RuntimeFileEnvVar{
					{Name: "CS_PKCS11_R3_CFG", SecretKey: "cs_pkcs11_R3.cfg"},
				},
			},
		},
	}

	env := buildContainerEnv(cluster)

	for name, want := range map[string]string{
		"BAO_SEAL_TYPE":       portopenbao.SealTypePKCS11,
		"BAO_HSM_LIB":         "/usr/local/lib/libpkcs11.so",
		"BAO_HSM_TOKEN_LABEL": "OpenBao",
		"BAO_HSM_KEY_LABEL":   "bao-root-key-aes",
		"BAO_HSM_MECHANISM":   "AES_GCM",
		"LD_LIBRARY_PATH":     "/usr/local/lib",
		"CS_PKCS11_R3_CFG":    "/etc/bao/seal-creds/cs_pkcs11_R3.cfg",
	} {
		got := findEnvVar(env, name)
		if got == nil || got.Value != want {
			t.Fatalf("%s = %v, want %q", name, got, want)
		}
	}

	cryptoServer := findEnvVar(env, "CRYPTOSERVER")
	if cryptoServer == nil || cryptoServer.ValueFrom == nil || cryptoServer.ValueFrom.SecretKeyRef == nil {
		t.Fatal("expected CRYPTOSERVER to come from SecretKeyRef")
	}
	if cryptoServer.ValueFrom.SecretKeyRef.Name != "pkcs11-creds" || cryptoServer.ValueFrom.SecretKeyRef.Key != "cryptoserver" {
		t.Fatalf("CRYPTOSERVER SecretKeyRef = %s/%s, want pkcs11-creds/cryptoserver", cryptoServer.ValueFrom.SecretKeyRef.Name, cryptoServer.ValueFrom.SecretKeyRef.Key)
	}

	hsmPIN := findEnvVar(env, "BAO_HSM_PIN")
	if hsmPIN == nil || hsmPIN.ValueFrom == nil || hsmPIN.ValueFrom.SecretKeyRef == nil {
		t.Fatal("expected BAO_HSM_PIN to come from SecretKeyRef")
	}
}

func TestSealWiring_PKCS11InlinePINDoesNotInjectHSMPINEnv(t *testing.T) {
	cluster := newMinimalCluster("seal-pkcs11-inline-pin", "default")
	cluster.Spec.Unseal = &openbaov1alpha1.UnsealConfig{
		Type: portopenbao.SealTypePKCS11,
		CredentialsSecretRef: &corev1.LocalObjectReference{
			Name: "pkcs11-creds",
		},
		PKCS11: &openbaov1alpha1.PKCS11SealConfig{
			Lib:        "/usr/local/lib/libpkcs11.so",
			TokenLabel: "OpenBao",
			KeyLabel:   "bao-root-key-aes",
			PIN:        "1234",
		},
	}

	env := buildContainerEnv(cluster)
	if hasEnvVar(env, "BAO_HSM_PIN") {
		t.Fatal("expected BAO_HSM_PIN env var to be absent when pin is configured inline")
	}
}

func TestSealWiring_StaticExplicitAndImplicit_StillMountsUnseal(t *testing.T) {
	cases := []struct {
		name   string
		unseal *openbaov1alpha1.UnsealConfig
	}{
		{name: "explicit-static", unseal: &openbaov1alpha1.UnsealConfig{Type: "static"}},
		{name: "implicit-empty-type", unseal: &openbaov1alpha1.UnsealConfig{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cluster := newMinimalCluster("seal-"+tc.name, "default")
			cluster.Spec.Unseal = tc.unseal

			mounts := buildContainerVolumeMounts(cluster, path.Dir(openBaoRenderedConfig))
			volumes := buildStatefulSetVolumes(cluster, StatefulSetSpec{Pool: constants.LabelValueOpenBaoWorkloadPoolVoter})

			if !hasVolume(volumes, unsealVolumeName) || !hasVolumeMount(mounts, unsealVolumeName) {
				t.Fatalf("expected %q volume and mount for static seal case %q", unsealVolumeName, tc.name)
			}
		})
	}
}

func hasEnvVar(env []corev1.EnvVar, name string) bool {
	return findEnvVar(env, name) != nil
}

func findEnvVar(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}

func hasVolume(volumes []corev1.Volume, name string) bool {
	_, ok := getVolume(volumes, name)
	return ok
}

func getVolume(volumes []corev1.Volume, name string) (*corev1.VolumeSource, bool) {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i].VolumeSource, true
		}
	}
	return nil, false
}

func hasVolumeMount(mounts []corev1.VolumeMount, name string) bool {
	for i := range mounts {
		if mounts[i].Name == name {
			return true
		}
	}
	return false
}

func hasVolumeMountWithPath(mounts []corev1.VolumeMount, name, mountPath string) bool {
	for i := range mounts {
		if mounts[i].Name == name && mounts[i].MountPath == mountPath {
			return true
		}
	}
	return false
}
