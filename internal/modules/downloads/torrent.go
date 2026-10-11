package downloads

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalidTorrent = errors.New("invalid, unsupported, or oversized torrent file")

type TorrentFile struct {
	Name string
	Size int64
}
type TorrentMetadata struct {
	InfoHash string
	Aliases  []string
	Version  int
	Name     string
	Files    []TorrentFile
}
type bdecoder struct {
	data       []byte
	pos, nodes int
	info       []byte
}

func (d *bdecoder) value(depth int) (any, error) {
	d.nodes++
	if depth > 64 || d.nodes > 100000 || d.pos >= len(d.data) {
		return nil, ErrInvalidTorrent
	}
	start := d.pos
	c := d.data[d.pos]
	switch {
	case c == 'i':
		d.pos++
		n := d.pos
		for d.pos < len(d.data) && d.data[d.pos] != 'e' {
			d.pos++
		}
		if d.pos >= len(d.data) {
			return nil, ErrInvalidTorrent
		}
		v, err := strconv.ParseInt(string(d.data[n:d.pos]), 10, 64)
		d.pos++
		if err != nil {
			return nil, ErrInvalidTorrent
		}
		return v, nil
	case c == 'l':
		d.pos++
		out := []any{}
		for d.pos < len(d.data) && d.data[d.pos] != 'e' {
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if d.pos >= len(d.data) {
			return nil, ErrInvalidTorrent
		}
		d.pos++
		return out, nil
	case c == 'd':
		d.pos++
		out := map[string]any{}
		for d.pos < len(d.data) && d.data[d.pos] != 'e' {
			key, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, ErrInvalidTorrent
			}
			if _, exists := out[name]; exists {
				return nil, ErrInvalidTorrent
			}
			valueStart := d.pos
			v, err := d.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out[name] = v
			if depth == 0 && name == "info" {
				d.info = d.data[valueStart:d.pos]
			}
		}
		if d.pos >= len(d.data) {
			return nil, ErrInvalidTorrent
		}
		d.pos++
		return out, nil
	case c >= '0' && c <= '9':
		for d.pos < len(d.data) && d.data[d.pos] >= '0' && d.data[d.pos] <= '9' {
			d.pos++
		}
		if d.pos >= len(d.data) || d.data[d.pos] != ':' {
			return nil, ErrInvalidTorrent
		}
		n, err := strconv.Atoi(string(d.data[start:d.pos]))
		d.pos++
		if err != nil || n < 0 || n > len(d.data)-d.pos {
			return nil, ErrInvalidTorrent
		}
		s := string(d.data[d.pos : d.pos+n])
		d.pos += n
		return s, nil
	default:
		return nil, ErrInvalidTorrent
	}
}
func ParseTorrent(data []byte) (TorrentMetadata, error) {
	if len(data) == 0 || len(data) > 10<<20 {
		return TorrentMetadata{}, ErrInvalidTorrent
	}
	decoder := bdecoder{data: data}
	raw, err := decoder.value(0)
	if err != nil || decoder.pos != len(data) || len(decoder.info) == 0 {
		return TorrentMetadata{}, ErrInvalidTorrent
	}
	dict, ok := raw.(map[string]any)
	if !ok {
		return TorrentMetadata{}, ErrInvalidTorrent
	}
	info, ok := dict["info"].(map[string]any)
	if !ok {
		return TorrentMetadata{}, ErrInvalidTorrent
	}
	name, _ := info["name"].(string)
	pieces, _ := info["pieces"].(string)
	version, _ := info["meta version"].(int64)
	if !validTorrentName(name) || (version != 2 && (len(pieces) == 0 || len(pieces)%20 != 0)) {
		return TorrentMetadata{}, ErrInvalidTorrent
	}
	hash := sha1.Sum(decoder.info)
	v2 := sha256.Sum256(decoder.info)
	files := []TorrentFile{}
	if version == 2 {
		pieceLength, ok := info["piece length"].(int64)
		if !ok || pieceLength < 16384 || pieceLength&(pieceLength-1) != 0 {
			return TorrentMetadata{}, ErrInvalidTorrent
		}
		tree, ok := info["file tree"].(map[string]any)
		if !ok || tree[""] != nil {
			return TorrentMetadata{}, ErrInvalidTorrent
		}
		var err error
		files, err = v2Files(tree, nil, 0)
		if err != nil || len(files) == 0 {
			return TorrentMetadata{}, ErrInvalidTorrent
		}
	} else if entries, ok := info["files"].([]any); ok {
		for _, entry := range entries {
			item, ok := entry.(map[string]any)
			if !ok {
				return TorrentMetadata{}, ErrInvalidTorrent
			}
			size, ok := item["length"].(int64)
			if !ok || size < 0 {
				return TorrentMetadata{}, ErrInvalidTorrent
			}
			parts, ok := item["path"].([]any)
			if !ok || len(parts) == 0 {
				return TorrentMetadata{}, ErrInvalidTorrent
			}
			names := []string{name}
			for _, part := range parts {
				s, ok := part.(string)
				if !ok || !validTorrentName(s) {
					return TorrentMetadata{}, ErrInvalidTorrent
				}
				names = append(names, s)
			}
			files = append(files, TorrentFile{Name: strings.Join(names, "/"), Size: size})
		}
	} else {
		size, ok := info["length"].(int64)
		if !ok || size < 0 {
			return TorrentMetadata{}, ErrInvalidTorrent
		}
		files = append(files, TorrentFile{Name: name, Size: size})
	}
	primary := hex.EncodeToString(hash[:])
	aliases := []string{primary}
	protocol := 1
	if version == 2 {
		protocol = 2
		primary = hex.EncodeToString(v2[:20])
		aliases = []string{primary, hex.EncodeToString(v2[:])}
		if len(pieces) > 0 && len(pieces)%20 == 0 {
			aliases = append(aliases, hex.EncodeToString(hash[:]))
		}
	}
	return TorrentMetadata{InfoHash: primary, Aliases: aliases, Version: protocol, Name: name, Files: files}, nil
}

