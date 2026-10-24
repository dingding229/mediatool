package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"go/ast"
	"go/constant"
	"go/types"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These are the actual immutable nativeEndpoints, not a separately maintained
// route list. Package-local runtime tests independently compare NewRouter's list.
type nativeRoute struct {
	method, path, kind, location string
	value                        int64
	public                       bool
}

func kindExpr(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name == "kind"
	case *ast.SelectorExpr:
		return x.Sel.Name == "kind"
	}
	return false
}
func (s *source) nativeRoutes(t *testing.T) []nativeRoute {
	var out []nativeRoute
	for _, fn := range s.funcs {
		if fn.Name.Name != "nativeEndpoints" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || len(lit.Elts) != 4 {
				return true
			}
			ty, ok := s.info.TypeOf(lit).(*types.Named)
			if !ok || ty.Obj().Name() != "endpoint" {
				return true
			}
			method, path := s.str(lit.Elts[0].(ast.Expr)), s.str(lit.Elts[1].(ast.Expr))
			if method == "" || path == "" {
				t.Fatal("unresolved native registration")
			}
			value, ok := constant.Int64Val(s.info.Types[lit.Elts[2].(ast.Expr)].Value)
			if !ok {
				t.Fatal("unresolved native kind")
			}
			out = append(out, nativeRoute{method, strings.ReplaceAll(path, "*", "{wildcard}"), selector(lit.Elts[2].(ast.Expr)), "endpoints.go:" + strconv.Itoa(s.set.Position(lit.Pos()).Line), value, constant.BoolVal(s.info.Types[lit.Elts[3].(ast.Expr)].Value)})
			return false
		})
	}
	if len(out) == 0 {
		t.Fatal("no native endpoints")
	}
	return out
}
func (s *source) addNative(t *testing.T, doc object) {
	paths := doc["paths"].(object)
	s.nativeMode = true
	s.schemas["NativeError"] = object{"type": "object", "required": []string{"ResponseStatus"}, "properties": object{"ResponseStatus": object{"type": "object", "required": []string{"ErrorCode", "Message"}, "properties": object{"ErrorCode": object{"type": "string"}, "Message": object{"type": "string"}}}, "ErrorCode": object{"type": "string"}, "Message": object{"type": "string"}}}
	var handle *ast.FuncDecl
	for _, fn := range s.funcs {
		if fn.Name.Name == "handle" {
			handle = fn
		}
	}
	for _, r := range s.nativeRoutes(t) {
		s.kindValue = r.value
		for _, key := range normalized(r.method + " " + r.path) {
			method, path, _ := strings.Cut(key, " ")
			o := newOperation()
			s.collect(handle, strings.ToUpper(method), o, map[*ast.FuncDecl]bool{})
			// Native request decoding uses bounded PascalCase protocol inputs, not the
			// management decoder; these values are data, never a permission to transcode.
			if method == "post" && (r.kind == "playbackResponse" || r.kind == "reportResponse" || r.kind == "personalResponse") {
				props := object{}
				for _, key := range []string{"ItemId", "MediaSourceId", "UserId", "PlaySessionId", "EventName", "LastPlayedDate"} {
					props[key] = object{"type": "string"}
				}
				for _, key := range []string{"PositionTicks", "PlaybackPositionTicks", "StartTimeTicks", "PlayCount", "MaxStreamingBitrate", "AudioStreamIndex", "SubtitleStreamIndex"} {
					props[key] = object{"type": "integer"}
				}
				for _, key := range []string{"IsPaused", "Played", "IsFavorite", "EnableDirectPlay", "EnableDirectStream", "EnableTranscoding"} {
					props[key] = object{"type": "boolean"}
				}
				props["DeviceProfile"] = object{"type": "object", "additionalProperties": true}
				o.body = []object{{"type": "object", "properties": props, "additionalProperties": true, "description": "兼容空请求体与显式null；重复冲突值拒绝。播放参数不启用relay/HLS/转码。"}}
			}
			responses := object{}
			for status, schemas := range o.responses {
				body := object{"description": "原生Emby协议，PascalCase，不包管理员Envelope。"}
				if status != 204 && status != 304 && method != "head" && len(schemas) > 0 {
					body["content"] = object{"application/json": object{"schema": uniqueSchemas(schemas)}}
				}
				responses[strconv.Itoa(status)] = body
			}
			switch r.kind {
			case "playbackResponse":
				delete(responses, "302")
			case "streamResponse", "subtitleResponse":
				responses = object{"302": object{"description": "原始视频/外挂字幕严格302到官方CDN，按实际播放器UA取链；不读取正文、不提取或转换字幕。", "headers": object{"Location": object{"schema": object{"type": "string", "format": "uri"}}}}}
				if r.kind == "subtitleResponse" && method != "head" {
					content := object{}
					for _, mime := range []string{"application/x-subrip", "text/x-ssa", "text/vtt"} {
						content[mime] = object{"schema": object{"type": "string", "format": "binary"}}
					}
					responses["200"] = object{"description": "唯一内容例外：本程序已登记上传的≤5MiB UTF-8字幕；媒体用户令牌、条目/物理版本/库权限与UA准入校验一致。no-store/nosniff/CSP sandbox；不提取内嵌字幕。", "content": content}
				} else if r.kind == "subtitleResponse" {
					responses["200"] = object{"description": "已鉴权的上传字幕HEAD，无正文"}
				}

			case "imageResponse":
				responses = object{"200": object{"description": "受控图片", "content": object{"image/jpeg": object{"schema": object{"type": "string", "format": "binary"}}, "image/png": object{"schema": object{"type": "string", "format": "binary"}}, "image/webp": object{"schema": object{"type": "string", "format": "binary"}}}}, "304": object{"description": "ETag匹配，无正文"}}
			case "brandingCSSResponse":
				responses = object{"200": object{"description": "本地空样式，不加载外部资源", "content": object{"text/css": object{"schema": object{"type": "string"}}}}}
			case "socketResponse":
				responses = object{}
			case "reportResponse", "passwordResponse", "logoutResponse", "noContentResponse":
				delete(responses, "200")
				responses["204"] = object{"description": "操作已接受，无正文；不是媒体转码或媒体传输完成证明。"}
			}
			for _, status := range []int{400, 401, 403, 404, 405, 409, 413, 429, 500, 503} {
				o.errors[status] = map[string]bool{}
			}
			if r.kind == "socketResponse" {
				o.errors[501] = map[string]bool{}
			}
			for code := range o.errors {
				responses[strconv.Itoa(code)] = object{"description": "固定原生错误，ResponseStatus.ErrorCode/Message；429参照Retry-After，不显示驱动SQL。", "content": object{"application/json": object{"schema": ref("NativeError")}}}
			}
			security := []any{object{"EmbyToken": []string{}}}
			if r.public {
				security = []any{object{}, object{"EmbyToken": []string{}}}
			}
			params := []any{}
			for _, m := range parameterPattern.FindAllStringSubmatch(path, -1) {
				params = append(params, object{"name": m[1], "in": "path", "required": true, "schema": object{"type": "string"}})
			}
			names := []string{}
			for name := range o.params {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				params = append(params, o.params[name])
			}
			if r.kind == "imageResponse" {
				params = append(params, object{"name": "If-None-Match", "in": "header", "schema": object{"type": "string"}})
			}
			if r.kind == "streamResponse" || r.kind == "subtitleResponse" {
				params = append(params, object{"name": "User-Agent", "in": "header", "required": true, "schema": object{"type": "string"}})
			}
			op := object{"operationId": "native_" + operationID(method, path), "tags": []string{"原生Emby协议"}, "summary": r.kind, "description": "仅native/media listener；允许/emby与/mediabrowser前缀及大小写兼容，不提供管理路由。GET同时支持HEAD，已知path的OPTIONS为204静态Allow。/socket为501，不支持WebSocket；嵌入字幕为404。", "security": security, "servers": []any{object{"url": "{nativeOrigin}", "variables": object{"nativeOrigin": object{"default": "http://localhost:8092", "description": "替换为MEMBER_PUBLIC_URL/原生MEMBER_ADDR对应地址，示例为默认内部8092，非管理端口或外部Emby8096。"}}}}, "x-listener": "native-media", "x-go-registration": r.method + " " + r.path, "x-go-handler": "handle:" + r.kind, "x-source": r.location, "responses": responses}
			if r.kind == "imageResponse" {
				op["description"] = op["description"].(string) + " 图片须媒体用户鉴权；读取缓存或返回304前检查条目/演员的库与分级可见性；省略index等同0，图片使用private缓存。"
			}
			if len(params) > 0 {
				op["parameters"] = params
			}
			if method != "get" && method != "head" && len(o.body) > 0 {
				op["requestBody"] = object{"required": r.kind == "authenticateResponse" || r.kind == "passwordResponse", "content": object{"application/json": object{"schema": uniqueSchemas(o.body)}}}
			}
			if paths[path] == nil {
				paths[path] = object{}
			}
			if paths[path].(object)[method] != nil {
				t.Fatal("native/management path conflict", method, path)
			}
			paths[path].(object)[method] = op
		}
	}
	// OPTIONS is generated by the real native router for every known path.
	for path, item := range paths {
		methods := item.(object)
		native := false
		for _, v := range methods {
			if v.(object)["x-listener"] == "native-media" {
				native = true
			}
		}
		if !native {
			continue
		}
		params := []any{}
		for _, m := range parameterPattern.FindAllStringSubmatch(path, -1) {
			params = append(params, object{"name": m[1], "in": "path", "required": true, "schema": object{"type": "string"}})
		}
		methods["options"] = object{"parameters": params, "operationId": "native_" + operationID("options", path), "tags": []string{"原生Emby协议"}, "summary": "known-path Allow", "security": []any{}, "x-listener": "native-media", "servers": []any{object{"url": "http://localhost:8092"}}, "responses": object{"204": object{"description": "静态Allow/Access-Control-Allow-Methods，无Access-Control-Allow-Origin/Credentials，不授予跨域访问。", "headers": object{"Allow": object{"schema": object{"type": "string"}}}}}}
	}
}

// Repeated protocol/error responses are shared components, not thousands of
// duplicated Envelope copies. Names are deterministic content hashes, never OK.
func compactResponses(doc object) {
	components := doc["components"].(object)
	responses := object{}
	components["responses"] = responses
	for _, item := range doc["paths"].(object) {
		for _, value := range item.(object) {
			op := value.(object)
			for status, value := range op["responses"].(object) {
				raw, _ := json.Marshal(value)
				sum := sha256.Sum256(raw)
				name := "HTTP" + status + "_" + hex.EncodeToString(sum[:6])
				responses[name] = value
				op["responses"].(object)[status] = object{"$ref": "#/components/responses/" + name}
			}
		}
	}
}
