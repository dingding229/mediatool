package networkproxy

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

var ErrInvalid = errors.New("代理协议仅限 http/socks5；请填写合法的主机、1–65535 端口和认证信息")
var ErrNeedsInput = errors.New("代理配置需重新填写；不会回退直连")
var ErrPassword = errors.New("更换代理协议、主机、端口或用户名时，请重填密码或明确清除原密码")

type Fields struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
}
type Record struct {
	Fields
	TMDB        bool   `json:"tmdb"`
	Sites       bool   `json:"sites"`
	Revision    int64  `json:"revision"`
	Configured  bool   `json:"configured"`
	HasPassword bool   `json:"hasPassword"`
	NeedsInput  bool   `json:"needsInput"`
	Password    string `json:"-"`
}
type Update struct {
	Fields
	TMDB          bool   `json:"tmdb"`
	Sites         bool   `json:"sites"`
	Revision      int64  `json:"revision"`
	Password      string `json:"password"`
	ClearPassword bool   `json:"clearPassword"`
	Clear         bool   `json:"clear"`
}

func empty(f Fields) bool { return f.Host == "" && f.Port == 0 && f.Username == "" }
func Validate(r Record) error {
	if r.NeedsInput {
		return ErrNeedsInput
	}
	if r.Protocol != "" && r.Protocol != "http" && r.Protocol != "socks5" {
		return ErrInvalid
	}
	if empty(r.Fields) && r.Password == "" {
		if r.TMDB || r.Sites {
			return ErrInvalid
		}
		return nil
	}
	if (r.Protocol != "http" && r.Protocol != "socks5") || r.Port < 1 || r.Port > 65535 || len(r.Host) > 253 || len(r.Username) > 255 || len(r.Password) > 255 {
		return ErrInvalid
	}
	if strings.ContainsAny(r.Host, "/@?#\\[]%") || strings.IndexFunc(r.Host, unicode.IsSpace) >= 0 || strings.IndexFunc(r.Username+r.Password, unicode.IsControl) >= 0 || r.Protocol == "http" && strings.Contains(r.Username, ":") {
		return ErrInvalid
	}
	if net.ParseIP(r.Host) == nil {
		if r.Host == "" || strings.Contains(r.Host, ":") {
			return ErrInvalid
		}
		for _, label := range strings.Split(strings.TrimSuffix(r.Host, "."), ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return ErrInvalid
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return ErrInvalid
				}
			}
		}
	}
	if r.Username == "" && r.Password != "" {
		return ErrInvalid
	}
	return nil
}
func (r Record) proxyURL() string {
	if empty(r.Fields) {
		return ""
	}
	u := url.URL{Scheme: r.Protocol, Host: net.JoinHostPort(r.Host, strconv.Itoa(r.Port))}
	if r.Username != "" {
		u.User = url.UserPassword(r.Username, r.Password)
	}
	return u.String()
}
func decodePayload(out Record, values map[string]string) Record {
	out.Configured = true
	if values["v"] != "2" {
		out.NeedsInput = true
		return out
	}
	for k := range values {
		switch k {
		case "v", "protocol", "host", "port", "username", "password":
		default:
			out.NeedsInput = true
			return out
		}
	}
	port, err := strconv.Atoi(values["port"])
	parsed := out
	parsed.Fields = Fields{values["protocol"], values["host"], port, values["username"]}
	parsed.Password = values["password"]
	if err != nil || empty(parsed.Fields) || Validate(parsed) != nil {
		out.NeedsInput = true
		return out
	}
	parsed.HasPassword = parsed.Password != ""
	return parsed
}
