package openbao

import openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"

const (
	// SealTypePKCS11 is the OpenBao PKCS#11 seal type wire value.
	SealTypePKCS11 = "pkcs11"

	EnvBaoSealType           = "BAO_SEAL_TYPE"
	EnvBaoHSMLib             = "BAO_HSM_LIB"
	EnvBaoHSMSlot            = "BAO_HSM_SLOT"
	EnvBaoHSMTokenLabel      = "BAO_HSM_TOKEN_LABEL"
	EnvBaoHSMPIN             = "BAO_HSM_PIN"
	EnvBaoHSMKeyLabel        = "BAO_HSM_KEY_LABEL"
	EnvBaoHSMDefaultKeyLabel = "BAO_HSM_DEFAULT_KEY_LABEL"
	EnvBaoHSMKeyID           = "BAO_HSM_KEY_ID"
	EnvBaoHSMMechanism       = "BAO_HSM_MECHANISM"
	EnvBaoHSMRSAOAEPHash     = "BAO_HSM_RSA_OAEP_HASH"
	EnvLDLibraryPath         = "LD_LIBRARY_PATH"
)

// PKCS11RuntimeMappingKind identifies the source list for a PKCS#11 runtime
// mapping.
type PKCS11RuntimeMappingKind string

const (
	PKCS11RuntimeMappingEnv     PKCS11RuntimeMappingKind = "runtime.env"
	PKCS11RuntimeMappingFileEnv PKCS11RuntimeMappingKind = "runtime.fileEnv"
)

// PKCS11RuntimeMapping is a normalized runtime env or file-env mapping from
// spec.unseal.pkcs11.runtime.
type PKCS11RuntimeMapping struct {
	Kind      PKCS11RuntimeMappingKind
	Name      string
	SecretKey string
}

// PKCS11RuntimeMappings returns the PKCS#11 runtime env and file-env mappings
// in declaration order.
func PKCS11RuntimeMappings(runtime *openbaov1alpha1.PKCS11RuntimeConfig) []PKCS11RuntimeMapping {
	if runtime == nil {
		return nil
	}

	mappings := make([]PKCS11RuntimeMapping, 0, len(runtime.Env)+len(runtime.FileEnv))
	for _, env := range runtime.Env {
		mappings = append(mappings, PKCS11RuntimeMapping{
			Kind:      PKCS11RuntimeMappingEnv,
			Name:      env.Name,
			SecretKey: env.SecretKey,
		})
	}
	for _, env := range runtime.FileEnv {
		mappings = append(mappings, PKCS11RuntimeMapping{
			Kind:      PKCS11RuntimeMappingFileEnv,
			Name:      env.Name,
			SecretKey: env.SecretKey,
		})
	}
	return mappings
}

// IsPKCS11SealOwnedEnvVar reports whether name is managed by the operator from
// spec.unseal.pkcs11 and must not be supplied through runtime env mappings.
func IsPKCS11SealOwnedEnvVar(name string) bool {
	switch name {
	case EnvBaoSealType,
		EnvBaoHSMLib,
		EnvBaoHSMSlot,
		EnvBaoHSMTokenLabel,
		EnvBaoHSMPIN,
		EnvBaoHSMKeyLabel,
		EnvBaoHSMDefaultKeyLabel,
		EnvBaoHSMKeyID,
		EnvBaoHSMMechanism,
		EnvBaoHSMRSAOAEPHash,
		EnvLDLibraryPath:
		return true
	default:
		return false
	}
}

// IsValidEnvVarName applies the portable environment variable name pattern used
// by Kubernetes and POSIX-style shells.
func IsValidEnvVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
