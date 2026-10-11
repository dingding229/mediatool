package sites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"github.com/antchfx/htmlquery"
	"github.com/antchfx/xmlquery"
	"golang.org/x/net/html"
)

type document struct {
	mode string
	html *html.Node
	xml  *xmlquery.Node
	data any
	raw  string
}

func parseDocument(raw []byte, mode string) (document, error) {
	if !supportedParserMode(mode) || strings.EqualFold(mode, "None") {
		return document{}, ErrConfig
	}
	d := document{mode: mode, raw: string(raw)}
	var err error
	switch strings.ToLower(mode) {
	case "jsonpath":
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		err = decoder.Decode(&d.data)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = ErrConfig
			}
		}
	case "xml":
		d.xml, err = xmlquery.Parse(bytes.NewReader(raw))
	default:
		d.html, err = htmlquery.Parse(bytes.NewReader(raw))
	}
	if err != nil {
		return d, ErrConfig
	}
	return d, nil
}
func (d document) values(selector string, scope any) ([]any, error) {
	if !supportedParserMode(d.mode) || strings.EqualFold(d.mode, "None") {
		return nil, ErrConfig
	}
	if selector == "" {
		return []any{scope}, nil
	}
	switch strings.ToLower(d.mode) {
	case "jsonpath":
		v, exists := d.jsonValue(selector, scope)
		if !exists || v == nil {
			return nil, nil
		}
		if a, ok := v.([]any); ok {
			return a, nil
		}
		return []any{v}, nil
	case "xpath":
		node := d.html
		if n, ok := scope.(*html.Node); ok {
			node = n
		}
		matches, err := htmlquery.QueryAll(node, selector)
		if err != nil {
			return nil, ErrConfig
		}
		out := []any{}
		for _, match := range matches {
			out = append(out, match)
		}
		return out, nil
	case "xml":
		node := d.xml
		if n, ok := scope.(*xmlquery.Node); ok {
			node = n
		}
		matches, err := xmlquery.QueryAll(node, selector)
		if err != nil {
			return nil, ErrConfig
		}
		out := []any{}
		for _, match := range matches {
			out = append(out, match)
		}
		return out, nil
	default:
		node := d.html
		if n, ok := scope.(*html.Node); ok {
			node = n
		}
		compiled, err := cascadia.Compile(selector)
		if err != nil {
			return nil, ErrConfig
		}
		selection := goquery.NewDocumentFromNode(node).FindMatcher(compiled)
		out := []any{}
		for _, n := range selection.Nodes {
			out = append(out, n)
		}
		return out, nil
	}
}

type fieldResolver func(string) (any, error)