func validTorrentName(s string) bool {
	return s != "" && s != "." && s != ".." && utf8.ValidString(s) && !strings.ContainsAny(s, "/\\\x00")
}
func v2Files(tree map[string]any, prefix []string, depth int) ([]TorrentFile, error) {
	if depth > 64 {
		return nil, ErrInvalidTorrent
	}
	out := []TorrentFile{}
	keys := []string{}
	for key := range tree {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, name := range keys {
		if !validTorrentName(name) {
			return nil, ErrInvalidTorrent
		}
		child, ok := tree[name].(map[string]any)
		if !ok {
			return nil, ErrInvalidTorrent
		}
		parts := append(append([]string{}, prefix...), name)
		if raw, leaf := child[""]; leaf {
			if len(child) != 1 {
				return nil, ErrInvalidTorrent
			}
			info, ok := raw.(map[string]any)
			if !ok {
				return nil, ErrInvalidTorrent
			}
			size, ok := info["length"].(int64)
			if !ok || size < 0 {
				return nil, ErrInvalidTorrent
			}
			root, _ := info["pieces root"].(string)
			if size > 0 && len(root) != 32 {
				return nil, ErrInvalidTorrent
			}
			attr, _ := info["attr"].(string)
			if strings.Contains(attr, "l") {
				return nil, ErrInvalidTorrent
			}
			if strings.Contains(attr, "p") {
				continue
			}
			out = append(out, TorrentFile{Name: strings.Join(parts, "/"), Size: size})
		} else {
			nested, err := v2Files(child, parts, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, nested...)
		}
		if len(out) > 100000 {
			return nil, ErrInvalidTorrent
		}
	}
	return out, nil
}
