package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func runPreflightChecks() error {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv(portopenbao.EnvBaoSealType)), "pkcs11") {
		return nil
	}
	return validatePKCS11Runtime()
}

func validatePKCS11Runtime() error {
	lib := strings.TrimSpace(os.Getenv(portopenbao.EnvBaoHSMLib))
	if lib == "" {
		return nil
	}

	cleanLib := filepath.Clean(lib)
	info, err := os.Stat(cleanLib)
	if err != nil {
		return fmt.Errorf("PKCS#11 library %q is not accessible: %w", lib, err)
	}
	if info.IsDir() {
		return fmt.Errorf("PKCS#11 library %q is a directory, expected a file", lib)
	}
	return nil
}
