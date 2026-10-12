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

const MaxPackageBytes = 8 << 20
const MaxUnpackedBytes = 24 << 20

type SettingField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required,omitempty"`
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
}
type Package struct {
	UI       string   `json:"-"`
	Module   []byte   `json:"-"`
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
		if name != "manifest.json" && name != "settings.json" && name != "README.md" && name != "plugin.wasm" && name != "ui.html" {
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
	if Validate(out.Manifest) != nil {
		return out, ErrInvalid
	}
	out.UI = string(contents["ui.html"])
	if len(out.UI) > 256<<10 || len(out.UI) > 0 && !out.Manifest.CustomUI || out.Manifest.CustomUI && (out.Manifest.Runtime != "wasm" || len(out.UI) == 0) {
		return out, ErrInvalid
	}
	out.Module = contents["plugin.wasm"]
	if ValidateModule(context.Background(), out.Module) != nil {
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
		case "string", "password", "boolean", "number", "select", "textarea", "date":
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
func (r Registry) Install(ctx context.Context, p Package, revision int64) error {
	if r.DB == nil || Validate(p.Manifest) != nil || ValidateModule(ctx, p.Module) != nil || len(p.Digest) != 64 {
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
		_, e = tx.Exec(ctx, `INSERT INTO extension_plugins(id,name,version,manifest,package_digest,package_files,wasm_module,readme,ui_html)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, raw, p.Digest, files, p.Module, p.Readme, p.UI)
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
		tag, err := tx.Exec(ctx, `UPDATE extension_plugins SET name=$2,version=$3,manifest=$4,enabled=false,package_digest=$5,package_files=$6,config_ciphertext=$7,revision=revision+1,wasm_module=$9,readme=$10,ui_html=$11,updated_at=now()WHERE id=$1 AND revision=$8`, p.Manifest.ID, p.Manifest.Name, p.Manifest.Version, raw, p.Digest, files, cipher, revision, p.Module, p.Readme, p.UI)
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
		if item.Manifest.ID == id {
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
		if item.Manifest.ID != id {
			continue
		}
		if !item.Enabled {
			return nil, ErrInvalid
		}
		event := Event{Version: 1, Kind: "plugin.test", Data: map[string]any{"filename": filename, "test": true}}
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
