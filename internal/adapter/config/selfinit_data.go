package config

import (
	"fmt"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/zclconf/go-cty/cty"
)

// buildAuditDeviceData builds the API request data for an audit device from structured config.
func buildAuditDeviceData(device *openbaov1alpha1.SelfInitAuditDevice) (cty.Value, error) {
	if device == nil {
		return cty.NilVal, fmt.Errorf("audit device config is nil")
	}

	var optionsMap map[string]cty.Value

	switch device.Type {
	case "file":
		if device.FileOptions == nil {
			return cty.NilVal, fmt.Errorf("fileOptions is required for file audit device")
		}
		optionsMap = map[string]cty.Value{
			"file_path": cty.StringVal(device.FileOptions.FilePath),
		}
		if device.FileOptions.Mode != "" {
			optionsMap["mode"] = cty.StringVal(device.FileOptions.Mode)
		}
	case "http":
		if device.HTTPOptions == nil {
			return cty.NilVal, fmt.Errorf("httpOptions is required for http audit device")
		}
		optionsMap = map[string]cty.Value{
			"uri": cty.StringVal(device.HTTPOptions.URI),
		}
		if device.HTTPOptions.Headers != nil && len(device.HTTPOptions.Headers.Raw) > 0 {
			headersVal, err := decodeJSONToCty(device.HTTPOptions.Headers.Raw, "audit device headers")
			if err != nil {
				return cty.NilVal, err
			}
			optionsMap["headers"] = headersVal
		}
	case "syslog":
		if device.SyslogOptions != nil {
			optionsMap = make(map[string]cty.Value)
			if device.SyslogOptions.Facility != "" {
				optionsMap["facility"] = cty.StringVal(device.SyslogOptions.Facility)
			}
			if device.SyslogOptions.Tag != "" {
				optionsMap["tag"] = cty.StringVal(device.SyslogOptions.Tag)
			}
		}
	case "socket":
		if device.SocketOptions != nil {
			optionsMap = make(map[string]cty.Value)
			if device.SocketOptions.Address != "" {
				optionsMap["address"] = cty.StringVal(device.SocketOptions.Address)
			}
			if device.SocketOptions.SocketType != "" {
				optionsMap["socket_type"] = cty.StringVal(device.SocketOptions.SocketType)
			}
			if device.SocketOptions.WriteTimeout != "" {
				optionsMap["write_timeout"] = cty.StringVal(device.SocketOptions.WriteTimeout)
			}
		}
	default:
		return cty.NilVal, fmt.Errorf("unsupported audit device type: %s", device.Type)
	}

	dataMap := map[string]cty.Value{
		"type": cty.StringVal(device.Type),
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
