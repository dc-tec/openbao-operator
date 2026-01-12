package config

import (
	"fmt"
	"strings"

	openbaov1alpha1 "github.com/dc-tec/openbao-operator/api/v1alpha1"
	"github.com/zclconf/go-cty/cty"
)

type selfInitRequestDataHandler func(req openbaov1alpha1.SelfInitRequest) (cty.Value, error)

type selfInitRequestDataHandlerRegistration struct {
	Prefix  string
	Handler selfInitRequestDataHandler
}

var selfInitRequestDataHandlers = []selfInitRequestDataHandlerRegistration{
	{
		Prefix:  "sys/audit/",
		Handler: selfInitAuditDeviceDataHandler,
	},
	{
		Prefix:  "sys/auth/",
		Handler: selfInitAuthMethodDataHandler,
	},
	{
		Prefix:  "sys/mounts/",
		Handler: selfInitSecretEngineDataHandler,
	},
	{
		Prefix:  "sys/policies/",
		Handler: selfInitPolicyDataHandler,
	},
}

func resolveSelfInitRequestStructuredData(req openbaov1alpha1.SelfInitRequest) (cty.Value, bool, error) {
	handler, ok := findSelfInitRequestDataHandler(req.Path)
	if !ok {
		return cty.NilVal, false, nil
	}

	val, err := handler(req)
	if err != nil {
		return cty.NilVal, true, err
	}
	return val, true, nil
}

func findSelfInitRequestDataHandler(path string) (selfInitRequestDataHandler, bool) {
	bestIdx := -1
	bestLen := -1
	for i, reg := range selfInitRequestDataHandlers {
		if !strings.HasPrefix(path, reg.Prefix) {
			continue
		}
		if len(reg.Prefix) > bestLen {
			bestIdx = i
			bestLen = len(reg.Prefix)
		}
	}

	if bestIdx == -1 {
		return nil, false
	}
	return selfInitRequestDataHandlers[bestIdx].Handler, true
}

func selfInitAuditDeviceDataHandler(req openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
	if req.AuditDevice == nil {
		return cty.NilVal, fmt.Errorf("audit device request %q at path %q must use structured auditDevice field, not raw data", req.Name, req.Path)
	}

	dataVal, err := buildAuditDeviceData(req.AuditDevice)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to build audit device data for request %q: %w", req.Name, err)
	}

	return dataVal, nil
}

func selfInitAuthMethodDataHandler(req openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
	if req.AuthMethod == nil {
		return cty.NilVal, fmt.Errorf("auth method request %q at path %q must use structured authMethod field, not raw data", req.Name, req.Path)
	}

	dataVal, err := buildAuthMethodData(req.AuthMethod)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to build auth method data for request %q: %w", req.Name, err)
	}

	return dataVal, nil
}

func selfInitSecretEngineDataHandler(req openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
	if req.SecretEngine == nil {
		return cty.NilVal, fmt.Errorf("secret engine request %q at path %q must use structured secretEngine field, not raw data", req.Name, req.Path)
	}

	dataVal, err := buildSecretEngineData(req.SecretEngine)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to build secret engine data for request %q: %w", req.Name, err)
	}

	return dataVal, nil
}

func selfInitPolicyDataHandler(req openbaov1alpha1.SelfInitRequest) (cty.Value, error) {
	if req.Policy == nil {
		return cty.NilVal, fmt.Errorf("policy request %q at path %q must use structured policy field, not raw data", req.Name, req.Path)
	}

	dataVal, err := buildPolicyData(req.Policy)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to build policy data for request %q: %w", req.Name, err)
	}

	return dataVal, nil
}
