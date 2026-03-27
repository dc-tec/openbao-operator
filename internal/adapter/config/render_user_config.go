package config

import (
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func buildUserConfigTokens(config *openbaov1alpha1.OpenBaoConfiguration) hclwrite.Tokens {
	if config == nil {
		return nil
	}

	var attrs hclUserConfigurationAttributes
	attrs.LogLevel = stringPtr(config.LogLevel)
	if config.Logging != nil {
		attrs.LogFormat = stringPtr(config.Logging.Format)
		attrs.LogFile = stringPtr(config.Logging.File)
		attrs.LogRotateDuration = stringPtr(config.Logging.RotateDuration)
		attrs.LogRotateBytes = config.Logging.RotateBytes
		attrs.PIDFile = stringPtr(config.Logging.PIDFile)
		if config.Logging.RotateMaxFiles != nil {
			val := *config.Logging.RotateMaxFiles
			attrs.LogRotateMaxFiles = &val
		}
	}
	if config.Plugin != nil {
		attrs.PluginFileUID = config.Plugin.FileUID
		attrs.PluginFilePerms = stringPtr(config.Plugin.FilePermissions)
		attrs.PluginAutoDownload = config.Plugin.AutoDownload
		attrs.PluginAutoRegister = config.Plugin.AutoRegister
		attrs.PluginDownloadMode = stringPtr(config.Plugin.DownloadBehavior)
	}
	attrs.DefaultLeaseTTL = stringPtr(config.DefaultLeaseTTL)
	attrs.MaxLeaseTTL = stringPtr(config.MaxLeaseTTL)
	attrs.CacheSize = config.CacheSize
	attrs.DisableCache = config.DisableCache
	attrs.DetectDeadlocks = boolPtrString(config.DetectDeadlocks)
	attrs.RawStorageEndpoint = config.RawStorageEndpoint
	attrs.Introspection = config.IntrospectionEndpoint
	attrs.ImpreciseLeaseRoleTracking = config.ImpreciseLeaseRoleTracking
	attrs.UnsafeAllowAPIAuditCreation = config.UnsafeAllowAPIAuditCreation
	attrs.AllowAuditLogPrefixing = config.AllowAuditLogPrefixing
	attrs.EnableResponseHeaderHostname = config.EnableResponseHeaderHostname
	attrs.EnableResponseHeaderRaftNodeID = config.EnableResponseHeaderRaftNodeID

	tmpFile := hclwrite.NewEmptyFile()
	gohcl.EncodeIntoBody(attrs, tmpFile.Body())
	return normalizeTrailingNewline(tmpFile.Body().BuildTokens(nil))
}
