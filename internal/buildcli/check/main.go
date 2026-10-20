// plugin-check audits the complete non-standard dependency closure of the pinned
// registry before release. It is a build tool, never invoked by the server.
package check

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type pkg struct {
	ImportPath, Dir   string
	Standard          bool
	GoFiles, CgoFiles []string
}

func Main() {
	if e := check(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func check() error {
	cmd := exec.Command("go", "list", "-deps", "-json", "github.com/dingding229/mediatool-plugin/registry")
	raw, e := cmd.StdoutPipe()
	if e != nil {
		return e
	}
	if e = cmd.Start(); e != nil {
		return e
	}
	decoder := json.NewDecoder(raw)
	var audit error
	for {
		var p pkg
		e = decoder.Decode(&p)
		if e == io.EOF {
			break
		}
		if e != nil {
			audit = e
			break
		}
		if p.Standard {
			continue
		}
		for _, name := range append(p.GoFiles, p.CgoFiles...) {
			if e = auditFile(filepath.Join(p.Dir, name)); e != nil && audit == nil {
				audit = e
			}
		}
	}
	if e = cmd.Wait(); e != nil {
		return e
	}
	return audit
}
func auditFile(path string) error {
	f, e := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if e != nil {
		return e
	}
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Recv == nil && x.Name.Name == "init" {
				return fmt.Errorf("custom init prohibited: %s", path)
			}
		case *ast.GenDecl:
			if x.Tok != token.VAR {
				continue
			}
			for _, spec := range x.Specs {
				v := spec.(*ast.ValueSpec)
				for _, expr := range v.Values {
					if !pure(expr) {
						return fmt.Errorf("non-pure top-level evaluation prohibited: %s", path)
					}
				}
			}
		}
	}
	return nil
}
func pure(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BasicLit, *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return pure(x.X)
	case *ast.ParenExpr:
		return pure(x.X)
	case *ast.UnaryExpr:
		return (x.Op == token.ADD || x.Op == token.SUB || x.Op == token.NOT || x.Op == token.XOR) && pure(x.X)
	case *ast.BinaryExpr:
		return pure(x.X) && pure(x.Y)
	case *ast.CompositeLit:
		for _, v := range x.Elts {
			if !pure(v) {
				return false
			}
		}
		return true
	case *ast.KeyValueExpr:
		return pure(x.Key) && pure(x.Value)
	case *ast.CallExpr: // Interface assertions such as (*Plugin)(nil) are type-only.
		if len(x.Args) != 1 {
			return false
		}
		id, ok := x.Args[0].(*ast.Ident)
		if !ok || id.Name != "nil" {
			return false
		}
		fn := x.Fun
		if p, ok := fn.(*ast.ParenExpr); ok {
			fn = p.X
		}
		_, ok = fn.(*ast.StarExpr)
		return ok
	}
	return false
}
