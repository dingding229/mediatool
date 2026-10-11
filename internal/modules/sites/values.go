package sites

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func checkedNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case json.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return number, err == nil && !math.IsNaN(number) && !math.IsInf(number, 0)
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case float64:
		return value, !math.IsNaN(value) && !math.IsInf(value, 0)
	default:
		return 0, false
	}
}

func checkedInteger(value any, bits int) (int64, bool) {
	if bits != 31 && bits != 63 {
		return 0, false
	}
	switch value.(type) {
	case json.Number, string, int, int64:
		integer, err := strconv.ParseInt(strings.TrimSpace(text(value)), 10, bits+1)
		if err == nil && integer >= 0 {
			return integer, bits == 63 || integer < 1<<31
		}
	}
	number, valid := checkedNumber(value)
	if !valid || number < 0 || number >= math.Exp2(float64(bits)) || math.Trunc(number) != number {
		return 0, false
	}
	return int64(number), true
}

func parameterValues(value any, vars map[string]string) ([]string, error) {
	values, ok := value.([]any)
	if !ok {
		values = []any{value}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		switch value.(type) {
		case nil, string, json.Number, bool, int, int64, float64:
			out = append(out, substitute(text(value), vars))
		default:
			return nil, ErrConfig
		}
	}
	return out, nil
}

func substituteJSON(value any, vars map[string]string, depth int) (any, error) {
	if depth > 64 {
		return nil, ErrConfig
	}
	switch value := value.(type) {
	case string:
		return substitute(value, vars), nil
	case Map:
		out := Map{}
		for key, entry := range value {
			resolved, err := substituteJSON(entry, vars, depth+1)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(value))
		for index, entry := range value {
			resolved, err := substituteJSON(entry, vars, depth+1)
			if err != nil {
				return nil, err
			}
			out[index] = resolved
		}
		return out, nil
	case nil, json.Number, bool, int, int64, float64:
		return value, nil
	default:
		return nil, ErrConfig
	}
}

func trimFieldStrings(value any, trim func(string) string) any {
	switch value := value.(type) {
	case string:
		return trim(value)
	case []any:
		out := make([]any, len(value))
		for index, entry := range value {
			out[index] = trimFieldStrings(entry, trim)
		}
		return out
	default:
		return value
	}
}
