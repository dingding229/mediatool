package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Icons are image resources from the same pin, not inline HTML or remote URLs.
func exportIcon(moduleDir, id, name, destination string, check bool) (resource, error) {
	var out resource
	root, e := os.OpenRoot(filepath.Join(moduleDir, "plugins", id))
	if e != nil {
		return out, e
	}
	defer func() { _ = root.Close() }() // build-only descriptor teardown
	for rel := name; rel != "."; rel = filepath.Dir(rel) {
		st, e := root.Lstat(rel)
		if e != nil {
			return out, e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return out, errors.New("icon symlink prohibited")
		}
	}
	st, e := root.Stat(name)
	if e != nil {
		return out, e
	}
	if !st.Mode().IsRegular() || st.Size() > 64<<10 {
		return out, errors.New("icon type or size exceeds bound")
	}
	b, e := root.ReadFile(name)
	if e != nil {
		return out, e
	}
	if e = validateIcon(name, b); e != nil {
		return out, e
	}
	sum := sha256.Sum256(b)
	out = resource{id, filepath.ToSlash(name), hex.EncodeToString(sum[:])}
	if !check {
		target := filepath.Join(destination, id, name)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return out, e
		}
		e = os.WriteFile(target, b, 0644)
	}
	return out, e
}
func validateIcon(name string, b []byte) error {
	invalid := errors.New("only bounded static SVG/PNG/WebP icons are supported")
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".webp":
		typ := http.DetectContentType(b)
		if typ != "image/png" && typ != "image/webp" {
			return invalid
		}
		return nil
	case ".svg":
		d := xml.NewDecoder(bytes.NewReader(b))
		count := 0
		for {
			token, e := d.Token()
			if e == io.EOF {
				break
			}
			if e != nil {
				return invalid
			}
			switch v := token.(type) {
			case xml.ProcInst:
				if v.Target != "xml" {
					return invalid
				}
			case xml.Directive:
				return invalid
			case xml.StartElement:
				count++
				if count > 256 {
					return invalid
				}
				switch v.Name.Local {
				case "svg", "g", "path", "rect", "circle", "ellipse", "line", "polygon", "polyline", "title", "desc":
				default:
					return invalid
				}
				if count == 1 && v.Name.Local != "svg" {
					return invalid
				}
				if v.Name.Space != "" && v.Name.Space != "http://www.w3.org/2000/svg" {
					return invalid
				}
				for _, a := range v.Attr {
					k := strings.ToLower(a.Name.Local)
					if strings.HasPrefix(k, "on") || k == "href" || k == "src" || k == "style" || strings.Contains(strings.ToLower(a.Value), "url(") {
						return invalid
					}
				}
			}
		}
		if count == 0 {
			return invalid
		}
		return nil
	}
	return invalid
}
