package api_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var parameterPattern = regexp.MustCompile(`\{([^}]+)\}`)

type object = map[string]any
type source struct {
	set        *token.FileSet
	info       *types.Info
	files      []*ast.File
	funcs      map[*types.Func]*ast.FuncDecl
	schemas    object
	building   map[string]bool
	pkg        *types.Package
	kindValue  int64
	nativeMode bool
}
type routeDoc struct {
	pattern, handler, location string
	externalScope              string
	expr                       ast.Expr
	fn                         *ast.FuncDecl
	admin                      bool
}

func loadSource(t *testing.T) *source { return loadPackage(t, "media-tool/internal/platform/httpapi") }
func loadPackage(t *testing.T, target string) *source {
	t.Helper()
	cmd := exec.Command("go", "list", "-export", "-deps", "-json", target)
	cmd.Dir = os.Getenv("APICONTRACT_ROOT")
	if cmd.Dir == "" {
		cmd.Dir = ".."
	}
	raw, e := cmd.Output()
	if e != nil {
		t.Fatalf("load Go export metadata: %v", e)
	}
	exports := map[string]string{}
	var dir string
	var names []string
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		var p struct {
			ImportPath, Export, Dir string
			GoFiles                 []string
		}
		e = dec.Decode(&p)
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		exports[p.ImportPath] = p.Export
		if p.ImportPath == target {
			dir = p.Dir
			names = p.GoFiles
		}
	}
	s := &source{set: token.NewFileSet(), info: &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}}, funcs: map[*types.Func]*ast.FuncDecl{}, schemas: object{}, building: map[string]bool{}}
	for _, name := range names {
		f, e := parser.ParseFile(s.set, filepath.Join(dir, name), nil, parser.ParseComments)
		if e != nil {
			t.Fatal(e)
		}
		s.files = append(s.files, f)
	}
	imp := importer.ForCompiler(s.set, "gc", func(path string) (io.ReadCloser, error) { return os.Open(exports[path]) })
	cfg := types.Config{Importer: imp}
	s.pkg, e = cfg.Check(target, s.set, s.files, s.info)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range s.files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok {
				if obj, ok := s.info.Defs[fn.Name].(*types.Func); ok {
					s.funcs[obj] = fn
				}
			}
		}
	}
	return s
}
func unquote(e ast.Expr) (string, bool) {
	if x, ok := e.(*ast.BasicLit); ok && x.Kind == token.STRING {
		v, e := strconv.Unquote(x.Value)
		return v, e == nil
	}
	return "", false
}
func (s *source) str(e ast.Expr) string {
	if v, ok := unquote(e); ok {
		return v
	}
	if v := s.info.Types[e].Value; v != nil && v.Kind() == constant.String {
		return constant.StringVal(v)
	}
	return ""
}
func (s *source) callee(e ast.Expr) *ast.FuncDecl {
	var o types.Object
	switch x := e.(type) {
	case *ast.Ident:
		o = s.info.Uses[x]
	case *ast.SelectorExpr:
		if v := s.info.Selections[x]; v != nil {
			o = v.Obj()
		} else {
			o = s.info.Uses[x.Sel]
		}
	}
	if f, ok := o.(*types.Func); ok {
		return s.funcs[f]
	}
	return nil
}
func selector(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return x.Sel.Name
	}
	return ""
}
func registeredPatterns(s *source) []string {
	reachable := map[*ast.FuncDecl]bool{}
	var visit func(*ast.FuncDecl)
	visit = func(fn *ast.FuncDecl) {
		if fn == nil || reachable[fn] {
			return
		}
		reachable[fn] = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				visit(s.callee(c.Fun))
			}
			return true
		})
	}
	for _, fn := range s.funcs {
		if fn.Name.Name == "New" {
			visit(fn)
		}
	}
	set := map[string]bool{}
	for _, r := range s.routes(nil) {
		for _, fn := range s.funcs {
			if reachable[fn] && r.expr.Pos() >= fn.Pos() && r.expr.End() <= fn.End() {
				set[r.pattern] = true
			}
		}
	}
	out := []string{}
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
func (s *source) routes(t *testing.T) []routeDoc {
	var out []routeDoc
	for _, f := range s.files {
		env := map[string][]string{}
		ast.Inspect(f, func(n ast.Node) bool {
			r, ok := n.(*ast.RangeStmt)
			if !ok {
				return true
			}
			id, ok := r.Value.(*ast.Ident)
			if !ok {
				return true
			}
			if list, ok := r.X.(*ast.CompositeLit); ok {
				for _, v := range list.Elts {
					if x, ok := v.(ast.Expr); ok {
						if text, yes := unquote(x); yes {
							env[id.Name] = append(env[id.Name], text)
						}
					}
				}
			}
			return true
		})
		var eval func(ast.Expr) []string
		eval = func(e ast.Expr) []string {
			if text := s.str(e); text != "" {
				return []string{text}
			}
			switch x := e.(type) {
			case *ast.Ident:
				return env[x.Name]
			case *ast.BinaryExpr:
				if x.Op == token.ADD {
					var texts []string
					for _, a := range eval(x.X) {
						for _, b := range eval(x.Y) {
							texts = append(texts, a+b)
						}
					}
					return texts
				}
			}
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || len(c.Args) != 2 || selector(c.Fun) != "HandleFunc" && selector(c.Fun) != "Handle" {
				return true
			}
			patterns := eval(c.Args[0])
			for _, pattern := range patterns {
				r := routeDoc{pattern: pattern, expr: c.Args[1], location: filepath.Base(s.set.Position(c.Pos()).Filename) + ":" + strconv.Itoa(s.set.Position(c.Pos()).Line)}
				var unwrap func(ast.Expr)
				unwrap = func(e ast.Expr) {
					switch x := e.(type) {
					case *ast.CallExpr:
						name := selector(x.Fun)
						if name == "external" && len(x.Args) > 1 {
							r.externalScope = s.str(x.Args[0])
							unwrap(x.Args[1])
							return
						}
						if name == "protected" {
							r.admin = true
						}
						if (name == "protected" || name == "HandlerFunc" || name == "longStream") && len(x.Args) > 0 {
							unwrap(x.Args[0])
							return
						}
						if fn := s.callee(x.Fun); fn != nil {
							r.fn = fn
							r.handler = fn.Name.Name
						}
					case *ast.FuncLit:
						r.fn = &ast.FuncDecl{Body: x.Body}
						r.handler = "anonymous"
					default:
						if fn := s.callee(e); fn != nil {
							r.fn = fn
							r.handler = fn.Name.Name
						}
					}
				}
				unwrap(c.Args[1])
				if r.handler == "" {
					r.handler = "mount"
				}
				out = append(out, r)
			}
			return true
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].pattern < out[j].pattern })
	return out
}
func schemaName(n *types.Named) string {
	p := "builtin"
	if n.Obj().Pkg() != nil {
		p = strings.TrimPrefix(n.Obj().Pkg().Path(), "media-tool/internal/")
	}
	raw := p + "_" + n.Obj().Name()
	if n.TypeArgs() != nil {
		for i := 0; i < n.TypeArgs().Len(); i++ {
			raw += "_" + types.TypeString(n.TypeArgs().At(i), func(p *types.Package) string { return p.Name() })
		}
	}
	return regexp.MustCompile(`[^a-zA-Z0-9_.-]`).ReplaceAllString(raw, "_")
}
func ref(name string) object { return object{"$ref": "#/components/schemas/" + name} }
func rawJSON() object {
	return object{"description": "真实开放JSON值（动态插件、元数据或数据库单元格），不代表固定DTO缺少字段定义。", "oneOf": []any{object{"type": "object", "additionalProperties": true}, object{"type": "array", "items": object{}}, object{"type": "string"}, object{"type": "number"}, object{"type": "boolean"}, object{"type": "null"}}}
}
func (s *source) schema(t types.Type) object {
	if t == nil {
		return rawJSON()
	}
	t = types.Unalias(t)
	switch x := t.(type) {
	case *types.Pointer:
		return object{"anyOf": []any{s.schema(x.Elem()), object{"type": "null"}}}
	case *types.Named:
		if x.Obj().Pkg() != nil {
			p := x.Obj().Pkg().Path()
			if p == "time" && x.Obj().Name() == "Time" || p == "media-tool/internal/modules/embyapi/dto" && x.Obj().Name() == "UTCDateTime" {
				return object{"type": "string", "format": "date-time"}
			}
			if p == "encoding/json" && x.Obj().Name() == "RawMessage" {
				return rawJSON()
			}
		}
		name := schemaName(x)
		if _, ok := s.schemas[name]; !ok {
			s.schemas[name] = object{}
			body := s.schema(x.Underlying())
			if x.Obj().Pkg() != nil && x.Obj().Pkg().Path() == "media-tool/internal/modules/embyapi/dto" && x.Obj().Name() == "Array" {
				body["type"] = "array"
			}
			s.schemas[name] = body
			body["x-go-type"] = types.TypeString(x, func(p *types.Package) string { return p.Path() })
			if b, ok := x.Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 && x.Obj().Pkg() != nil {
				var enums []string
				for _, key := range x.Obj().Pkg().Scope().Names() {
					if c, ok := x.Obj().Pkg().Scope().Lookup(key).(*types.Const); ok && types.Identical(c.Type(), x) && c.Val().Kind() == constant.String {
						enums = append(enums, constant.StringVal(c.Val()))
					}
				}
				if len(enums) > 0 {
					sort.Strings(enums)
					body["enum"] = enums
				}
			}
		}
		return ref(name)
	case *types.Struct:
		props := object{}
		required := []string{}
		for i := 0; i < x.NumFields(); i++ {
			f := x.Field(i)
			if !f.Exported() {
				continue
			}
			tag := reflect.StructTag(x.Tag(i)).Get("json")
			parts := strings.Split(tag, ",")
			key := parts[0]
			if key == "-" {
				continue
			}
			if f.Embedded() && key == "" {
				body := s.deref(s.schema(f.Type()))
				if values, ok := body["properties"].(object); ok {
					for k, v := range values {
						props[k] = v
					}
				}
				if req, ok := body["required"].([]string); ok {
					required = append(required, req...)
				}
				continue
			}
			if key == "" {
				key = f.Name()
			}
			props[key] = s.schema(f.Type())
			if !strings.Contains(tag, "omitempty") {
				required = append(required, key)
			}
		}
		body := object{"type": "object", "properties": props, "additionalProperties": false}
		if len(required) > 0 {
			sort.Strings(required)
			body["required"] = required
		}
		return body
	case *types.Slice:
		if b, ok := x.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Uint8 {
			return object{"type": "string", "contentEncoding": "base64"}
		}
		return object{"type": []string{"array", "null"}, "items": s.schema(x.Elem())}
	case *types.Array:
		return object{"type": "array", "items": s.schema(x.Elem()), "minItems": x.Len(), "maxItems": x.Len()}
	case *types.Map:
		return object{"type": []string{"object", "null"}, "additionalProperties": s.schema(x.Elem())}
	case *types.Basic:
		switch {
		case x.Info()&types.IsBoolean != 0:
			return object{"type": "boolean"}
		case x.Info()&types.IsInteger != 0:
			return object{"type": "integer", "format": "int64"}
		case x.Info()&types.IsFloat != 0:
			return object{"type": "number"}
		case x.Info()&types.IsString != 0:
			return object{"type": "string"}
		}
	}
	return rawJSON()
}
func (s *source) deref(o object) object {
	if p, ok := o["$ref"].(string); ok {
		if b, ok := s.schemas[strings.TrimPrefix(p, "#/components/schemas/")].(object); ok {
			return b
		}
	}
	return o
}
func (s *source) expr(e ast.Expr, fn *ast.FuncDecl, seen map[types.Object]bool) object {
	if lit, ok := e.(*ast.CompositeLit); ok {
		t := s.info.TypeOf(e)
		if m, ok := t.Underlying().(*types.Map); ok {
			if b, ok := m.Key().Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
				props := object{}
				req := []string{}
				for _, elt := range lit.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						key := s.str(kv.Key)
						if key != "" {
							props[key] = s.expr(kv.Value, fn, seen)
							req = append(req, key)
						}
					}
				}
				if len(props) > 0 {
					sort.Strings(req)
					return object{"type": "object", "properties": props, "required": req, "additionalProperties": true}
				}
			}
		}
	}
	if id, ok := e.(*ast.Ident); ok && fn != nil {
		obj := s.info.Uses[id]
		if obj != nil && !seen[obj] {
			next := map[types.Object]bool{}
			for k, v := range seen {
				next[k] = v
			}
			next[obj] = true
			var found ast.Expr
			var appended ast.Expr
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if a, ok := n.(*ast.AssignStmt); ok {
					for i, l := range a.Lhs {
						if x, ok := l.(*ast.Ident); ok && (s.info.Defs[x] == obj || s.info.Uses[x] == obj) && i < len(a.Rhs) {
							switch a.Rhs[i].(type) {
							case *ast.CompositeLit:
								found = a.Rhs[i]
							}
						}
					}
				}
				if c, ok := n.(*ast.CallExpr); ok && selector(c.Fun) == "append" && len(c.Args) >= 2 {
					if x, ok := c.Args[0].(*ast.Ident); ok && s.info.Uses[x] == obj {
						appended = c.Args[1]
					}
				}
				return true
			})
			if appended != nil {
				if _, ok := s.info.TypeOf(e).Underlying().(*types.Slice); ok {
					return object{"type": "array", "items": s.expr(appended, fn, next)}
				}
			}
			if found != nil {
				return s.expr(found, fn, next)
			}
		}
	}
	return s.schema(s.info.TypeOf(e))
}

