package openbao

const (
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
