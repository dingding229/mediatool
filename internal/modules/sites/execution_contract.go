package sites

import "strings"

// The absent/empty parser is the existing CSS default. Only the explicitly
// implemented modes are accepted; unknown values must not become HTML/CSS.
func supportedParserMode(mode string) bool {
	switch strings.ToLower(mode) {
	case "", "cssselector", "xpath", "jsonpath", "xml", "none":
		return true
	default:
		return false
	}
}

func requestParser(rule Map) (string, error) {
	value, exists := rule["parser"]
	if !exists {
		return "", nil
	}
	mode, valid := value.(string)
	if !valid || !supportedParserMode(mode) {
		return "", ErrConfig
	}
	return mode, nil
}

func executionFlag(rule Map, name string) (bool, error) {
	raw, exists := rule[name]
	if !exists {
		return false, nil
	}
	value, valid := raw.(bool)
	if !valid {
		return false, ErrConfig
	}
	return value, nil
}

// Validate before resolving a URL, reading credentials or making any HTTP
// request. No browser executor is implemented; a regular HTTP response cannot
// satisfy an explicitly browser-dependent request.
func validateRequestExecution(rule Map) error {
	if len(rule) == 0 {
		return ErrConfig
	}
	disabled, err := executionFlag(rule, "disabled")
	if err != nil || disabled {
		return ErrConfig
	}
	browser, err := executionFlag(rule, "chrome")
	if err != nil || browser {
		return ErrConfig
	}
	_, err = requestParser(rule)
	return err
}

func validateDocumentRequest(rule Map) error {
	if err := validateRequestExecution(rule); err != nil {
		return err
	}
	mode, _ := requestParser(rule)
	if strings.EqualFold(mode, "None") {
		return ErrConfig
	}
	return nil
}