type operation struct {
	responses map[int][]object
	errors    map[int]map[string]bool
	body      []object
	params    map[string]object
	raw       bool
}

func newOperation() *operation {
	return &operation{responses: map[int][]object{}, errors: map[int]map[string]bool{}, params: map[string]object{}}
}
func (s *source) code(e ast.Expr, fn *ast.FuncDecl) []int {
	if v := s.info.Types[e].Value; v != nil && v.Kind() == constant.Int {
		n, _ := constant.Int64Val(v)
		if n >= 100 && n < 600 {
			return []int{int(n)}
		}
	}
	if id, ok := e.(*ast.Ident); ok {
		obj := s.info.Uses[id]
		set := map[int]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if a, ok := n.(*ast.AssignStmt); ok {
				for i, l := range a.Lhs {
					if x, ok := l.(*ast.Ident); ok && (s.info.Defs[x] == obj || s.info.Uses[x] == obj) && i < len(a.Rhs) {
						if v := s.info.Types[a.Rhs[i]].Value; v != nil && v.Kind() == constant.Int {
							x, _ := constant.Int64Val(v)
							if x >= 100 && x < 600 {
								set[int(x)] = true
							}
						}
					}
				}
			}
			return true
		})
		var out []int
		for n := range set {
			out = append(out, n)
		}
		sort.Ints(out)
		if len(out) > 0 {
			return out
		}
	}
	return []int{200}
}
func (s *source) strings(e ast.Expr, fn *ast.FuncDecl) []string {
	if v := s.str(e); v != "" {
		return []string{v}
	}
	if id, ok := e.(*ast.Ident); ok {
		obj := s.info.Uses[id]
		set := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if a, ok := n.(*ast.AssignStmt); ok {
				for i, l := range a.Lhs {
					if x, ok := l.(*ast.Ident); ok && (s.info.Defs[x] == obj || s.info.Uses[x] == obj) && i < len(a.Rhs) {
						if v := s.str(a.Rhs[i]); v != "" {
							set[v] = true
						}
					}
				}
			}
			return true
		})
		var out []string
		for v := range set {
			out = append(out, v)
		}
		sort.Strings(out)
		return out
	}
	return nil
}
func (s *source) methodCondition(e ast.Expr, method string) (bool, bool) {
	b, ok := e.(*ast.BinaryExpr)
	if !ok {
		return false, false
	}
	if b.Op == token.LAND || b.Op == token.LOR {
		a, ak := s.methodCondition(b.X, method)
		c, ck := s.methodCondition(b.Y, method)
		if ak && ck {
			if b.Op == token.LAND {
				return a && c, true
			}
			return a || c, true
		}
	}
	if b.Op == token.EQL || b.Op == token.NEQ {
		sel, ok := b.X.(*ast.SelectorExpr)
		value := s.str(b.Y)
		yes := value != ""
		if ok && yes && sel.Sel.Name == "Method" {
			same := method == value
			if b.Op == token.NEQ {
				same = !same
			}
			return same, true
		}
	}
	return false, false
}
func (s *source) commonErrors(name string, o *operation) {
	for _, fn := range s.funcs {
		if fn.Name.Name != name {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok || selector(c.Fun) != "fail" || len(c.Args) < 4 {
				return true
			}
			for _, code := range s.code(c.Args[1], fn) {
				if code < 400 {
					code = 503
				}
				if o.errors[code] == nil {
					o.errors[code] = map[string]bool{}
				}
				for _, key := range s.strings(c.Args[2], fn) {
					o.errors[code][key] = true
				}
			}
			return true
		})
	}
}
func (s *source) collect(fn *ast.FuncDecl, method string, o *operation, visited map[*ast.FuncDecl]bool) {
	if fn == nil || visited[fn] {
		return
	}
	visited[fn] = true
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		if n == nil {
			return
		}
		if s.nativeMode {
			if sw, ok := n.(*ast.SwitchStmt); ok && kindExpr(sw.Tag) {
				var chosen, fallback *ast.CaseClause
				for _, item := range sw.Body.List {
					cc := item.(*ast.CaseClause)
					if len(cc.List) == 0 {
						fallback = cc
					}
					for _, e := range cc.List {
						if v := s.info.Types[e].Value; v != nil {
							n, _ := constant.Int64Val(v)
							if n == s.kindValue {
								chosen = cc
							}
						}
					}
				}
				if chosen == nil {
					chosen = fallback
				}
				if chosen != nil {
					for _, stmt := range chosen.Body {
						walk(stmt)
					}
				}
				return
			}
			if branch, ok := n.(*ast.IfStmt); ok {
				if b, ok := branch.Cond.(*ast.BinaryExpr); ok && (b.Op == token.EQL || b.Op == token.NEQ) && kindExpr(b.X) {
					if v := s.info.Types[b.Y].Value; v != nil {
						n, _ := constant.Int64Val(v)
						yes := n == s.kindValue
						if b.Op == token.NEQ {
							yes = !yes
						}
						if yes {
							walk(branch.Body)
						} else {
							walk(branch.Else)
						}
						return
					}
				}
			}
		}
		if branch, ok := n.(*ast.IfStmt); ok {
			if yes, known := s.methodCondition(branch.Cond, method); known {
				if yes {
					walk(branch.Body)
				} else {
					walk(branch.Else)
				}
				return
			}
		}
		if c, ok := n.(*ast.CallExpr); ok {
			name := selector(c.Fun)
			switch name {
			case "write":
				if len(c.Args) == 2 {
					o.responses[200] = append(o.responses[200], s.expr(c.Args[1], fn, map[types.Object]bool{}))
					return
				}
				if len(c.Args) == 3 {
					for _, code := range s.code(c.Args[1], fn) {
						o.responses[code] = append(o.responses[code], s.expr(c.Args[2], fn, map[types.Object]bool{}))
					}
				}
				return
			case "fail", "pipelineFailure":
				if len(c.Args) == 2 {
					for _, status := range s.code(c.Args[1], fn) {
						if status < 400 {
							status = 503
						}
						if o.errors[status] == nil {
							o.errors[status] = map[string]bool{}
						}
					}
					return
				}
				if len(c.Args) >= 4 {
					for _, code := range s.code(c.Args[1], fn) {
						if code < 400 {
							code = 503
						}
						if o.errors[code] == nil {
							o.errors[code] = map[string]bool{}
						}
						for _, key := range s.strings(c.Args[2], fn) {
							o.errors[code][key] = true
						}
					}
				}
				return
			case "decode", "decodeLimit", "decodeAuthBody":
				s.commonErrors("decodeLimit", o)
				if len(c.Args) >= 3 {
					ty := s.info.TypeOf(c.Args[2])
					if p, ok := ty.(*types.Pointer); ok {
						ty = p.Elem()
					}
					body := s.deref(s.schema(ty))
					copy := object{}
					for k, v := range body {
						if k != "required" {
							copy[k] = v
						}
					}
					copy["description"] = "字段级与条件必填校验由业务规则决定；确认字段只在明确批准时传true，不能自动补全。"
					o.body = append(o.body, copy)
				}
				return
			case "writeAuthError":
				if len(c.Args) > 1 {
					for _, status := range s.code(c.Args[1], fn) {
						if status >= 400 {
							o.errors[status] = map[string]bool{}
						}
					}
				}
				return
			case "WriteHeader":
				if s.nativeMode && len(c.Args) == 1 {
					for _, status := range s.code(c.Args[0], fn) {
						if status >= 400 {
							o.errors[status] = map[string]bool{}
						} else {
							o.responses[status] = nil
						}
					}
				}
				return
			case "Scalar", "Boolean", "list", "listParts", "Get", "Has":
				if len(c.Args) >= 1 {
					for _, key := range s.queryNames(c.Args[0], fn) {
						if key != "" {
							if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
								origin := s.info.TypeOf(sel.X)
								if origin != nil && (strings.Contains(types.TypeString(origin, nil), "url.Values") || s.nativeMode && strings.Contains(types.TypeString(origin, nil), "Values")) {
									schema := object{"type": "string"}
									if name == "Boolean" {
										schema["type"] = "boolean"
									}
									if name == "list" || name == "listParts" {
										schema = object{"type": "array", "items": object{"type": "string"}}
									}
									o.params[key] = object{"name": key, "in": "query", "required": false, "schema": schema}
									if name == "list" || name == "listParts" {
										o.params[key]["style"] = "form"
										o.params[key]["explode"] = false
									}
								}
							}
						}
					}
				}
			case "queryValue":
				if s.nativeMode && len(c.Args) > 1 {
					for _, key := range s.queryNames(c.Args[1], fn) {
						o.params[key] = object{"name": key, "in": "query", "required": false, "schema": object{"type": "string"}}
					}
				}
			case "queryInt":
				if len(c.Args) >= 5 {
					key := s.str(c.Args[1])
					schema := object{"type": "integer"}
					for i, label := range []string{"default", "minimum", "maximum"} {
						if value := s.info.Types[c.Args[i+2]].Value; value != nil {
							n, _ := constant.Int64Val(value)
							schema[label] = n
						}
					}
					o.params[key] = object{"name": key, "in": "query", "schema": schema}
				}
			case "Redirect":
				if len(c.Args) >= 4 {
					for _, code := range s.code(c.Args[3], fn) {
						o.responses[code] = nil
					}
					o.raw = true
				}
				return
			case "NotFound":
				o.responses[404] = nil
				o.raw = true
				return
			case "ServeContent", "ServeFile":
				o.responses[200] = nil
				o.responses[206] = nil
				o.raw = true
				return
			}
			if other := s.callee(c.Fun); other != nil {
				s.collect(other, method, o, visited)
			}
		}
		ast.Inspect(n, func(child ast.Node) bool {
			if child == n {
				return true
			}
			walk(child)
			return false
		})
	}
	walk(fn.Body)
}

