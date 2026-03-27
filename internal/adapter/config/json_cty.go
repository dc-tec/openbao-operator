package config

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/zclconf/go-cty/cty"
)

func decodeJSONToCty(raw []byte, subject string) (cty.Value, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return cty.NilVal, fmt.Errorf("failed to decode %s: %w", subject, err)
	}

	ctyVal, err := jsonToCty(decoded)
	if err != nil {
		return cty.NilVal, fmt.Errorf("failed to convert %s to HCL: %w", subject, err)
	}

	return ctyVal, nil
}

// jsonToCty converts a decoded JSON value (maps, slices, strings, numbers,
// booleans) into a cty.Value tree suitable for hclwrite. This function uses
// `any` because encoding/json produces generic map and slice trees.
func jsonToCty(v any) (cty.Value, error) {
	switch val := v.(type) {
	case map[string]any:
		if len(val) == 0 {
			return cty.EmptyObjectVal, nil
		}

		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		obj := make(map[string]cty.Value, len(val))
		for _, k := range keys {
			if val[k] == nil {
				continue
			}

			child, err := jsonToCty(val[k])
			if err != nil {
				return cty.NilVal, err
			}
			if child != cty.NilVal {
				obj[k] = child
			}
		}

		return cty.ObjectVal(obj), nil
	case []any:
		if len(val) == 0 {
			return cty.EmptyTupleVal, nil
		}

		elems := make([]cty.Value, 0, len(val))
		for _, elem := range val {
			if elem == nil {
				continue
			}
			child, err := jsonToCty(elem)
			if err != nil {
				return cty.NilVal, err
			}
			if child != cty.NilVal {
				elems = append(elems, child)
			}
		}

		if len(elems) == 0 {
			return cty.EmptyTupleVal, nil
		}
		return cty.TupleVal(elems), nil
	case string:
		return cty.StringVal(val), nil
	case bool:
		return cty.BoolVal(val), nil
	case float64:
		if val == math.Trunc(val) {
			return cty.StringVal(strconv.FormatInt(int64(val), 10)), nil
		}
		return cty.StringVal(strconv.FormatFloat(val, 'f', -1, 64)), nil
	case nil:
		return cty.NilVal, nil
	default:
		return cty.NilVal, fmt.Errorf("unsupported JSON value type %T in self-init data", v)
	}
}
