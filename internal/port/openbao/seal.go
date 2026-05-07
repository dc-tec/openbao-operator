package openbao

const (
	// SealType* constants are OpenBao seal type wire values.
	SealTypeAWSKMS        = "awskms"
	SealTypeAzureKeyVault = "azurekeyvault"
	SealTypeGCPCKMS       = "gcpckms"
	SealTypeKMIP          = "kmip"
	SealTypeOCIKMS        = "ocikms"
	SealTypePKCS11        = "pkcs11"
	SealTypeStatic        = "static"
	SealTypeTransit       = "transit"
)
