package config

import (
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func buildAuditDeviceBlocks(devices []openbaov1alpha1.AuditDevice) ([]*hclwrite.Block, error) {
	blocks := make([]*hclwrite.Block, 0, len(devices))
	for _, device := range devices {
		if device.Type == "" || device.Path == "" {
			continue
		}

		block := gohcl.EncodeAsBlock(hclAuditDevice{
			Type:        device.Type,
			Path:        device.Path,
			Description: stringPtr(device.Description),
		}, "audit")

		optionsVal, ok, err := buildAuditOptionsValue(device)
		if err != nil {
			return nil, err
		}
		if ok {
			block.Body().SetAttributeValue("options", optionsVal)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func buildAuditOptionsValue(device openbaov1alpha1.AuditDevice) (cty.Value, bool, error) {
	var options map[string]cty.Value

	switch device.Type {
	case "file":
		if device.FileOptions != nil {
			options = map[string]cty.Value{
				"file_path": cty.StringVal(device.FileOptions.FilePath),
			}
			if device.FileOptions.Mode != "" {
				options["mode"] = cty.StringVal(device.FileOptions.Mode)
			}
		}
	case "http":
		if device.HTTPOptions != nil {
			options = map[string]cty.Value{
				"uri": cty.StringVal(device.HTTPOptions.URI),
			}
			if device.HTTPOptions.Headers != nil && len(device.HTTPOptions.Headers.Raw) > 0 {
				headersVal, err := decodeJSONToCty(device.HTTPOptions.Headers.Raw, "HTTP audit device headers")
				if err != nil {
					return cty.NilVal, false, err
				}
				options["headers"] = headersVal
			}
		}
	case "syslog":
		if device.SyslogOptions != nil {
			options = make(map[string]cty.Value)
			if device.SyslogOptions.Facility != "" {
				options["facility"] = cty.StringVal(device.SyslogOptions.Facility)
			}
			if device.SyslogOptions.Tag != "" {
				options["tag"] = cty.StringVal(device.SyslogOptions.Tag)
			}
		}
	case "socket":
		if device.SocketOptions != nil {
			options = make(map[string]cty.Value)
			if device.SocketOptions.Address != "" {
				options["address"] = cty.StringVal(device.SocketOptions.Address)
			}
			if device.SocketOptions.SocketType != "" {
				options["socket_type"] = cty.StringVal(device.SocketOptions.SocketType)
			}
			if device.SocketOptions.WriteTimeout != "" {
				options["write_timeout"] = cty.StringVal(device.SocketOptions.WriteTimeout)
			}
		}
	}

	if len(options) > 0 {
		return cty.ObjectVal(options), true, nil
	}

	if device.Options != nil && len(device.Options.Raw) > 0 {
		ctyVal, err := decodeJSONToCty(device.Options.Raw, "audit device options")
		if err != nil {
			return cty.NilVal, false, err
		}
		return ctyVal, true, nil
	}

	return cty.NilVal, false, nil
}
