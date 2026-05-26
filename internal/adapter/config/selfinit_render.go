package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type autopilotRequestConfig struct {
	CleanupDeadServers             bool   `json:"cleanup_dead_servers"`
	DeadServerLastContactThreshold string `json:"dead_server_last_contact_threshold"`
	MinQuorum                      int    `json:"min_quorum"`
	ServerStabilizationTime        string `json:"server_stabilization_time"`
}

// RenderSelfInitHCL renders only the self-initialization stanzas as a separate
// HCL configuration. This is stored in a separate ConfigMap that is only mounted
// for pod-0, since only the first pod needs to execute initialization requests.
// If bootstrapConfig is provided, it will be merged with user requests.
func RenderSelfInitHCL(cluster *openbaov1alpha1.OpenBaoCluster, bootstrapConfig *OperatorBootstrapConfig) ([]byte, error) {
	file := hclwrite.NewEmptyFile()
	body := file.Body()

	if err := validateAuditFileStorageConfiguration(cluster); err != nil {
		return nil, err
	}

	// If bootstrap config provided, render it first
	if bootstrapConfig != nil {
		if strings.TrimSpace(bootstrapConfig.OIDCIssuerURL) == "" {
			return nil, fmt.Errorf("OIDC issuer URL is required to render operator bootstrap")
		}
		if strings.TrimSpace(bootstrapConfig.OIDCDiscoveryURL) == "" && strings.TrimSpace(bootstrapConfig.OIDCJWKSURL) == "" && len(bootstrapConfig.JWTKeysPEM) == 0 {
			return nil, fmt.Errorf("an OIDC discovery URL, a JWKS URL, or at least one JWT public key is required to render operator bootstrap")
		}
		if strings.TrimSpace(bootstrapConfig.OperatorNS) == "" {
			return nil, fmt.Errorf("operator namespace is required to render operator bootstrap")
		}
		if strings.TrimSpace(bootstrapConfig.OperatorSA) == "" {
			return nil, fmt.Errorf("operator service account name is required to render operator bootstrap")
		}

		body.AppendBlock(buildSelfInitBootstrapInitializeBlock(cluster, *bootstrapConfig))
	}

	// Render user self-init requests if enabled
	if cluster.Spec.SelfInit != nil && cluster.Spec.SelfInit.Enabled {
		requests := cluster.Spec.SelfInit.Requests
		if !hasRequest(requests, "configure-autopilot") {
			req, err := defaultAutopilotRequest()
			if err != nil {
				return nil, fmt.Errorf("failed to create default autopilot request: %w", err)
			}
			requests = append(requests, req)
		}

		if err := renderSelfInitStanzas(body, requests); err != nil {
			return nil, fmt.Errorf("failed to render self-init stanzas: %w", err)
		}
	}

	return file.Bytes(), nil
}

func hasRequest(requests []openbaov1alpha1.SelfInitRequest, name string) bool {
	for _, req := range requests {
		if req.Name == name {
			return true
		}
	}
	return false
}

func defaultAutopilotRequest() (openbaov1alpha1.SelfInitRequest, error) {
	data, err := json.Marshal(autopilotRequestConfig{
		CleanupDeadServers:             true,
		DeadServerLastContactThreshold: "24h",
		MinQuorum:                      3,
		ServerStabilizationTime:        "10s",
	})
	if err != nil {
		return openbaov1alpha1.SelfInitRequest{}, err
	}

	return openbaov1alpha1.SelfInitRequest{
		Name:      "configure-autopilot",
		Operation: openbaov1alpha1.SelfInitOperationUpdate,
		Path:      "sys/storage/raft/autopilot/configuration",
		Data: &apiextensionsv1.JSON{
			Raw: data,
		},
	}, nil
}

// renderSelfInitStanzas generates HCL initialize stanzas for OpenBao's self-initialization feature.
// Each request is rendered as an initialize block containing a named request block with the specified
// operation, path, and optional data fields. The request name is required by OpenBao's configuration
// schema and is used as the map key when parsing JSON/HCL.
func renderSelfInitStanzas(body *hclwrite.Body, requests []openbaov1alpha1.SelfInitRequest) error {
	for _, req := range requests {
		if strings.TrimSpace(req.Name) == "" {
			continue
		}

		initLabel := req.Name
		requestLabel := fmt.Sprintf("%s-request", req.Name)

		initBlock := buildInitializeBlock(initLabel)
		initBody := initBlock.Body()
		requestBlock := buildInitializeRequestBlock(requestLabel, string(req.Operation), req.Path, req.AllowFailure)
		requestBody := requestBlock.Body()

		dataVal := cty.NilVal
		if structuredVal, handled, err := resolveSelfInitRequestStructuredData(req); err != nil {
			return err
		} else if handled {
			dataVal = structuredVal
		} else if req.Data != nil && len(req.Data.Raw) > 0 {
			ctyVal, err := decodeJSONToCty(req.Data.Raw, fmt.Sprintf("self-init data for request %q", req.Name))
			if err != nil {
				return err
			}
			dataVal = ctyVal
		}

		if dataVal != cty.NilVal {
			if err := setSelfInitRequestData(requestBody, dataVal); err != nil {
				return fmt.Errorf("failed to render self-init request data for request %q: %w", req.Name, err)
			}
		}

		initBody.AppendBlock(requestBlock)
		body.AppendBlock(initBlock)
	}

	return nil
}

//nolint:unparam // Error return maintained for API consistency and future extensibility
func setSelfInitRequestData(requestBody *hclwrite.Body, dataVal cty.Value) error {
	if dataVal == cty.NilVal {
		return nil
	}

	// Prefer "data { ... }" blocks which match OpenBao self-init docs and the
	// operator-bootstrap output. Some endpoints appear to ignore "data = { ... }".
	if dataVal.Type().IsObjectType() || dataVal.Type().IsMapType() {
		dataBlock := requestBody.AppendNewBlock("data", nil)
		dataBody := dataBlock.Body()

		dataMap := dataVal.AsValueMap()
		keys := make([]string, 0, len(dataMap))
		for k := range dataMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			dataBody.SetAttributeValue(k, dataMap[k])
		}
		return nil
	}

	requestBody.SetAttributeValue("data", dataVal)
	return nil
}
