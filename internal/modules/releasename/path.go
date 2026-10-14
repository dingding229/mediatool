package releasename

import (
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateTemplate validates syntax independently of real media values. Variable
// values are validated again after substitution; none can inject directories.
func ValidateTemplate(template string) error {
	if len(template) == 0 || len(template) > 512 || strings.HasPrefix(template, "/") || strings.Contains(template, "\\") {
		return errors.New("invalid relative template")
	}
	parts := strings.Split(template, "/")
	if len(parts) > 12 {
		return errors.New("too many template directories")
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return errors.New("invalid template component")
		}
		valid := true
		clean := variables.ReplaceAllStringFunc(part, func(token string) string {
			if !supportedToken(strings.Trim(token, "{}")) {
				valid = false
			}
			return "value"
		})
		if !valid || strings.ContainsAny(clean, "{}\\") {
			return errors.New("unknown naming token")
		}
		if err := validComponent(clean); err != nil {
			return err
		}
	}
	return nil
}
func supportedToken(name string) bool {
	switch name {
	case "title", "year", "season", "episode", "season_episode", "resolution", "videoFormat", "source", "codec", "group", "ext", "original":
		return true
	}
	return false
}
func validComponent(s string) error {
	if s == "" || s == "." || s == ".." || len(s) > 255 || !utf8.ValidString(s) || strings.ContainsAny(s, "/\\:") {
		return errors.New("invalid rendered component")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("invalid rendered component")
		}
	}
	return nil
}
func RenderPath(template string, h Hint, original string) (string, error) {
	if err := ValidateTemplate(template); err != nil {
		return "", err
	}
	parts := strings.Split(template, "/")
	for i, part := range parts {
		componentHint := h
		if i < len(parts)-1 {
			componentHint.Extension = ""
		}
		componentOriginal := original
		if i < len(parts)-1 {
			componentOriginal = strings.TrimSuffix(original, h.Extension)
		}
		value, err := Render(part, componentHint, componentOriginal)
		if err != nil {
			return "", err
		}
		if err = validComponent(value); err != nil {
			return "", err
		}
		parts[i] = value
	}
	result := strings.Join(parts, "/")
	if path.Clean(result) != result {
		return "", errors.New("noncanonical rendered path")
	}
	return result, nil
}
