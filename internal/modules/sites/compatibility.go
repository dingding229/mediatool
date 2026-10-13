package sites

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html/charset"
)

type compatibilityIssue struct {
	code string // Fixed classifications; no site-controlled values in audit output.
	note string
}

func Compatibility(c Config) Map {
	issues := []compatibilityIssue{}
	add := func(code, note string) { issues = append(issues, compatibilityIssue{code, note}) }
	if c.Encoding != "" {
		if encoding, _ := charset.Lookup(c.Encoding); encoding == nil {
			add("unsupported_encoding", "encoding unsupported")
		}
	}
	for _, name := range []string{"search", "user_basic_info", "profile", "gen_dl_token"} {
		rule := c.Requests[name]
		if len(rule) == 0 {
			continue
		}
		if browser, err := executionFlag(rule, "chrome"); err != nil {
			add("invalid_execution_flags", name+": invalid browser execution flag")
		} else if browser {
			add("browser_execution_required", name+": browser required")
		}
		if disabled, err := executionFlag(rule, "disabled"); err != nil {
			add("invalid_execution_flags", name+": invalid disabled execution flag")
		} else if disabled {
			add("request_disabled", name+": request explicitly disabled")
		}
		if _, err := requestParser(rule); err != nil {
			add("unsupported_parser", name+": unsupported or invalid parser")
		} else if strings.EqualFold(text(rule["parser"]), "None") {
			add("raw_parser_for_document", name+": raw parser cannot extract document fields")
		}
		if _, _, err := requestPayload(rule, nil, nil); err != nil {
			add("invalid_request_payload", name+": invalid or ambiguous request payload")
		}
		declaredHeaders, _, headerErr := declaredRequestHeaders(rule, nil)
		if headerErr != nil {
			add("invalid_request_headers", name+": invalid or ambiguous request headers")
		}
		requiredHeaders, requiredErr := requiredRequestHeaderNames(rule)
		if requiredErr != nil {
			add("invalid_required_headers", name+": invalid or ambiguous required headers")
		}
		for _, header := range requiredHeaders {
			if !strings.EqualFold(header, "x-api-key") && !strings.EqualFold(header, "apitoken") && strings.TrimSpace(declaredHeaders.Get(header)) == "" {
				add("authentication_producer_required", name+": required header needs dedicated authentication")
			}
		}
		// Filters belong to field rules. A request payload, parameter, literal
		// constant or lookup table with keys such as "filters" is ordinary data.
		scanFieldFilters(object(rule["fields"]), name+".fields", add)
		scanFieldFilters(object(object(rule["flat_map_fields"])["fields"]), name+".flat_map_fields.fields", add)
	}
	if c.Schema != "NexusPHP" && c.Schema != "mTorrent" {
		add("framework_live_validation_required", "framework requires dedicated live validation")
	}
	notes := make([]string, 0, len(issues))
	seen := map[string]bool{}
	codes := []string{}
	for _, issue := range issues {
		notes = append(notes, issue.note)
		if !seen[issue.code] {
			seen[issue.code] = true
			codes = append(codes, issue.code)
		}
	}
	sort.Strings(codes)
	state := "parser_supported"
	if len(issues) > 0 {
		state = "partial"
	}
	return Map{"state": state, "notes": notes, "issueCodes": codes, "accountVerified": false, "source": NativeRepository}
}

func scanFieldFilters(fields Map, at string, add func(string, string)) {
	for _, name := range keys(fields) {
		scanFieldRuleFilters(object(fields[name]), at+"."+name, add)
	}
}
func scanFieldRuleFilters(rule Map, at string, add func(string, string)) {
	for _, raw := range slice(rule["filters"]) {
		filterRule := object(raw)
		name := text(filterRule["name"])
		switch name {
		case "strip", "constant", "append_left", "append_right", "replace", "split", "pad_zero", "not_blank", "eq", "case", "count", "byte_size", "timestamp", "user_profile_holder", "append_field_right":
		case "timezone":
			add("timezone_semantics_unverified", at+": timezone shift semantics require consumer verification")
		case "regex", "re_search":
			pattern := text(filterRule["args"])
			if args := slice(filterRule["args"]); len(args) > 0 {
				pattern = text(args[0])
			}
			if _, err := regexp.Compile(pattern); err != nil {
				add("unsupported_regexp", at+": regexp unsupported")
			}
		default:
			add("unsupported_filter", at+": unsupported filter")
		}
	}
	for _, alternative := range slice(rule["any"]) {
		scanFieldRuleFilters(object(alternative), at+".any", add)
	}
}
