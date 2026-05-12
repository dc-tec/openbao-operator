package config

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

func buildAuditDeviceBlocks(devices []openbaov1alpha1.AuditDevice) ([]*hclwrite.Block, error) {
	blocks := make([]*hclwrite.Block, 0, len(devices))
	seenPaths := make(map[string]struct{}, len(devices))
	for index, device := range devices {
		auditType, auditPath, err := validateDeclarativeAuditDevice(index, device)
		if err != nil {
			return nil, err
		}
		if _, ok := seenPaths[auditPath]; ok {
			return nil, fmt.Errorf("audit device %d: duplicate path %q", index, auditPath)
		}
		seenPaths[auditPath] = struct{}{}
		device.Type = auditType
		device.Path = auditPath

		block := gohcl.EncodeAsBlock(hclAuditDevice{
			Type:        auditType,
			Path:        auditPath,
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
	case auditTypeFile:
		if device.FileOptions != nil {
			options = buildFileAuditOptions(device.FileOptions)
		}
	case auditTypeHTTP:
		if device.HTTPOptions != nil {
			httpOptions, err := buildHTTPAuditOptions(device.HTTPOptions, auditHTTPHeadersContext)
			if err != nil {
				return cty.NilVal, false, err
			}
			options = httpOptions
		}
	case auditTypeSyslog:
		if device.SyslogOptions != nil {
			options = buildSyslogAuditOptions(device.SyslogOptions)
		}
	case auditTypeSocket:
		if device.SocketOptions != nil {
			options = buildSocketAuditOptions(device.SocketOptions)
		}
	}

	if len(options) > 0 {
		return cty.ObjectVal(options), true, nil
	}

	if device.Options != nil && len(device.Options.Raw) > 0 {
		rawOptions, err := decodeAuditStringOptions(device.Options.Raw, auditDeviceOptionsContext)
		if err != nil {
			return cty.NilVal, false, err
		}
		if len(rawOptions) == 0 {
			return cty.NilVal, false, nil
		}
		return cty.ObjectVal(auditOptionsToCty(rawOptions)), true, nil
	}

	return cty.NilVal, false, nil
}
