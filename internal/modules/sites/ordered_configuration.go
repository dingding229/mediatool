package sites

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// jsonb does not preserve object-key declaration order. Field selector cases
// become an internal ordered branch array before storage; lookup-filter maps
// and request bodies remain ordinary objects. No upstream assets are bundled.
var ErrCaseOrderUnavailable = errors.New("stored site case priority is unavailable; re-import the fixed source snapshot")

type orderedObject struct {
	keys   []string
	values map[string]any
}

func readOrderedValue(dec *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, ErrConfig
	}
	token, err := dec.Token()
	if err != nil {
		return nil, ErrConfig
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := orderedObject{values: map[string]any{}}
		for dec.More() {
			rawKey, err := dec.Token()
			if err != nil {
				return nil, ErrConfig
			}
			key, ok := rawKey.(string)
			if !ok {
				return nil, ErrConfig
			}
			if _, duplicate := object.values[key]; duplicate {
				return nil, ErrConfig
			}
			value, err := readOrderedValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			object.keys = append(object.keys, key)
			object.values[key] = value
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrConfig
		}
		return object, nil
	case '[':
		values := []any{}
		for dec.More() {
			value, err := readOrderedValue(dec, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrConfig
		}
		return values, nil
	default:
		return nil, ErrConfig
	}
}

func plainOrderedValue(value any) any {
	switch value := value.(type) {
	case orderedObject:
		result := Map{}
		for _, key := range value.keys {
			result[key] = plainOrderedValue(value.values[key])
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, child := range value {
			result[index] = plainOrderedValue(child)
		}
		return result
	default:
		return value
	}
}

func orderedFieldRule(value any, stored bool) (Map, error) {
	node, ok := value.(orderedObject)
	if !ok {
		return nil, ErrConfig
	}
	rule := object(plainOrderedValue(node))
	if value, exists := node.values["case"]; exists {
		switch branches := value.(type) {
		case orderedObject:
			if stored && len(branches.keys) > 1 {
				return nil, ErrCaseOrderUnavailable
			}
			ordered := []any{}
			for _, selector := range branches.keys {
				ordered = append(ordered, Map{"selector": selector, "value": plainOrderedValue(branches.values[selector])})
			}
			rule["case"] = ordered
		case []any:
			seen := map[string]bool{}
			for _, entry := range branches {
				branch := object(plainOrderedValue(entry))
				selector, valid := branch["selector"].(string)
				if !valid || selector == "" || seen[selector] || len(branch) != 2 {
					return nil, ErrConfig
				}
				if _, exists := branch["value"]; !exists {
					return nil, ErrConfig
				}
				seen[selector] = true
			}
		default:
			return nil, ErrConfig
		}
	}
	if alternatives, exists := node.values["any"]; exists {
		entries, ok := alternatives.([]any)
		if !ok {
			return nil, ErrConfig
		}
		out := []any{}
		for _, entry := range entries {
			parsed, err := orderedFieldRule(entry, stored)
			if err != nil {
				return nil, err
			}
			out = append(out, parsed)
		}
		rule["any"] = out
	}
	return rule, nil
}

func orderedFields(value any, stored bool) (Map, error) {
	node, ok := value.(orderedObject)
	if !ok {
		return nil, ErrConfig
	}
	fields := Map{}
	for _, key := range node.keys {
		rule, err := orderedFieldRule(node.values[key], stored)
		if err != nil {
			return nil, err
		}
		fields[key] = rule
	}
	return fields, nil
}

func decodeDefinition(raw []byte, stored bool) (Map, error) {
	if len(raw) == 0 {
		return Map{}, nil
	}
	if len(raw) > 2<<20 {
		return nil, ErrConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := readOrderedValue(decoder, 0)
	if err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, ErrConfig
	}
	node, ok := value.(orderedObject)
	if !ok {
		return nil, ErrConfig
	}
	result := object(plainOrderedValue(node))
	requests, present := node.values["requests"].(orderedObject)
	if !present {
		return result, nil
	}
	for _, name := range requests.keys {
		request, ok := requests.values[name].(orderedObject)
		if !ok {
			return nil, ErrConfig
		}
		target := object(object(result["requests"])[name])
		if fields, exists := request.values["fields"]; exists {
			parsed, err := orderedFields(fields, stored)
			if err != nil {
				return nil, err
			}
			target["fields"] = parsed
		}
		if flat, exists := request.values["flat_map_fields"]; exists {
			group, ok := flat.(orderedObject)
			if !ok {
				return nil, ErrConfig
			}
			if fields, exists := group.values["fields"]; exists {
				parsed, err := orderedFields(fields, stored)
				if err != nil {
					return nil, err
				}
				object(target["flat_map_fields"])["fields"] = parsed
			}
		}
	}
	return result, nil
}

func canonicalDefinitionJSON(raw []byte) ([]byte, error) {
	root, err := decodeDefinition(raw, false)
	if err != nil {
		return nil, err
	}
	return json.Marshal(root)
}
