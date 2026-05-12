package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
)

const (
	auditTypeFile   = "file"
	auditTypeHTTP   = "http"
	auditTypeSyslog = "syslog"
	auditTypeSocket = "socket"

	auditOptionFilePath     = "file_path"
	auditOptionMode         = "mode"
	auditOptionURI          = "uri"
	auditOptionHeaders      = "headers"
	auditOptionFacility     = "facility"
	auditOptionTag          = "tag"
	auditOptionAddress      = "address"
	auditOptionSocketType   = "socket_type"
	auditOptionWriteTimeout = "write_timeout"

	auditDeviceContext          = "audit device"
	auditDeviceOptionsContext   = "audit device options"
	auditHTTPHeadersContext     = "HTTP audit device headers"
	selfInitAuditHeadersContext = "audit device headers"
)

type auditOptionFamilies struct {
	file   bool
	http   bool
	syslog bool
	socket bool
}

func auditOptionFamiliesForDevice(device openbaov1alpha1.AuditDevice) auditOptionFamilies {
	return auditOptionFamilies{
		file:   device.FileOptions != nil,
		http:   device.HTTPOptions != nil,
		syslog: device.SyslogOptions != nil,
		socket: device.SocketOptions != nil,
	}
}

func auditOptionFamiliesForSelfInit(device *openbaov1alpha1.SelfInitAuditDevice) auditOptionFamilies {
	return auditOptionFamilies{
		file:   device.FileOptions != nil,
		http:   device.HTTPOptions != nil,
		syslog: device.SyslogOptions != nil,
		socket: device.SocketOptions != nil,
	}
}

func validateDeclarativeAuditDevice(index int, device openbaov1alpha1.AuditDevice) (string, string, error) {
	context := fmt.Sprintf("audit device %d", index)
	auditType := strings.TrimSpace(device.Type)
	auditPath := normalizeAuditPath(device.Path)
	if auditType == "" {
		return "", "", fmt.Errorf("%s: type is required", context)
	}
	if auditPath == "" {
		return "", "", fmt.Errorf("%s: path is required", context)
	}
	if err := validateAuditOptionFamilies(context, auditType, auditOptionFamiliesForDevice(device)); err != nil {
		return "", "", err
	}

	switch auditType {
	case auditTypeFile:
		if device.FileOptions != nil {
			if strings.TrimSpace(device.FileOptions.FilePath) == "" {
				return "", "", fmt.Errorf("%s: fileOptions.filePath is required for file audit devices", context)
			}
		} else {
			hasFilePath, err := rawAuditOptionHasNonEmptyKey(device.Options, auditOptionFilePath)
			if err != nil {
				return "", "", fmt.Errorf("%s: %w", context, err)
			}
			if !hasFilePath {
				return "", "", fmt.Errorf("%s: fileOptions.filePath or options.file_path is required for file audit devices", context)
			}
		}
	case auditTypeHTTP:
		if device.HTTPOptions != nil {
			if strings.TrimSpace(device.HTTPOptions.URI) == "" {
				return "", "", fmt.Errorf("%s: httpOptions.uri is required for http audit devices", context)
			}
		} else {
			hasURI, err := rawAuditOptionHasNonEmptyKey(device.Options, auditOptionURI)
			if err != nil {
				return "", "", fmt.Errorf("%s: %w", context, err)
			}
			if !hasURI {
				return "", "", fmt.Errorf("%s: httpOptions.uri or options.uri is required for http audit devices", context)
			}
		}
	case auditTypeSyslog, auditTypeSocket:
	default:
		return "", "", fmt.Errorf("%s: unsupported audit device type %q", context, auditType)
	}

	return auditType, auditPath, nil
}

func validateSelfInitAuditDevice(device *openbaov1alpha1.SelfInitAuditDevice) error {
	if device == nil {
		return fmt.Errorf("audit device config is nil")
	}
	auditType := strings.TrimSpace(device.Type)
	if auditType == "" {
		return fmt.Errorf("audit device type is required")
	}
	if err := validateAuditOptionFamilies(auditDeviceContext, auditType, auditOptionFamiliesForSelfInit(device)); err != nil {
		return err
	}
	switch auditType {
	case auditTypeFile:
		if device.FileOptions != nil && strings.TrimSpace(device.FileOptions.FilePath) == "" {
			return fmt.Errorf("fileOptions.filePath is required for file audit device")
		}
	case auditTypeHTTP:
		if device.HTTPOptions != nil && strings.TrimSpace(device.HTTPOptions.URI) == "" {
			return fmt.Errorf("httpOptions.uri is required for http audit device")
		}
	}
	return nil
}

