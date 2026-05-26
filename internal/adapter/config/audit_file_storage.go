package config

import (
	"fmt"
	"path"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/dc-tec/openbao-operator/internal/platform/constants"
	portopenbao "github.com/dc-tec/openbao-operator/internal/port/openbao"
)

func validateAuditFileStorageConfiguration(cluster *openbaov1alpha1.OpenBaoCluster) error {
	if !portopenbao.HasAuditFileStorage(cluster) {
		return nil
	}

	mountPath := portopenbao.AuditFileStorageMountPath(cluster)
	if !path.IsAbs(mountPath) || path.Clean(mountPath) == "/" {
		return fmt.Errorf("auditFileStorage.mountPath must be an absolute path and must not be /")
	}
	if auditFileStorageMountPathForbidden(mountPath) {
		return fmt.Errorf("auditFileStorage.mountPath %q must not be /tmp or under OpenBao's data path %q", mountPath, constants.PathData)
	}

	for index, device := range cluster.Spec.Audit {
		if strings.TrimSpace(device.Type) != auditTypeFile {
			continue
		}

		filePath, ok, err := declarativeAuditDeviceFilePath(device)
		if err != nil {
			return fmt.Errorf("audit device %d: %w", index, err)
		}
		if !ok || strings.TrimSpace(filePath) == "" || auditFilePathIsSpecial(filePath) {
			continue
		}
		if err := validateFileAuditPathUnderStorage(cluster, fmt.Sprintf("audit device %d", index), filePath, mountPath); err != nil {
			return err
		}
	}

	if err := validateAuditFileStorageSelfInitConfiguration(cluster, mountPath); err != nil {
		return err
	}

	return nil
}

func validateAuditFileStorageSelfInitConfiguration(cluster *openbaov1alpha1.OpenBaoCluster, mountPath string) error {
	if cluster.Spec.SelfInit == nil || !cluster.Spec.SelfInit.Enabled {
		return nil
	}

	for index, req := range cluster.Spec.SelfInit.Requests {
		if !strings.HasPrefix(strings.TrimSpace(req.Path), "sys/audit/") ||
			req.AuditDevice == nil ||
			strings.TrimSpace(req.AuditDevice.Type) != auditTypeFile {
			continue
		}

		filePath, ok := selfInitAuditDeviceFilePath(req.AuditDevice)
		if !ok || strings.TrimSpace(filePath) == "" || auditFilePathIsSpecial(filePath) {
			continue
		}

		context := fmt.Sprintf("self-init request %d %q", index, req.Name)
		if err := validateFileAuditPathUnderStorage(cluster, context, filePath, mountPath); err != nil {
			return err
		}
	}

	return nil
}

func validateFileAuditPathUnderStorage(cluster *openbaov1alpha1.OpenBaoCluster, context string, filePath string, mountPath string) error {
	if !path.IsAbs(filePath) || !portopenbao.PathUnderAuditFileStorage(cluster, filePath) {
		return fmt.Errorf("%s: file audit path %q must be under auditFileStorage.mountPath %q when spec.auditFileStorage is configured", context, filePath, mountPath)
	}
	return nil
}

func declarativeAuditDeviceFilePath(device openbaov1alpha1.AuditDevice) (string, bool, error) {
	if device.FileOptions != nil {
		return strings.TrimSpace(device.FileOptions.FilePath), true, nil
	}
	if device.Options == nil || len(device.Options.Raw) == 0 {
		return "", false, nil
	}
	options, err := decodeAuditStringOptions(device.Options.Raw, auditDeviceOptionsContext)
	if err != nil {
		return "", false, err
	}
	filePath, ok := options[auditOptionFilePath]
	return strings.TrimSpace(filePath), ok, nil
}

func selfInitAuditDeviceFilePath(device *openbaov1alpha1.SelfInitAuditDevice) (string, bool) {
	if device == nil || device.FileOptions == nil {
		return "", false
	}
	return strings.TrimSpace(device.FileOptions.FilePath), true
}

func auditFilePathIsSpecial(filePath string) bool {
	switch strings.TrimSpace(filePath) {
	case "stdout", "discard":
		return true
	default:
		return false
	}
}

func auditFileStorageMountPathForbidden(mountPath string) bool {
	cleanMount := path.Clean(strings.TrimSpace(mountPath))
	for _, forbidden := range []string{"/tmp", constants.PathData} {
		cleanForbidden := path.Clean(forbidden)
		if cleanMount == cleanForbidden || strings.HasPrefix(cleanMount, cleanForbidden+"/") {
			return true
		}
	}
	return false
}
