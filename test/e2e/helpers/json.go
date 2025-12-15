package helpers

import (
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// JSONFrom marshals an arbitrary Go value into apiextensionsv1.JSON.
//
// It accepts an interface because json.Marshal's signature requires it for values
// that are not known at compile time (e.g., structs, maps) and E2E tests often
// build small request payloads inline.
func JSONFrom(v interface{}) (*apiextensionsv1.JSON, error) { //nolint:revive // json.Marshal requires an interface for generic encoding.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return &apiextensionsv1.JSON{Raw: raw}, nil
}
