package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestT16RemovedEmbyEnvironmentNeverRead(t *testing.T) {
	keys := []string{"EMBY_GATEWAY_ADDR", "EMBY_PUBLIC_URL", "EMBY_UPSTREAM_URL", "EMBY_STRM_PATH_PREFIX", "EMBY_STRM_RESOLVER_ORIGINS", "EMBY_DIRECT_PLAY_ONLY", "EMBY_PROXY_ENABLED", "EMBY_302_ENABLED"}
	called := map[string]bool{}
	_, err := FromLookup(func(k string) (string, bool) { called[k] = true; return lookup(validEnv())(k) })
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if called[k] {
			t.Fatal("retired environment read", k)
		}
	}
	for _, root := range []string{"../../../internal", "../../../cmd"} {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			raw, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			for _, k := range keys {
				if strings.Contains(string(raw), `"`+k+`"`) {
					t.Errorf("retired variable remains in runtime %s: %s", p, k)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
