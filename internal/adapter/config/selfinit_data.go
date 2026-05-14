package config

import (
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/zclconf/go-cty/cty"
)

// buildAuditDeviceData builds the API request data for an audit device from structured config.
func buildAuditDeviceData(device *openbaov1alpha1.SelfInitAuditDevice) (cty.Value, error) {
	if err := validateSelfInitAuditDevice(device); err != nil {
		return cty.NilVal, err
	}

	var optionsMap map[string]cty.Value

	auditType := strings.TrimSpace(device.Type)
	switch auditType {
	case auditTypeFile:
		if device.FileOptions == nil {
			return cty.NilVal, fmt.Errorf("fileOptions is required for file audit device")
		}
		optionsMap = buildFileAuditOptions(device.FileOptions)
	case auditTypeHTTP:
		if device.HTTPOptions == nil {
			return cty.NilVal, fmt.Errorf("httpOptions is required for http audit device")
		}
		httpOptions, err := buildHTTPAuditOptions(device.HTTPOptions, selfInitAuditHeadersContext)
		if err != nil {
			return cty.NilVal, err
		}
		optionsMap = httpOptions
	case auditTypeSyslog:
		if device.SyslogOptions != nil {
			optionsMap = buildSyslogAuditOptions(device.SyslogOptions)
		}
	case auditTypeSocket:
		if device.SocketOptions != nil {
			optionsMap = buildSocketAuditOptions(device.SocketOptions)
		}
	default:
		return cty.NilVal, fmt.Errorf("unsupported audit device type: %s", auditType)
	}

	dataMap := map[string]cty.Value{
		"type": cty.StringVal(auditType),
	}
	if device.Description != "" {
		dataMap["description"] = cty.StringVal(device.Description)
	}
	if len(optionsMap) > 0 {
		dataMap["options"] = cty.ObjectVal(optionsMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildAuthMethodData builds the API request data for an auth method from structured config.
func buildAuthMethodData(authMethod *openbaov1alpha1.SelfInitAuthMethod) (cty.Value, error) {
	if authMethod == nil {
		return cty.NilVal, fmt.Errorf("auth method config is nil")
	}

	dataMap := map[string]cty.Value{
		"type": cty.StringVal(authMethod.Type),
	}

	if authMethod.Description != "" {
		dataMap["description"] = cty.StringVal(authMethod.Description)
	}

	if len(authMethod.Config) > 0 {
		configMap := make(map[string]cty.Value, len(authMethod.Config))
		for k, v := range authMethod.Config {
			configMap[k] = cty.StringVal(v)
		}
		dataMap["config"] = cty.ObjectVal(configMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildSecretEngineData builds the API request data for a secret engine from structured config.
func buildSecretEngineData(secretEngine *openbaov1alpha1.SelfInitSecretEngine) (cty.Value, error) {
	if secretEngine == nil {
		return cty.NilVal, fmt.Errorf("secret engine config is nil")
	}

	dataMap := map[string]cty.Value{
		"type": cty.StringVal(secretEngine.Type),
	}

	if secretEngine.Description != "" {
		dataMap["description"] = cty.StringVal(secretEngine.Description)
	}

	if len(secretEngine.Options) > 0 {
		optionsMap := make(map[string]cty.Value, len(secretEngine.Options))
		for k, v := range secretEngine.Options {
			optionsMap[k] = cty.StringVal(v)
		}
		dataMap["options"] = cty.ObjectVal(optionsMap)
	}

	return cty.ObjectVal(dataMap), nil
}

// buildPolicyData builds the API request data for a policy from structured config.
func buildPolicyData(policy *openbaov1alpha1.SelfInitPolicy) (cty.Value, error) {
	if policy == nil {
		return cty.NilVal, fmt.Errorf("policy config is nil")
	}
	if policy.Policy == "" {
		return cty.NilVal, fmt.Errorf("policy content is required")
	}

	return cty.ObjectVal(map[string]cty.Value{
		"policy": cty.StringVal(policy.Policy),
	}), nil
}
