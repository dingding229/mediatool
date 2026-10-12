package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxPackageBytes = 2 << 20
const MaxUnpackedBytes = 4 << 20

type SettingField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
}
type NameRule struct {
	Pattern   string `json:"pattern"`
	Title     string `json:"title,omitempty"`
	Year      string `json:"year,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}
type Package struct {
	Manifest Manifest `json:"manifest"`
	Digest   string   `json:"digest"`
	Files    []string `json:"files"`
	Readme   string   `json:"readme"`
}

func ParsePackage(data []byte) (Package, error) {
	out := Package{Files: []string{}}
	if len(data) == 0 || len(data) > MaxPackageBytes {
		return out, ErrInvalid
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) < 1 || len(z.File) > 16 {
		return out, ErrInvalid
	}
	seen := map[string]bool{}
	size := uint64(0)
	contents := map[string][]byte{}
	for _, file := range z.File {
		name := file.Name
		if name != path.Clean(name) || strings.ContainsAny(name, "\\\x00\r\n") || strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || seen[name] || !file.Mode().IsRegular() {
			return out, ErrInvalid
		}
		if name != "manifest.json" && name != "settings.json" && name != "rules.json" && name != "README.md" {
			return out, ErrInvalid
		}
		seen[name] = true
		if file.UncompressedSize64 > MaxUnpackedBytes || size > MaxUnpackedBytes-file.UncompressedSize64 {
			return out, ErrInvalid
		}
		size += file.UncompressedSize64
		reader, e := file.Open()
		if e != nil {
			return out, ErrInvalid
		}
		raw, e := io.ReadAll(io.LimitReader(reader, MaxUnpackedBytes+1))
		reader.Close()
		if e != nil || len(raw) > MaxUnpackedBytes {
			return out, ErrInvalid
		}
		contents[name] = raw
		out.Files = append(out.Files, name)
	}
	if strictJSON(contents["manifest.json"], &out.Manifest) != nil {
		return out, ErrInvalid
	}
	if raw, ok := contents["settings.json"]; ok {
		if len(out.Manifest.Settings) > 0 || strictJSON(raw, &out.Manifest.Settings) != nil {
			return out, ErrInvalid
		}
	}
	if raw, ok := contents["rules.json"]; ok {
		if len(out.Manifest.Rules) > 0 || strictJSON(raw, &out.Manifest.Rules) != nil {
			return out, ErrInvalid
		}
	}
	if Validate(out.Manifest) != nil {
		return out, ErrInvalid
	}
	out.Readme = string(contents["README.md"])
	if len(out.Readme) > 65536 {
		return out, ErrInvalid
	}
	sum := sha256.Sum256(data)
	out.Digest = hex.EncodeToString(sum[:])
	return out, nil
}
func strictJSON(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > 128<<10 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func SecretField(f SettingField) bool {
	return f.Type == "password" || regexp.MustCompile(`(?i)(password|token|secret|api.?key|authorization|cookie)`).MatchString(f.Key)
}

var settingKey = regexp.MustCompile(`^[a-z][a-zA-Z0-9_]{1,39}$`)

func validateFields(fields []SettingField) error {
	if len(fields) > 24 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, f := range fields {
		if !settingKey.MatchString(f.Key) || seen[f.Key] || len(f.Label) < 1 || len(f.Label) > 80 || len(f.Default) > 1000 || len(f.Options) > 20 {
			return ErrInvalid
		}
		seen[f.Key] = true
		switch f.Type {
		case "string", "password", "boolean", "number", "select":
		default:
			return ErrInvalid
		}
		if SecretField(f) && f.Default != "" {
			return ErrInvalid
		}
		if f.Type == "select" && len(f.Options) == 0 {
			return ErrInvalid
		}
		for _, option := range f.Options {
			if len(option) > 120 {
				return ErrInvalid
			}
		}
	}
	if _, err := ValidateSettings(fields, map[string]string{}, false); err != nil {
		return err
	}
	return nil
}
func ValidateSettings(fields []SettingField, in map[string]string, require bool) (map[string]string, error) {
	out := map[string]string{}
	known := map[string]bool{}
	for _, f := range fields {
		known[f.Key] = true
		v, exists := in[f.Key]
		if !exists {
			v = f.Default
		}
		if len(v) > 2000 || strings.ContainsRune(v, 0) {
			return nil, ErrInvalid
		}
		if require && f.Required && strings.TrimSpace(v) == "" {
			return nil, ErrInvalid
		}
		if v != "" {
			switch f.Type {
			case "boolean":
				if v != "true" && v != "false" {
					return nil, ErrInvalid
				}
			case "number":
				n, e := strconv.ParseFloat(v, 64)
				if e != nil || math.IsInf(n, 0) || math.IsNaN(n) {
					return nil, ErrInvalid
				}
			case "select":
				if !contains(f.Options, v) {
					return nil, ErrInvalid
				}
			}
		}
		out[f.Key] = v
	}
	for k := range in {
		if !known[k] {
			return nil, ErrInvalid
		}
	}
	return out, nil
}
func rulesOutput(m Manifest, event Event, settings map[string]string) (json.RawMessage, error) {
	name, _ := event.Data["filename"].(string)
	if event.Kind != "media.recognize" || len(name) > 1024 {
		return nil, ErrInvalid
	}
	for _, rule := range m.Rules {
		rx, e := regexp.Compile(rule.Pattern)
		if e != nil {
			return nil, ErrInvalid
		}
		matches := rx.FindStringSubmatchIndex(name)
		if matches == nil {
			continue
		}
		out := map[string]string{}
		for k, v := range map[string]string{"title": rule.Title, "year": rule.Year, "mediaType": rule.MediaType} {
			if v != "" {
				value := string(rx.ExpandString(nil, v, name, matches))
				for key, x := range settings {
					value = strings.ReplaceAll(value, "{{setting:"+key+"}}", x)
				}
				if len(value) > 200 || strings.ContainsAny(value, "/\\\r\n\x00") {
					return nil, ErrInvalid
				}
				out[k] = value
			}
		}
		return json.Marshal(out)
	}
	return json.RawMessage(`{}`), nil
}
func validateRules(m Manifest) error {
	if m.Runtime != "rules" {
		return nil
	}
	if m.Endpoint != "" || len(m.Rules) < 1 || len(m.Rules) > 32 || len(m.Capabilities) != 1 || m.Capabilities[0] != "media.recognize" {
		return ErrInvalid
	}
	for _, r := range m.Rules {
		for _, f := range m.Settings {
			if SecretField(f) && strings.Contains(r.Title+r.Year, "{{setting:"+f.Key+"}}") {
				return ErrInvalid
			}
		}
		if len(r.Pattern) > 512 || len(r.Title) > 200 || len(r.Year) > 80 || r.MediaType != "" && r.MediaType != "movie" && r.MediaType != "series" {
			return ErrInvalid
		}
		if _, e := regexp.Compile(r.Pattern); e != nil {
			return ErrInvalid
		}
	}
	return nil
}
func (r Registry) Install(ctx context.Context, p Package, revision int64) error {
	if r.DB == nil || Validate(p.Manifest) != nil || len(p.Digest) != 64 {
		return ErrInvalid
	}
	tx, e := r.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(p.Manifest)
	files, _ := json.Marshal(p.Files)
	if revision == 0 {
		_, e = tx.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest,package_digest,package_files)VALUES($1,$2,$3,$4,$5,$6)`, p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, raw, p.Digest, files)
	} else {
		var encrypted, oldRaw []byte
		if e = tx.QueryRow(ctx, `SELECT config_ciphertext,manifest FROM extension_plugins WHERE id=$1 AND revision=$2 FOR UPDATE`, p.Manifest.ID, revision).Scan(&encrypted, &oldRaw); e != nil {
			return ErrInvalid
		}
		config := map[string]string{}
		if len(encrypted) > 0 {
			if r.Vault == nil {
				return ErrInvalid
			}
			config, e = r.Vault.Open("plugin:"+p.Manifest.ID, encrypted)
			if e != nil {
				return e
			}
		}
		var old Manifest
		if json.Unmarshal(oldRaw, &old) != nil {
			return ErrInvalid
		}
		oldTypes := map[string]string{}
		for _, field := range old.Settings {
			oldTypes[field.Key] = field.Type
		}
		compatible := map[string]string{}
		for _, field := range p.Manifest.Settings {
			if v, ok := config[field.Key]; ok && oldTypes[field.Key] == field.Type {
				candidate := map[string]string{field.Key: v}
				_, e := ValidateSettings([]SettingField{field}, candidate, false)
				if e == nil {
					compatible[field.Key] = v
				}
			}
		}
		var cipher []byte
		if len(compatible) > 0 {
			cipher, e = r.Vault.Seal("plugin:"+p.Manifest.ID, compatible)
			if e != nil {
				return e
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE extension_plugins SET name=$2,version=$3,manifest=$4,enabled=false,package_digest=$5,package_files=$6,config_ciphertext=$7,revision=revision+1,updated_at=now()WHERE id=$1 AND revision=$8`, p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, raw, p.Digest, files, cipher, revision)
		e = err
		if e == nil && tag.RowsAffected() != 1 {
			return ErrInvalid
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (r Registry) Settings(ctx context.Context, id string) (map[string]string, []string, int64, error) {
	var raw, encrypted []byte
	var revision int64
	if r.DB == nil {
		return nil, nil, 0, ErrInvalid
	}
	e := r.DB.QueryRow(ctx, `SELECT manifest,config_ciphertext,revision FROM extension_plugins WHERE id=$1`, id).Scan(&raw, &encrypted, &revision)
	if e != nil {
		return nil, nil, 0, e
	}
	var m Manifest
	if json.Unmarshal(raw, &m) != nil {
		return nil, nil, 0, ErrInvalid
	}
	values := map[string]string{}
	if len(encrypted) > 0 {
		if r.Vault == nil {
			return nil, nil, 0, ErrInvalid
		}
		values, e = r.Vault.Open("plugin:"+id, encrypted)
		if e != nil {
			return nil, nil, 0, e
		}
	}
	values, e = ValidateSettings(m.Settings, values, false)
	secretKeys := []string{}
	for _, field := range m.Settings {
		if SecretField(field) {
			secretKeys = append(secretKeys, field.Key)
		}
	}
	return values, secretKeys, revision, e
}
func (r Registry) SaveSettings(ctx context.Context, id string, in map[string]string, clear []string, revision int64) error {
	if r.Vault == nil {
		return ErrInvalid
	}
	list, e := r.List(ctx)
	if e != nil {
		return e
	}
	var m Manifest
	for _, item := range list {
		if item.Manifest.ID == id && !item.Builtin {
			m = item.Manifest
		}
	}
	if m.ID == "" {
		return ErrInvalid
	}
	existing, _, rev, e := r.Settings(ctx, id)
	if e != nil || rev != revision {
		return ErrInvalid
	}
	secretKeys := map[string]bool{}
	for _, f := range m.Settings {
		if SecretField(f) {
			secretKeys[f.Key] = true
			if v, ok := in[f.Key]; ok && v == "" {
				delete(in, f.Key)
			}
		}
	}
	for k, v := range in {
		existing[k] = v
	}
	for _, k := range clear {
		if !secretKeys[k] {
			return ErrInvalid
		}
		existing[k] = ""
	}
	values, e := ValidateSettings(m.Settings, existing, false)
	if e != nil {
		return e
	}
	cipher, e := r.Vault.Seal("plugin:"+id, values)
	if e != nil {
		return e
	}
	tag, e := r.DB.Exec(ctx, `UPDATE extension_plugins SET config_ciphertext=$2,enabled=false,revision=revision+1,updated_at=now()WHERE id=$1 AND revision=$3`, id, cipher, revision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalid
	}
	return nil
}
func (r Registry) recordRun(ctx context.Context, id, kind, state, message string) {
	if r.DB == nil {
		return
	}
	r.DB.Exec(ctx, `INSERT INTO extension_runs(id,plugin_id,kind,state,message,finished_at)VALUES(gen_random_uuid(),$1,$2,$3,$4,now())`, id, kind, state, message)
	r.DB.Exec(ctx, `DELETE FROM extension_runs WHERE id IN(SELECT id FROM extension_runs WHERE plugin_id=$1 ORDER BY created_at DESC OFFSET 100)`, id)
}
func (r Registry) Test(ctx context.Context, id, filename string) (json.RawMessage, error) {
	items, e := r.List(ctx)
	if e != nil {
		return nil, e
	}
	for _, item := range items {
		if item.Manifest.ID != id || item.Builtin {
			continue
		}
		if !item.Enabled {
			return nil, ErrInvalid
		}
		kind := "media.recognize"
		event := Event{Version: 1, Kind: kind, Data: map[string]any{"filename": filename, "test": true}}
		if !contains(item.Manifest.Capabilities, kind) {
			event.Kind = "media.organized"
			event.Data = map[string]any{"test": true, "eventId": "manual-test", "items": []any{}}
		}
		bounded, stop := context.WithTimeout(ctx, 4*time.Second)
		defer stop()
		return r.invoke(bounded, item.Manifest, event)
	}
	return nil, fmt.Errorf("plugin unavailable")
}

func (r Registry) DeleteRevision(ctx context.Context, id string, revision int64) error {
	if r.DB == nil || !identifier.MatchString(id) || id == "release-name" || id == "naming-template" || revision < 1 {
		return ErrInvalid
	}
	tag, e := r.DB.Exec(ctx, `DELETE FROM extension_plugins WHERE id=$1 AND revision=$2`, id, revision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalid
	}
	return nil
}

// Sanitize using the exact settings snapshot used for invocation, even when an
// administrator changes/clears configuration while a remote hook is in flight.
func redactOutput(raw json.RawMessage, m Manifest, values map[string]string) (json.RawMessage, error) {
	var data any
	if len(raw) == 0 {
		return raw, nil
	}
	if json.Unmarshal(raw, &data) != nil {
		return nil, ErrInvalid
	}
	secrets := []string{}
	for _, f := range m.Settings {
		if SecretField(f) && values[f.Key] != "" {
			secrets = append(secrets, values[f.Key])
		}
	}
	cleanText := func(s string) string {
		for _, value := range secrets {
			s = strings.ReplaceAll(s, value, "[插件秘密已隐藏]")
		}
		return s
	}
	var clean func(any) any
	clean = func(v any) any {
		switch x := v.(type) {
		case string:
			return cleanText(x)
		case map[string]any:
			out := map[string]any{}
			for k, value := range x {
				out[cleanText(k)] = clean(value)
			}
			return out
		case []any:
			for i, value := range x {
				x[i] = clean(value)
			}
			return x
		default:
			return x
		}
	}
	return json.Marshal(clean(data))
}