func (d document) field(name string, rule Map, scope any, vars map[string]string) (any, error) {
	return d.fieldWithResolver(name, rule, scope, vars, nil)
}
func (d document) fieldWithResolver(name string, rule Map, scope any, vars map[string]string, resolve fieldResolver) (any, error) {
	var value any
	if alternatives, ok := rule["any"].([]any); ok {
		for _, alternative := range alternatives {
			v, err := d.fieldWithResolver(name, object(alternative), scope, vars, resolve)
			if err != nil {
				return nil, err
			}
			if !blank(v) {
				value = v
				break
			}
		}
	} else if branches, ok := rule["case"].([]any); ok {
		for _, raw := range branches {
			branch := object(raw)
			if text(branch["selector"]) == "*" {
				value = branch["value"]
			}
		}
		for _, raw := range branches {
			branch := object(raw)
			selector := text(branch["selector"])
			if selector == "*" {
				continue
			}
			found, err := d.values(selector, scope)
			if err != nil {
				return nil, err
			}
			if len(found) > 0 {
				value = branch["value"]
				break
			}
		}
	} else if _, exists := rule["case"]; exists {
		return nil, ErrCaseOrderUnavailable
	} else {
		selector := text(rule["selector"])
		if selector == "" && strings.EqualFold(d.mode, "JsonPath") {
			selector = name
		}
		values, err := d.values(selector, scope)
		if err != nil {
			return nil, err
		}
		out := []any{}
		if text(rule["selection"]) == "html" {
			var b strings.Builder
			node := d.html
			if n, ok := scope.(*html.Node); ok {
				node = n
			}
			html.Render(&b, node)
			out = append(out, b.String())
		} else {
			for _, v := range values {
				switch node := v.(type) {
				case *html.Node:
					if attribute := text(rule["attribute"]); attribute != "" {
						s := ""
						for _, a := range node.Attr {
							if a.Key == attribute {
								s = a.Val
								break
							}
						}
						out = append(out, s)
					} else {
						copy := cloneNode(node)
						selection := goquery.NewDocumentFromNode(copy).Selection
						if remove := text(rule["remove"]); remove != "" {
							matcher, err := cascadia.Compile(remove)
							if err != nil {
								return nil, ErrConfig
							}
							selection.FindMatcher(matcher).Remove()
						}
						out = append(out, strings.TrimSpace(selection.Text()))
					}
				case *xmlquery.Node:
					if attribute := text(rule["attribute"]); attribute != "" {
						out = append(out, node.SelectAttr(attribute))
					} else {
						out = append(out, node.InnerText())
					}
				default:
					out = append(out, v)
				}
			}
		}
		if rule["array"] == true {
			value = out
		} else if len(out) > 0 {
			value = out[0]
		}
	}
	for _, raw := range slice(rule["filters"]) {
		filterRule := object(raw)
		apply := func(input any) (any, error) {
			if text(filterRule["name"]) == "append_field_right" {
				name, ok := filterRule["args"].(string)
				if !ok || resolve == nil {
					return nil, ErrConfig
				}
				related, err := resolve(name)
				if err != nil {
					return nil, err
				}
				return text(input) + text(related), nil
			}
			return filter(input, filterRule, vars)
		}
		var err error
		if array, ok := value.([]any); ok && text(filterRule["name"]) != "count" {
			out := make([]any, len(array))
			for index, entry := range array {
				out[index], err = apply(entry)
				if err != nil {
					return nil, err
				}
			}
			value = out
		} else {
			value, err = apply(value)
			if err != nil {
				return nil, err
			}
		}
	}
	if chars, ok := rule["trim_chars"].(string); ok {
		value = trimFieldStrings(value, func(value string) string { return strings.Trim(value, chars) })
	} else if rule["trim_chars"] == true {
		value = trimFieldStrings(value, strings.TrimSpace)
	}
	if blank(value) {
		if def, ok := rule["default"]; ok {
			value = def
		}
	}
	return value, nil
}
func cloneNode(n *html.Node) *html.Node {
	c := &html.Node{Type: n.Type, Data: n.Data, DataAtom: n.DataAtom, Namespace: n.Namespace, Attr: append([]html.Attribute(nil), n.Attr...)}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		c.AppendChild(cloneNode(child))
	}
	return c
}
func filter(value any, rule Map, vars map[string]string) (any, error) {
	name := text(rule["name"])
	args := rule["args"]
	list := slice(args)
	arg := func(i int, fallback string) string {
		if i < len(list) {
			return substitute(text(list[i]), vars)
		}
		return fallback
	}
	s := text(value)
	switch name {
	case "strip":
		return strings.TrimSpace(s), nil
	case "constant":
		return args, nil
	case "append_left":
		return substitute(text(args), vars) + s, nil
	case "append_right":
		return s + substitute(text(args), vars), nil
	case "user_profile_holder":
		return substitute(s, vars), nil
	case "replace":
		return strings.ReplaceAll(s, arg(0, ""), arg(1, "")), nil
	case "split":
		parts := strings.Split(s, arg(0, "\n"))
		index, _ := strconv.Atoi(arg(1, "0"))
		if index < 0 {
			index = len(parts) + index
		}
		if index < 0 || index >= len(parts) {
			return "", nil
		}
		return parts[index], nil
	case "pad_zero":
		n, _ := strconv.Atoi(text(args))
		if n < 0 || n > 100 {
			return nil, ErrConfig
		}
		return strings.Repeat("0", max(0, n-len(s))) + s, nil
	case "not_blank":
		if blank(value) {
			if len(list) > 1 {
				return list[1], nil
			}
			return false, nil
		}
		if len(list) > 0 {
			return list[0], nil
		}
		return true, nil
	case "eq":
		return s == text(args), nil
	case "case":
		return object(args)[s], nil
	case "count":
		if array, ok := value.([]any); ok {
			return len(array), nil
		}
		return utf8.RuneCountInString(s), nil
	case "regex", "re_search":
		pattern := text(args)
		if len(list) > 0 {
			pattern = arg(0, "")
		}
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return nil, ErrConfig
		}
		if name == "regex" {
			return compiled.MatchString(s), nil
		}
		found := compiled.FindStringSubmatch(s)
		index, _ := strconv.Atoi(arg(1, "0"))
		if index < 0 || index >= len(found) {
			return "", nil
		}
		return found[index], nil
	case "byte_size":
		if integer, err := strconv.ParseUint(strings.TrimSpace(s), 10, 63); err == nil {
			return int64(integer), nil
		}
		match := regexp.MustCompile(`(?i)([0-9][0-9,.]*)\s*([KMGTPE]?I?B)`).FindStringSubmatch(strings.ReplaceAll(s, "\u00a0", " "))
		if len(match) != 3 {
			return nil, nil
		}
		n, err := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", ""), 64)
		if err != nil {
			return nil, ErrConfig
		}
		unit := strings.ToUpper(match[2])
		power := strings.Index("BKMGTPE", unit[:1])
		if power < 0 {
			power = 0
		}
		for range power {
			n *= 1024
		}
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n >= math.Exp2(63) {
			return nil, ErrConfig
		}
		return int64(n), nil
	case "timestamp":
		if blank(value) {
			return nil, nil
		}
		layouts := []string{}
		if len(list) > 16 {
			return nil, ErrConfig
		}
		for index := range list {
			layouts = append(layouts, arg(index, ""))
		}
		if len(layouts) == 0 && text(args) != "" {
			layouts = append(layouts, text(args))
		}
		layouts = append(layouts, time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02")
		for _, layout := range layouts {
			if layout != "" {
				if parsed, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
					return parsed.UnixMilli(), nil
				}
			}
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			if n < 0 || n > 253402300799999 {
				return nil, ErrConfig
			}
			if n < 100000000000 {
				n *= 1000
			}
			return n, nil
		}
		return nil, nil
	case "timezone":
		// Upstream numeric shifts are not IANA offsets (some exceed +14).
		// Until their consumer semantics are established, never label a no-op as
		// supported or silently corrupt timestamps. Search resolves required fields
		// only, so an unused display-time field does not break resource discovery.
		return nil, ErrConfig
	default:
		return nil, fmt.Errorf("unsupported site filter: %s", name)
	}
}

var placeholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func substitute(s string, vars map[string]string) string {
	// Single pass: user search text containing {passkey} is data, not another
	// template. Never recursively expand credentials inside substituted values.
	return placeholder.ReplaceAllStringFunc(s, func(match string) string {
		if value, ok := vars[match[1:len(match)-1]]; ok {
			return value
		}
		return match
	})
}

func numeric(value any) float64 {
	if v, ok := value.(float64); ok {
		return v
	}
	n, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(text(value)), ",", ""), 64)
	return n
}
func mapFields(d document, rules Map, scope any, vars map[string]string) (Map, error) {
	return mapFieldsSelected(d, rules, scope, vars, keys(rules), nil)
}

func mapFieldsSelected(d document, rules Map, scope any, vars map[string]string, names []string, inherited Map) (Map, error) {
	out := Map{}
	active := map[string]bool{}
	var resolve fieldResolver
	resolve = func(name string) (any, error) {
		if value, done := out[name]; done {
			return value, nil
		}
		raw, exists := rules[name]
		if !exists {
			if value, exists := inherited[name]; exists {
				return value, nil
			}
			return nil, ErrConfig
		}
		if active[name] || len(active) >= 64 {
			return nil, ErrConfig
		}
		active[name] = true
		defer delete(active, name)
		value, err := d.fieldWithResolver(name, object(raw), scope, vars, resolve)
		if err != nil {
			return nil, err
		}
		out[name] = value
		return value, nil
	}
	for _, name := range names {
		if _, exists := rules[name]; !exists {
			continue
		}
		if _, err := resolve(name); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Resolve JSONPath without collapsing missing/null/scalar into an empty array.
// Field extraction may accept them; the declared search result list may not.
func (d document) jsonValue(selector string, scope any) (any, bool) {
	v := scope
	if v == nil {
		v = d.data
	}
	parts := strings.Split(strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(selector, "$."), "[", "."), "]", ""), ".")
	for _, part := range parts {
		if part == "" || part == "$" {
			continue
		}
		switch node := v.(type) {
		case Map:
			var ok bool
			v, ok = node[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			v = node[index]
		default:
			return nil, false
		}
	}
	return v, true
}