func normalized(pattern string) []string {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok {
		path = pattern
		method = "*"
	}
	path = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\.\.\.\}`).ReplaceAllString(path, "{$1}")
	paths := []string{path}
	if strings.HasSuffix(path, "/") {
		paths = append(paths, path+"{path}")
	}
	methods := []string{strings.ToLower(method)}
	if method == "GET" {
		methods = append(methods, "head")
	}
	if method == "*" {
		methods = []string{"get", "head", "post", "put", "patch", "delete", "options", "trace"}
	}
	var out []string
	for _, p := range paths {
		for _, m := range methods {
			out = append(out, m+" "+p)
		}
	}
	return out
}
func uniqueSchemas(values []object) any {
	seen := map[string]bool{}
	var out []any
	for _, v := range values {
		raw, _ := json.Marshal(v)
		if !seen[string(raw)] {
			seen[string(raw)] = true
			out = append(out, v)
		}
	}
	if len(out) == 1 {
		return out[0]
	}
	return object{"anyOf": out}
}
func envelope(data any) object {
	return object{"type": "object", "additionalProperties": false, "required": []string{"data"}, "properties": object{"data": data}}
}
func domain(path string) string {
	for _, group := range []struct {
		tag      string
		prefixes []string
	}{
		{"外部凭据API", []string{"/api/external/"}},
		{"站点与PT账号", []string{"/api/v1/pt/"}},
		{"插件", []string{"/api/v1/plugins", "/api/v1/plugin/"}},
		{"云盘授权与连接", []string{"/api/v1/integrations", "/api/v1/readiness", "/api/v1/cloud"}},
		{"Emby兼容挂载", []string{"/emby"}},
		{"登录会话与账号", []string{"/api/v1/auth/"}},
		{"运维备份", []string{"/api/v1/ops"}},
		{"日志与数据视图", []string{"/api/v1/logs", "/api/v1/data/", "/api/v1/cache", "/api/v1/external-api/"}},
		{"搜索订阅", []string{"/api/v1/search", "/api/v1/subscription", "/api/v1/rss"}},
		{"下载整理链路", []string{"/api/v1/downloads", "/api/v1/transfers", "/api/v1/pipeline"}},
		{"原生媒体服务与用户", []string{"/api/v1/media-server/"}},
		{"媒体库与文件", []string{"/api/v1/media", "/api/v1/library", "/api/v1/files", "/api/v1/strm"}},
		{"通知", []string{"/api/v1/notification"}},
		{"系统设置", []string{"/api/v1/settings", "/api/v1/system", "/api/v1/classification"}},
	} {
		for _, prefix := range group.prefixes {
			if strings.HasPrefix(path, prefix) {
				return group.tag
			}
		}
	}
	return "基础与静态挂载"
}
func operationID(method, path string) string {
	sum := sha256.Sum256([]byte(path))
	return method + "_" + strings.Trim(regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(path, "_"), "_") + "_" + hex.EncodeToString(sum[:4])
}
func (s *source) build(t *testing.T) object {
	paths := object{}
	routes := s.routes(t)
	sourcePatterns := map[string]routeDoc{}
	for _, r := range routes {
		sourcePatterns[r.pattern] = r
	}
	actual := registeredPatterns(s)
	for _, p := range actual {
		if _, ok := sourcePatterns[p]; !ok {
			t.Fatalf("runtime registration lacks source contract: %s", p)
		}
	}
	schemes := object{"AdminSession": object{"type": "apiKey", "in": "cookie", "name": "media_tool_session", "description": "管理员持久会话Cookie；state-changing操作另需X-Requested-With及同源检查。"}, "ExternalAPIKey": object{"type": "http", "scheme": "bearer", "description": "独立永久API凭据，scope按具体操作限制，不接受管理员Cookie或STRM key。"}, "STRMKey": object{"type": "apiKey", "in": "query", "name": "apiKey", "description": "稳定STRM resolver授予或管理员配置key，不是临时CDN URL。"}, "EmbyToken": object{"type": "apiKey", "in": "header", "name": "X-Emby-Token", "description": "仅原生媒体用户token；兼容api_key查询别名详见FRONTEND_API。"}, "PluginAPIKey": object{"type": "apiKey", "in": "header", "name": "X-API-Key", "description": "仅descriptor.auth=apikey。"}, "PluginBearer": object{"type": "http", "scheme": "bearer", "description": "仅descriptor.auth=bearer，不能与X-API-Key混用。"}}
	s.schemas["ErrorEnvelope"] = object{"type": "object", "required": []string{"error"}, "properties": object{"error": object{"type": "object", "required": []string{"code", "message"}, "properties": object{"code": object{"type": "string"}, "message": object{"type": "string"}, "flow": object{"$ref": "#/components/schemas/FlowStatus"}}}}, "additionalProperties": false}
	s.schemas["FlowStatus"] = object{"type": "object", "required": []string{"phase", "retryable", "nextAction", "reason"}, "properties": object{"phase": object{"type": "string", "enum": []string{"approve", "complete", "download", "emby_refresh", "emby_verify", "execute", "inspect", "organize", "preview", "strm", "verify"}}, "retryable": object{"type": "boolean"}, "nextAction": object{"type": "string", "description": "动作提示，不是授权；unknown不得自动重放。"}, "reason": object{"type": "string"}}}
	seen := map[string]bool{}
	for _, pattern := range actual {
		r := sourcePatterns[pattern]
		for _, key := range normalized(pattern) {
			if seen[key] {
				continue
			}
			seen[key] = true
			method, path, _ := strings.Cut(key, " ")
			o := newOperation()
			if r.admin {
				s.commonErrors("protected", o)
			}
			if r.externalScope != "" {
				s.commonErrors("external", o)
			}
			s.collect(r.fn, strings.ToUpper(method), o, map[*ast.FuncDecl]bool{})
			if r.fn == nil {
				if lit, ok := r.expr.(*ast.FuncLit); ok {
					s.collect(&ast.FuncDecl{Body: lit.Body}, strings.ToUpper(method), o, map[*ast.FuncDecl]bool{})
				}
			}
			security := []any{}
			if r.admin {
				security = append(security, object{"AdminSession": []string{}})
			}
			if strings.HasPrefix(path, "/api/external/") {
				security = []any{object{"ExternalAPIKey": []string{}}}
			}
			if strings.HasPrefix(path, "/emby") {
				security = []any{object{"EmbyToken": []string{}}, object{}}
			}
			if strings.Contains(path, "strm302") {
				security = []any{object{"STRMKey": []string{}}}
				o.responses[302] = nil
				o.raw = true
			}
			if strings.Contains(path, "/api/v1/plugin/") {
				security = []any{object{"AdminSession": []string{}}, object{"PluginAPIKey": []string{}}, object{"PluginBearer": []string{}}, object{}}
			}
			op := object{"operationId": operationID(method, path), "tags": []string{domain(path)}, "summary": r.handler, "description": "当前注册入口；业务调用顺序、确认/幂等/unknown处理见docs/FRONTEND_API.md。", "security": security, "x-listener": "management", "x-go-registration": pattern, "x-go-handler": r.handler, "x-source": r.location}
			if path == "/api/v1/external-api/keys" {
				if method == "post" {
					op["description"] = "创建成功的201回执仅此一次包含完整token，请立即安全保存；若结果未知，先读取无令牌列表核对，不自动重试创建。"
				} else {
					op["description"] = "仅返回凭据元数据和15字符前缀脱敏提示；不返回token、摘要或密文。明文仅在POST创建成功的201回执中返回一次。"
				}
			}
			if r.externalScope != "" {
				op["x-required-scope"] = r.externalScope
			}
			if !strings.Contains(pattern, " ") {
				op["x-any-http-method"] = true
			}
			params := []any{}
			for _, m := range regexp.MustCompile(`\{([^}]+)\}`).FindAllStringSubmatch(path, -1) {
				params = append(params, object{"name": m[1], "in": "path", "required": true, "description": "服务端路由参数；不得二次URL解码或传入目录穿越。", "schema": object{"type": "string"}})
			}
			names := []string{}
			for name := range o.params {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				params = append(params, o.params[name])
			}
			if (r.admin || strings.HasPrefix(path, "/api/v1/auth/")) && method != "get" && method != "head" {
				params = append(params, object{"name": "X-Requested-With", "in": "header", "required": true, "schema": object{"type": "string", "const": "media-tool"}, "description": "还须通过同源Origin/Fetch Metadata检查；不能以此头代替会话。"})
			}
			if path == "/api/v1/pipeline/{kind}/{id}/retry" {
				for _, p := range params {
					v := p.(object)
					if v["name"] == "kind" {
						v["schema"] = object{"type": "string", "enum": []string{"downloads", "transfers"}}
					}
				}
			}
			if len(params) > 0 {
				op["parameters"] = params
			}
			if len(o.body) > 0 && method != "get" && method != "head" {
				if len(o.body) == 1 {
					props, _ := o.body[0]["properties"].(object)
					keys := []string{}
					if props["username"] != nil && props["password"] != nil {
						keys = []string{"username", "password"}
					}
					for _, codes := range o.errors {
						if codes["confirmation_required"] && props["confirm"] != nil {
							keys = append(keys, "confirm")
							props["confirm"].(object)["const"] = true
						}
					}
					if len(keys) > 0 {
						o.body[0]["required"] = keys
					}
				}
				op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": uniqueSchemas(o.body)}}}
			}
			responses := object{}
			for code, schemas := range o.responses {
				body := object{"description": httpDescription(code)}
				if len(schemas) > 0 && method != "head" {
					body["content"] = object{"application/json": object{"schema": envelope(uniqueSchemas(schemas))}}
				} else if code == 302 || code == 301 {
					body["headers"] = object{"Location": object{"description": "原始媒体临时URL；只向实际播放器302，不中转，不写入公开产物。", "schema": object{"type": "string", "format": "uri"}}}
				} else if method != "head" {
					mime := "application/octet-stream"
					if code >= 400 && o.raw {
						mime = "text/plain"
					}
					body["content"] = object{mime: object{"schema": object{"type": "string", "format": "binary"}}}
				}
				responses[strconv.Itoa(code)] = body
			}
			// Opaque mounted protocols are explicit; no fictitious generic JSON OK.
			if len(responses) == 0 && len(o.errors) == 0 {
				switch {
				case strings.HasPrefix(path, "/emby"):
					responses["default"] = object{"description": "外部Emby兼容网关；元数据/图片按上游协议，媒体仅严格302；此挂载不是原生media listener，也不是管理员Envelope。", "content": object{"application/json": object{"schema": rawJSON()}, "application/octet-stream": object{"schema": object{"type": "string", "format": "binary"}}}}
				case strings.HasPrefix(path, "/api/v1/plugin/"):
					responses["default"] = object{"description": "由当前同pin已授权descriptor决定；不把动态结果伪装为固定DTO。", "content": object{"application/json": object{"schema": rawJSON()}}}
				case path == "/" || path == "/{path}" || path == "/emby":
					responses["200"] = object{"description": "管理SPA/静态资源（非JSON API）", "content": object{"text/html": object{"schema": object{"type": "string"}}}}
				default:
					responses["default"] = object{"description": "该入口委托给专门协议/文件处理器；详见业务说明与source，不假造JSON成功结构。", "content": object{"application/json": object{"schema": rawJSON()}}}
				}
			}
			if strings.HasPrefix(path, "/api/v1/plugin/") {
				responses["default"] = object{"description": "动态descriptor响应：method/auth/body/状态与JSON由当前同pin已授权API定义决定。非管理员Envelope，不存在通用OK；先读取实例capabilities/metadata，不探测未声明路径。", "content": object{"application/json": object{"schema": rawJSON()}}}
				if method != "get" && method != "head" {
					op["requestBody"] = object{"required": false, "content": object{"application/json": object{"schema": rawJSON()}}}
				}
			}
			if path == "/" || path == "/{path}" {
				if method == "get" || method == "head" {
					responses = object{"200": object{"description": "管理SPA/静态资源（非JSON API）", "content": object{"text/html": object{"schema": object{"type": "string"}}}}}
					o.errors = map[int]map[string]bool{}
				} else {
					responses = object{}
					o.errors = map[int]map[string]bool{405: {"method_not_allowed": true}}
				}
			}
			if path == "/emby" && (method == "get" || method == "head") {
				responses["200"] = object{"description": "管理端硬刷新展示SPA；专用Emby网关listener根路径仍使用上游协议。", "content": object{"text/html": object{"schema": object{"type": "string"}}}}
			}
			if strings.Contains(path, "strm302") {
				for code, keys := range map[int][]string{400: {"not_media_file"}, 409: {"direct_redirect_required"}, 404: {"file_not_found"}, 502: {"unsafe_redirect", "storage_unavailable", "cloud_authentication_failed", "cloud_permission_denied", "cloud_rpc_unsupported", "cloud_direct_url_unavailable", "official_direct_validation_failed"}, 503: {"cloud_not_ready"}, 504: {"cloud_resolution_timeout"}, 501: {"storage_not_implemented"}} {
					if o.errors[code] == nil {
						o.errors[code] = map[string]bool{}
					}
					for _, k := range keys {
						o.errors[code][k] = true
					}
				}
			}
			if path == "/api/v1/downloads/manual" && method == "post" {
				op["requestBody"].(object)["content"].(object)["multipart/form-data"] = object{"schema": object{"type": "object", "required": []string{"torrent"}, "properties": object{"torrent": object{"type": "string", "format": "binary", "description": ".torrent，最大10MiB"}}, "additionalProperties": false}}
			}
			if path == "/api/v1/media/library/managed/{id}/metadata" && method == "patch" {
				props := object{}
				for _, key := range []string{"title", "originalTitle", "sortName", "overview", "tagline", "officialRating"} {
					max := 500
					if key == "overview" {
						max = 10000
					}
					if key == "tagline" {
						max = 1000
					}
					props[key] = object{"type": "string", "maxLength": max}
				}
				props["title"].(object)["minLength"] = 1
				props["year"] = object{"type": []string{"integer", "null"}, "minimum": 1800, "maximum": 2100}
				props["premiereDate"] = object{"type": []string{"string", "null"}, "format": "date"}
				props["communityRating"] = object{"type": []string{"number", "null"}, "minimum": 0, "maximum": 10, "description": "保存时四舍五入保留1位小数；null明确清空"}
				for _, key := range []string{"genres", "tags"} {
					props[key] = object{"type": "array", "maxItems": 50, "items": object{"type": "string", "maxLength": 100}, "description": "去首尾空白、去空项并去重"}
				}
				op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": object{"type": "object", "additionalProperties": false, "required": []string{"targetKey", "revision"}, "properties": object{
					"targetKey": object{"type": "string", "description": "GET返回的规范身份；绑定变化返回409"},
					"revision":  object{"type": "integer", "minimum": 0},
					"set":       object{"type": "object", "additionalProperties": false, "properties": props},
					"restore":   object{"type": "array", "uniqueItems": true, "items": object{"type": "string", "enum": []string{"title", "originalTitle", "sortName", "year", "premiereDate", "overview", "tagline", "genres", "tags", "communityRating", "officialRating"}}},
				}, "description": "set/restore至少一项；同字段不能同时修改和恢复。恢复解除锁定，采用最新已存刮削值，不触发联网获取。"}}}}
			}

			if path == "/api/v1/media/library/managed/{id}/images" && (method == "post" || method == "patch") {
				props := object{"targetKey": object{"type": "string"}, "revision": object{"type": "integer", "minimum": 0}, "kind": object{"type": "string", "enum": []string{"Primary", "Backdrop", "Logo"}}}
				required := []string{"targetKey", "revision", "kind"}
				mime := "application/json"
				if method == "post" {
					mime = "multipart/form-data"
					props["file"] = object{"type": "string", "format": "binary", "description": "JPG/PNG/WebP≤10MiB，真实格式与扩展名一致；≤1600万像素、单边≤8192，不支持动画；重新编码移除元数据"}
					required = append(required, "file")
				} else {
					props["path"] = object{"type": "string", "description": "空字符串恢复刮削值；非空只能是候选接口返回的TMDB路径，选中后服务端下载到本地"}
				}
				op["requestBody"] = object{"required": true, "content": object{mime: object{"schema": object{"type": "object", "required": required, "properties": props, "additionalProperties": false}}}}
				responses["409"] = object{"description": "图片revision或绑定已变化，请重新读取核对"}
				responses["429"] = object{"description": "解码并发已达上限2，稍后重试"}
			}
			if r.handler == "previewCron" {
				op["description"] = "管理员只读计算，上海时区。5/6/7字段；单次最多4096步、250毫秒，JSON体最多2048字节；业务最低间隔不放宽。有限计划最多返回5次，无未来时间时exhausted=true。"
				op["requestBody"] = object{"required": true, "content": object{"application/json": object{"schema": object{"type": "object", "required": []string{"expression", "purpose"}, "additionalProperties": false, "properties": object{"expression": object{"type": "string", "minLength": 1, "maxLength": 128, "description": "秒 分 时 日 月 周 年；5位省略秒和年，6位省略年；年1970～2199"}, "purpose": object{"type": "string", "enum": []string{"generic", "subscription", "scan", "site-metrics", "backup", "plugin"}}}}}}}
			}
			if r.handler == "uploadSubtitle" {
				op["requestBody"] = object{"required": true, "content": object{"multipart/form-data": object{"schema": object{"type": "object", "required": []string{"file", "uploadId"}, "properties": object{"file": object{"type": "string", "format": "binary", "description": "srt/ass/ssa/vtt，原文件和UTF-8转换后均≤5MiB"}, "encoding": object{"type": "string", "enum": []string{"auto", "utf-8", "utf-16le", "utf-16be", "utf-32le", "utf-32be", "gb18030", "big5", "shift-jis", "windows-1252"}}, "uploadId": object{"type": "string", "format": "uuid", "description": "客户端一次逻辑上传的稳定UUID，用于丢失响应后的幂等核对"}}, "additionalProperties": false}}}}
			}
			switch r.handler {
			case "opsDownload", "catalogImage", "tmdbPoster", "tmdbImage", "mediaImage", "ptTemplateIcon", "ptTemplateFavicon", "downloadSubtitle":
				mime := []string{"application/octet-stream"}
				switch r.handler {
				case "downloadSubtitle":
					mime = []string{"application/x-subrip", "text/x-ssa", "text/vtt"}
					responses["302"] = object{"description": "自动识别字幕仍302至115官方链接；只有已登记上传的≤5MiB UTF-8字幕可200直返。均需管理员cookie鉴权。", "headers": object{"Location": object{"schema": object{"type": "string", "format": "uri"}}}}
				case "opsDownload":
					mime = []string{"application/x-tar"}
				case "ptTemplateFavicon":
					mime = []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/x-icon", "image/vnd.microsoft.icon"}
				case "ptTemplateIcon":
					mime = []string{"image/x-icon"}
				case "catalogImage", "tmdbPoster", "tmdbImage", "mediaImage":
					mime = []string{"image/jpeg", "image/png", "image/webp"}
				}
				content := object{}
				for _, m := range mime {
					content[m] = object{"schema": object{"type": "string", "format": "binary"}}
				}
				responses["200"] = object{"description": "真实文件/图片响应，不包JSON Envelope；备份与STRM含私有数据，不可放公开产物。", "content": content}
				if r.handler == "ptTemplateIcon" || r.handler == "ptTemplateFavicon" || r.handler == "tmdbImage" || r.handler == "mediaImage" {
					responses["304"] = object{"description": "ETag匹配，无正文"}
					params = append(params, object{"name": "If-None-Match", "in": "header", "schema": object{"type": "string"}})
					op["parameters"] = params
				}
				if r.handler == "ptTemplateFavicon" {
					responses["206"] = object{"description": "图片字节范围，非Envelope", "content": content}
				}
				if r.handler == "tmdbImage" || r.handler == "mediaImage" {
					responses["502"] = object{"description": "固定图片获取错误，非Envelope", "content": object{"text/plain": object{"schema": object{"type": "string"}}}}
				}
			}
			if strings.Contains(path, "/media/assets/") && strings.Contains(path, "/subtitles") {
				for code, key := range map[int]string{400: "invalid_subtitle", 404: "subtitle_not_found", 409: "subtitle_conflict", 503: "subtitle_unavailable"} {
					if o.errors[code] == nil {
						o.errors[code] = map[string]bool{}
					}
					o.errors[code][key] = true
				}
			}
			if path == "/api/v1/jobs/events" {
				responses = object{"200": object{"description": "SSE事件流，客户端处理event/data与断线重连，不作为普通JSON响应解析。", "content": object{"text/event-stream": object{"schema": object{"type": "string"}}}}}
			}

			for code, codes := range o.errors {
				names := []string{}
				for name := range codes {
					names = append(names, name)
				}
				sort.Strings(names)
				responses[strconv.Itoa(code)] = object{"description": "错误Envelope；code按分支判断，不通过message猜测是否可重试。", "x-error-codes": names, "content": object{"application/json": object{"schema": errorSchema(names)}}}
			}
			if r.admin {
				for _, code := range []string{"401", "403"} {
					if _, ok := responses[code]; !ok {
						responses[code] = object{"description": "会话或CSRF/来源检查拒绝，无业务副作用。", "content": object{"application/json": object{"schema": ref("ErrorEnvelope")}}}
					}
				}
			}
			if strings.Contains(path, "strm302") {
				params = append(params, object{"name": "User-Agent", "in": "header", "required": true, "schema": object{"type": "string"}, "description": "实际播放器UA；浏览器不能伪造此头，不以浏览器探测代替播放器验收。"})
				for _, p := range params {
					if p.(object)["name"] == "apiKey" {
						p.(object)["required"] = true
					}
				}
				op["parameters"] = params
			}
			op["responses"] = responses
			if paths[path] == nil {
				paths[path] = object{}
			}
			paths[path].(object)[method] = op
		}
	}
	// Custom Marshaler adds flow to the persisted maintenance request projection.
	for _, value := range s.schemas {
		if m, ok := value.(object); ok {
			if name, _ := m["x-go-type"].(string); name == "media-tool/internal/modules/maintenance.Request" {
				if p, ok := m["properties"].(object); ok {
					p["flow"] = ref("FlowStatus")
					required, _ := m["required"].([]string)
					required = append(required, "flow")
					sort.Strings(required)
					m["required"] = required
				}
			}
		}
	}
	for _, value := range s.schemas {
		m, ok := value.(object)
		if !ok {
			continue
		}
		name, _ := m["x-go-type"].(string)
		props, _ := m["properties"].(object)
		if props == nil {
			continue
		}
		set := func(key string, values ...string) {
			if v, ok := props[key].(object); ok {
				v["enum"] = values
			}
		}
		switch name {
		case "media-tool/internal/modules/externalapi.Key":
			props["token"].(object)["description"] = "仅201创建成功时一次性返回；列表和后续查询不再返回。"
		case "media-tool/internal/modules/externalapi.KeySummary":
			m["description"] = "仅元数据及15字符前缀脱敏提示；不含明文、摘要或密文。"
		case "media-tool/internal/platform/flow.Status":
			m["properties"] = s.schemas["FlowStatus"].(object)["properties"]
		case "media-tool/internal/modules/integrations.Readiness":
			set("state", "not_configured", "configured", "authorization_required", "disabled", "check_required", "check_failed", "ready")
		case "media-tool/internal/modules/plugins/v2.Capability":
			set("state", "unsupported", "authorization_required", "disabled", "starting", "runtime_failed", "ready")
		case "media-tool/internal/modules/acquisition.Task":
			set("state", "pending", "added", "downloading", "local_complete", "strm_ready", "refresh_requested", "imported", "failed", "cancelled")
		case "media-tool/internal/modules/maintenance.Request":
			set("state", "preview", "approved", "executing", "completed", "failed", "cancelled", "expired")
		case "media-tool/internal/modules/jobs.Owned":
			set("state", "pending", "running", "retry", "succeeded", "failed", "paused", "cancelled", "unknown", "checking")
		}
	}

	return object{"openapi": "3.1.0", "info": object{"title": "Media Tool frontend and mounted protocol API", "version": "1.0.0", "description": "API 文档版本独立于程序构建版本；程序版本通过 /api/v1/system/version 读取。由实际路由注册与Go DTO派生；JSON是YAML1.2兼容写法，无新增运行依赖。schema79；说明与示例见docs/FRONTEND_API.md。"}, "servers": []any{object{"url": "/"}}, "paths": paths, "components": object{"schemas": s.schemas, "securitySchemes": schemes}, "x-protocol-boundaries": object{"emby": "挂载入口单列；原生Emby协议不是管理员Envelope。", "plugin": "仅声明宿主挂载；实际path/method/auth来自当前同pin运行descriptor，禁止远程代码。", "fallback": "/api/*为明确404；cloudStorage兼容前缀为明确501，不代表已实现功能。"}}
}
func httpDescription(code int) string {
	switch {
	case code == 302:
		return "严格302到原始CDN；不relay/HLS/转码。"
	case code >= 200 && code < 300:
		return "实际业务响应；排队/批准不等于执行完成。"
	default:
		return "按实际handler状态处理。"
	}
}
func generatedContract(t *testing.T) (*source, object) {
	t.Helper()
	s := loadSource(t)
	want := s.build(t)
	native := loadPackage(t, "media-tool/internal/modules/embyapi")
	native.schemas = s.schemas
	native.addNative(t, want)
	for _, schema := range s.schemas {
		m, ok := schema.(object)
		if !ok || m["x-go-type"] != "media-tool/internal/modules/embyapi/dto.MediaSourceInfo" {
			continue
		}
		props := m["properties"].(object)
		// Immutable MarshalJSON capabilities are not reflected struct fields.
		props["Type"] = object{"type": "string", "const": "Default"}
		props["Protocol"] = object{"type": "string", "const": "Http"}
		for _, k := range []string{"IsRemote", "SupportsDirectPlay", "SupportsDirectStream"} {
			props[k] = object{"type": "boolean", "const": true}
		}
		for _, k := range []string{"SupportsTranscoding", "RequiresOpening", "RequiresClosing"} {
			props[k] = object{"type": "boolean", "const": false}
		}
		required, _ := m["required"].([]string)
		unique := map[string]bool{}
		for _, k := range append(required, "Type", "Protocol", "IsRemote", "SupportsDirectPlay", "SupportsDirectStream", "SupportsTranscoding", "RequiresOpening", "RequiresClosing") {
			unique[k] = true
		}
		required = []string{}
		for k := range unique {
			required = append(required, k)
		}
		sort.Strings(required)
		m["required"] = required
		props["Path"].(object)["description"] = "配置媒体服务地址生成的无凭证稳定URL；客户端须提供媒体用户鉴权。不是STRM文件bearer或115临时URL。"
		props["DirectStreamUrl"].(object)["description"] = "同一native播放路由的相对URL，可携带当前媒体用户令牌；始终检查条目可见性与播放权限。"

	}
	annotateStateContracts(s.schemas)
	compactResponses(want)
	return s, want
}
func TestOpenAPIRoutesDTOAndGolden(t *testing.T) {
	s, want := generatedContract(t)
	raw, e := json.MarshalIndent(want, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	raw = append(raw, '\n')
	if os.Getenv("UPDATE_OPENAPI") == "1" {
		if e = os.WriteFile("openapi.yaml", raw, 0644); e != nil {
			t.Fatal(e)
		}
	}
	actual, e := os.ReadFile("openapi.yaml")
	if e != nil {
		t.Fatal(e)
	}
	var doc object
	if e = json.Unmarshal(actual, &doc); e != nil {
		t.Fatal("OpenAPI must be JSON-compatible YAML", e)
	}
	if !bytes.Equal(actual, raw) {
		t.Fatal("OpenAPI differs from router/Go DTO contract; run UPDATE_OPENAPI=1 go test ./api after reviewing the diff")
	}
	expected := map[string]bool{}
	for _, p := range registeredPatterns(s) {
		for _, key := range normalized(p) {
			expected[key] = true
		}
	}
	got := map[string]bool{}
	for p, value := range doc["paths"].(map[string]any) {
		for m := range value.(map[string]any) {
			if value.(map[string]any)[m].(map[string]any)["x-listener"] != "native-media" {
				got[m+" "+p] = true
			}
		}
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("bidirectional route mismatch: got=%d expected=%d", len(got), len(expected))
	}
	ids := map[string]bool{}
	for _, value := range doc["paths"].(map[string]any) {
		for _, value := range value.(map[string]any) {
			op := value.(map[string]any)
			id := op["operationId"].(string)
			if ids[id] {
				t.Fatal("duplicate operationId", id)
			}
			ids[id] = true
			if len(op["tags"].([]any)) == 0 || op["security"] == nil {
				t.Fatal(id)
			}
		}
	}
	if bytes.Contains(actual, []byte("#/components/responses/OK")) {
		t.Fatal("generic OK returned")
	}
	t.Logf("registered patterns=%d documented path/methods=%d schemas=%d", len(registeredPatterns(s)), len(expected), len(s.schemas))
}

func errorSchema(codes []string) object {
	if len(codes) == 0 {
		return ref("ErrorEnvelope")
	}
	return object{"allOf": []any{ref("ErrorEnvelope"), object{"properties": object{"error": object{"properties": object{"code": object{"type": "string", "enum": codes}}}}}}}
}

func (s *source) queryNames(e ast.Expr, fn *ast.FuncDecl) []string {
	if key := s.str(e); key != "" {
		return []string{key}
	}
	var id *ast.Ident
	field := ""
	switch x := e.(type) {
	case *ast.Ident:
		id = x
	case *ast.SelectorExpr:
		id, _ = x.X.(*ast.Ident)
		field = x.Sel.Name
	}
	if id == nil {
		return nil
	}
	obj := s.info.Uses[id]
	var out []string
	if field == "" {
		out = s.strings(e, fn)
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		r, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		v, ok := r.Value.(*ast.Ident)
		if !ok || s.info.Defs[v] != obj && s.info.Uses[v] != obj {
			return true
		}
		list, ok := r.X.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, item := range list.Elts {
			e, ok := item.(ast.Expr)
			if !ok {
				continue
			}
			if field == "" {
				if key := s.str(e); key != "" {
					out = append(out, key)
				}
				continue
			}
			row, ok := e.(*ast.CompositeLit)
			if !ok {
				continue
			}
			ty, ok := s.info.TypeOf(row).Underlying().(*types.Struct)
			if !ok {
				continue
			}
			index := -1
			for i := 0; i < ty.NumFields(); i++ {
				if ty.Field(i).Name() == field {
					index = i
				}
			}
			for i, item := range row.Elts {
				e, ok := item.(ast.Expr)
				if !ok {
					continue
				}
				if kv, ok := item.(*ast.KeyValueExpr); ok {
					if selector(kv.Key) != field {
						continue
					}
					e = kv.Value
				} else if i != index {
					continue
				}
				if key := s.str(e); key != "" {
					out = append(out, key)
				}
			}
		}
		return true
	})
	return out
}
