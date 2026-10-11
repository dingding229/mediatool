package downloads

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func encode(v any) []byte {
	var b bytes.Buffer
	switch value := v.(type) {
	case string:
		fmt.Fprintf(&b, "%d:%s", len(value), value)
	case int64:
		fmt.Fprintf(&b, "i%de", value)
	case map[string]any:
		b.WriteByte('d')
		keys := []string{}
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.Write(encode(k))
			b.Write(encode(value[k]))
		}
		b.WriteByte('e')
	case []any:
		b.WriteByte('l')
		for _, v := range value {
			b.Write(encode(v))
		}
		b.WriteByte('e')
	}
	return b.Bytes()
}
func TestV2AndHybridRawHashAndSafeManifest(t *testing.T) {
	info := map[string]any{"name": "Owned.Series", "meta version": int64(2), "piece length": int64(16384), "file tree": map[string]any{"Season 01": map[string]any{"Owned.Series.S01E01.mkv": map[string]any{"": map[string]any{"length": int64(100), "pieces root": strings.Repeat("a", 32)}}}}}
	rawInfo := encode(info)
	expected := sha256.Sum256(rawInfo)
	raw := encode(map[string]any{"info": info})
	meta, err := ParseTorrent(raw)
	if err != nil || meta.Version != 2 || meta.InfoHash != hex.EncodeToString(expected[:20]) || len(meta.Aliases) != 2 || len(meta.Files) != 1 || meta.Files[0].Name != "Season 01/Owned.Series.S01E01.mkv" {
		t.Fatal(meta, err)
	}
	info["pieces"] = strings.Repeat("b", 20)
	info["length"] = int64(100)
	meta, err = ParseTorrent(encode(map[string]any{"info": info}))
	if err != nil || len(meta.Aliases) != 3 {
		t.Fatal("hybrid aliases missing", meta, err)
	}
	info["file tree"] = map[string]any{"..": map[string]any{"": map[string]any{"length": int64(1), "pieces root": strings.Repeat("a", 32)}}}
	if _, err = ParseTorrent(encode(map[string]any{"info": info})); err == nil {
		t.Fatal("unsafe file tree accepted")
	}
	if _, err = ParseTorrent([]byte("<html>login</html>")); err == nil {
		t.Fatal("HTML accepted as torrent")
	}
}