func validateAuditOptionFamilies(context, auditType string, families auditOptionFamilies) error {
	switch {
	case auditType != auditTypeFile && families.file:
		return fmt.Errorf("%s: fileOptions is only supported for file audit devices", context)
	case auditType != auditTypeHTTP && families.http:
		return fmt.Errorf("%s: httpOptions is only supported for http audit devices", context)
	case auditType != auditTypeSyslog && families.syslog:
		return fmt.Errorf("%s: syslogOptions is only supported for syslog audit devices", context)
	case auditType != auditTypeSocket && families.socket:
		return fmt.Errorf("%s: socketOptions is only supported for socket audit devices", context)
	default:
		return nil
	}
}

func normalizeAuditPath(path string) string {
	return strings.Trim(strings.TrimSpace(path), "/")
}

func rawAuditOptionHasNonEmptyKey(options *apiextensionsv1.JSON, key string) (bool, error) {
	if options == nil || len(bytes.TrimSpace(options.Raw)) == 0 {
		return false, nil
	}
	decoded, err := decodeAuditStringOptions(options.Raw, auditDeviceOptionsContext)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(decoded[key]) != "", nil
}

func decodeAuditStringOptions(raw []byte, context string) (map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var decoded map[string]any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object with string-compatible scalar values: %w", context, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("%s must contain exactly one JSON object", context)
	}
	if decoded == nil {
		return nil, fmt.Errorf("%s must be a JSON object with string-compatible scalar values", context)
	}

	options := make(map[string]string, len(decoded))
	for key, value := range decoded {
		if strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%s contains an empty option key", context)
		}
		switch typed := value.(type) {
		case string:
			options[key] = typed
		case bool:
			options[key] = strconv.FormatBool(typed)
		case json.Number:
			options[key] = typed.String()
		default:
			return nil, fmt.Errorf("%s option %q must be a string-compatible scalar, got %T", context, key, value)
		}
	}

	return options, nil
}

func auditOptionsToCty(options map[string]string) map[string]cty.Value {
	ctyOptions := make(map[string]cty.Value, len(options))
	for key, value := range options {
		ctyOptions[key] = cty.StringVal(value)
	}
	return ctyOptions
}

func buildFileAuditOptions(options *openbaov1alpha1.FileAuditOptions) map[string]cty.Value {
	rendered := map[string]cty.Value{
		auditOptionFilePath: cty.StringVal(options.FilePath),
	}
	if options.Mode != "" {
		rendered[auditOptionMode] = cty.StringVal(options.Mode)
	}
	return rendered
}

func buildHTTPAuditOptions(options *openbaov1alpha1.HTTPAuditOptions, headersContext string) (map[string]cty.Value, error) {
	rendered := map[string]cty.Value{
		auditOptionURI: cty.StringVal(options.URI),
	}
	if options.Headers != nil && len(options.Headers.Raw) > 0 {
		headers, err := normalizeHTTPAuditHeaders(options.Headers.Raw, headersContext)
		if err != nil {
			return nil, err
		}
		rendered[auditOptionHeaders] = cty.StringVal(headers)
	}
	return rendered, nil
}

func buildSyslogAuditOptions(options *openbaov1alpha1.SyslogAuditOptions) map[string]cty.Value {
	rendered := make(map[string]cty.Value)
	if options.Facility != "" {
		rendered[auditOptionFacility] = cty.StringVal(options.Facility)
	}
	if options.Tag != "" {
		rendered[auditOptionTag] = cty.StringVal(options.Tag)
	}
	return rendered
}

func buildSocketAuditOptions(options *openbaov1alpha1.SocketAuditOptions) map[string]cty.Value {
	rendered := make(map[string]cty.Value)
	if options.Address != "" {
		rendered[auditOptionAddress] = cty.StringVal(options.Address)
	}
	if options.SocketType != "" {
		rendered[auditOptionSocketType] = cty.StringVal(options.SocketType)
	}
	if options.WriteTimeout != "" {
		rendered[auditOptionWriteTimeout] = cty.StringVal(options.WriteTimeout)
	}
	return rendered
}

func normalizeHTTPAuditHeaders(raw []byte, context string) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", nil
	}

	var headers map[string][]string
	if err := json.Unmarshal(trimmed, &headers); err != nil {
		return "", fmt.Errorf("%s must be a JSON object with string array values: %w", context, err)
	}
	if headers == nil {
		return "", fmt.Errorf("%s must be a JSON object with string array values", context)
	}
	for header := range headers {
		if strings.TrimSpace(header) == "" {
			return "", fmt.Errorf("%s contains an empty header name", context)
		}
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return "", fmt.Errorf("compact %s: %w", context, err)
	}
	return compact.String(), nil
}
