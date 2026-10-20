package check

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitializerGate(t *testing.T) {
	for _, tt := range []struct {
		code string
		ok   bool
	}{{`package p; var x = []string{"a"}`, true}, {`package p; var _ X=(*P)(nil)`, true}, {`package p; func init(){}`, false}, {`package p; var x = open()`, false}, {`package p; var x = func()int{return 1}()`, false}} {
		p := filepath.Join(t.TempDir(), "p.go")
		if e := os.WriteFile(p, []byte(tt.code), 0600); e != nil {
			t.Fatal(e)
		}
		if (auditFile(p) == nil) != tt.ok {
			t.Fatal(tt)
		}
	}
}
