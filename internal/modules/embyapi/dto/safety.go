package dto

import (
	"bytes"
	"encoding/json"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Path is a synthetic leaf only. No leading-slash exception: both '/' and '\\'
// are prohibited everywhere, as are URI/query delimiters and encoded separators.
func safeFilename(value string) bool {
	if value == "" {
		return true
	}
	dot := strings.LastIndexByte(value, '.')
	if dot < 1 || len(value)-dot > 17 {
		return false
	}
	for _, ch := range value[dot+1:] {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9') {
			return false
		}
	}
	return dot < len(value)-1 && len(value) <= 512 && utf8.ValidString(value) && !strings.ContainsAny(value, "/\\:?&=#%") && !strings.Contains(value, "..") && !strings.ContainsFunc(value, unicode.IsControl)
}

// Only a credential-free absolute native stream URL is permitted as Path.
// Its builder binds the origin to configuration, never request headers.
func safeStreamURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 4096 || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) || strings.Contains(value, "\\") {
		return false
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return false
	}
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return false
	}
	lower := strings.ToLower(decoded)
	for _, marker := range []string{"pickcode", "pick_code", "strm302", "/api/v1/cloudstorage", "/115/", "/root/", "/data/", "/mnt/", "..", "%", "\\"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	host := strings.ToLower(u.Hostname())
	if host == "115.com" || strings.HasSuffix(host, ".115.com") || host == "115cdn.net" || strings.HasSuffix(host, ".115cdn.net") {
		return false
	}
	if !strings.HasPrefix(u.Path, "/Videos/") || strings.Contains(u.Path, "//") {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q) != 2 || len(q["Static"]) != 1 || q.Get("Static") != "true" {
		return false
	}
	for _, key := range []string{"MediaSourceId"} {
		if len(q[key]) != 1 || q.Get(key) == "" || len(q.Get(key)) > 512 {
			return false
		}
		for _, ch := range q.Get(key) {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
				return false
			}
		}
	}
	return true
}

// DirectStreamUrl is the same native route and source, plus a media-user token.
// Path itself never carries credentials; authentication is supplied by the client.
func safeDirectURL(absolute, relative string) bool {
	if relative == "" {
		return true
	}
	u, err := url.Parse(absolute)
	direct, e := url.Parse(relative)
	if err != nil || e != nil || absolute == "" || !safeStreamURL(absolute) || direct.IsAbs() || direct.Host != "" || direct.Fragment != "" {
		return false
	}
	q, e := url.ParseQuery(direct.RawQuery)
	if e != nil || len(q["api_key"]) != 1 || q.Get("api_key") == "" {
		return false
	}
	for _, ch := range q.Get("api_key") {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return false
		}
	}
	if len(q.Get("api_key")) > 512 {
		return false
	}
	q.Del("api_key")
	direct.RawQuery = q.Encode()
	u.RawQuery = u.Query().Encode()
	return direct.RequestURI() == u.RequestURI()
}

// No arbitrary metadata blobs: restrict provider/image maps to known keys. Free
// metadata strings cannot carry known private mount or resolver URL markers.
func safeMetadata(value any) error {
	var visit func(reflect.Value) bool
	visit = func(v reflect.Value) bool {
		if v.Kind() == reflect.Pointer {
			return v.IsNil() || visit(v.Elem())
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				name := v.Type().Field(i).Name
				if v.Type() == reflect.TypeFor[MediaSourceInfo]() && name == "DirectStreamUrl" {
					if !safeDirectURL(v.FieldByName("Path").String(), field.String()) {
						return false
					}
					continue
				}
				if v.Type() == reflect.TypeFor[MediaSourceInfo]() && name == "Path" {
					if !safeStreamURL(field.String()) {
						return false
					}
					continue
				}
				if v.Type() == reflect.TypeFor[MediaStream]() && name == "DeliveryUrl" {
					if field.String() != "" && (!v.FieldByName("IsExternal").Bool() || v.FieldByName("DeliveryMethod").String() != "External" || !safeSubtitleURL(field.String())) {
						return false
					}
					continue
				}
				if name == "ProviderIds" || name == "ImageTags" {
					for _, key := range field.MapKeys() {
						allowed := " Tmdb Imdb "
						if name == "ImageTags" {
							allowed = " Primary Backdrop Thumb Logo "
						}
						if !strings.Contains(allowed, " "+key.String()+" ") {
							return false
						}
					}
				}
				if !visit(field) {
					return false
				}
			}
		case reflect.String:
			text := strings.ToLower(v.String())
			if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "\\") || strings.HasPrefix(text, "http:") || strings.HasPrefix(text, "https:") || strings.HasPrefix(text, "file:") || strings.HasPrefix(text, "smb:") {
				return false
			}
			for _, marker := range []string{"/115/", "/root/", "/data/", "/mnt/", "\\115\\", "strm302", "pickcode=", "pick_code=", "apikey=", "api_key=", "filekey=", ".115cdn.net/"} {
				if strings.Contains(text, marker) {
					return false
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				if !visit(v.Index(i)) {
					return false
				}
			}
		case reflect.Map:
			for _, key := range v.MapKeys() {
				if !visit(key) || !visit(v.MapIndex(key)) {
					return false
				}
			}
		}
		return true
	}
	if !visit(reflect.ValueOf(value)) {
		return ErrInvalid
	}
	return nil
}
func validateCapabilities(flags map[string]json.RawMessage) error {
	for key, raw := range flags {
		switch strings.ToLower(key) {
		case "type":
			var text string
			if json.Unmarshal(raw, &text) != nil || text != "Default" {
				return ErrInvalid
			}
		case "protocol":
			var text string
			if json.Unmarshal(raw, &text) != nil || text != "Http" {
				return ErrInvalid
			}
		case "isremote", "supportsdirectplay", "supportsdirectstream":
			if !bytes.Equal(bytes.TrimSpace(raw), []byte("true")) {
				return ErrInvalid
			}
		case "supportstranscoding", "requiresopening", "requiresclosing":
			if !bytes.Equal(bytes.TrimSpace(raw), []byte("false")) {
				return ErrInvalid
			}
		case "transcodingurl":
			return ErrInvalid
		}
	}
	return nil
}

func safeSubtitleURL(value string) bool {
	u, e := url.Parse(value)
	if e != nil {
		return false
	}
	q := u.Query()
	id := q.Get("SubtitleId")
	if len(q["SubtitleId"]) != 1 || len(id) != 36 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) != 7 || parts[1] != "Videos" || parts[3] != q.Get("MediaSourceId") || parts[4] != "Subtitles" || !strings.HasPrefix(parts[6], "Stream.") {
		return false
	}
	n, e := strconv.Atoi(parts[5])
	if e != nil || n < 0 {
		return false
	}
	ext := strings.TrimPrefix(parts[6], "Stream.")
	if len(ext) < 1 || len(ext) > 8 {
		return false
	}
	for _, c := range ext {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	q.Del("SubtitleId")
	u.Path = "/Videos/" + parts[2] + "/stream.mkv"
	u.RawQuery = q.Encode()
	direct := u.RequestURI()
	q.Del("api_key")
	u.RawQuery = q.Encode()
	return safeDirectURL(u.String(), direct)
}
