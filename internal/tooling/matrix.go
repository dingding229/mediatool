package tooling

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/dingding229/mediatool-plugin/sdk/contracts"
)

//go:embed matrix_policy.txt
var matrixPolicy string

func matrix(root string, check bool) error {
	b, e := RenderMatrix(root)
	if e != nil {
		return e
	}
	p := filepath.Join(root, "docs/PLUGIN_SUPPORT_MATRIX.md")
	if check {
		old, e := os.ReadFile(p)
		if e != nil || string(old) != string(b) {
			return errors.New("plugin support matrix stale; regenerate from exact pin")
		}
		return nil
	}
	return os.WriteFile(p, b, 0644)
}
func RenderMatrix(root string) ([]byte, error) {
	var inv struct {
		Pin             string
		ContractVersion int
		HostMethods     []map[string]any
		Manifests       []struct {
			ID, Version string
			History     map[string]string
		}
	}
	b, e := os.ReadFile(filepath.Join(root, "internal/modules/plugins/v2/library.json"))
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(b, &inv); e != nil {
		return nil, e
	}
	cmd := exec.Command("go", "list", "-m", "-json", "github.com/dingding229/mediatool-plugin")
	cmd.Dir = root
	b, e = cmd.Output()
	if e != nil {
		return nil, errors.New("pinned plugin module unavailable")
	}
	var module struct{ Dir, Version string }
	if e = json.Unmarshal(b, &module); e != nil {
		return nil, e
	}
	constants := map[string][]string{}
	files, e := filepath.Glob(filepath.Join(module.Dir, "sdk/contracts/*.go"))
	if e != nil {
		return nil, e
	}
	pattern := regexp.MustCompile(`\b(\w+)\s+(EventType|ChainEventType)\s*=\s*"([^"]+)"`)
	for _, p := range files {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, e = os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		for _, m := range pattern.FindAllStringSubmatch(string(b), -1) {
			constants[m[1]] = []string{m[2], m[3]}
		}
	}
	b, e = os.ReadFile(filepath.Join(root, "internal/modules/plugins/v2/host.go"))
	if e != nil {
		return nil, e
	}
	parts := strings.SplitN(string(b), "func hostScope(", 2)
	if len(parts) != 2 {
		return nil, errors.New("host scope contract missing")
	}
	host := strings.SplitN(parts[1], "func (s Store) Host", 2)[0]
	core := []map[string]any{}
	for _, m := range regexp.MustCompile(`case ([^:]+):\s*return "([^"]+)"`).FindAllStringSubmatch(host, -1) {
		for _, n := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
			level := "R"
			if strings.HasSuffix(n[1], ".update") || strings.HasSuffix(n[1], ".put") || strings.HasSuffix(n[1], ".delete") || n[1] == "notifications.send" {
				level = "W"
			}
			core = append(core, map[string]any{"name": n[1], "scope": m[2], "level": level})
		}
	}
	names := []string{}
	for n := range constants {
		names = append(names, n)
	}
	slices.Sort(names)
	events := [][]any{}
	for _, n := range names {
		v := constants[n]
		known := contracts.KnownEventType(contracts.EventType(v[1])) || contracts.KnownChainEventType(contracts.ChainEventType(v[1]))
		events = append(events, []any{n, v[0], v[1], known})
	}
	b, e = json.Marshal(map[string]any{"methods": inv.HostMethods, "core": core, "events": events})
	if e != nil {
		return nil, e
	}
	digest := sha256.Sum256(b)
	var out strings.Builder
	fmt.Fprintf(&out, "# 插件能力表\n\n- contractVersion：%d。\n- 契约摘要：`%x`。\n- 读取/生成：`go run ./cmd/media-tool-dev plugins matrix [--check]`；读取编译期元数据，不执行工厂。\n", inv.ContractVersion, digest)
	out.WriteString("- `registered` 只表示存在窄门面，不保证任意环境可用；actor、adapter、owner、selector、revision 和实时 scope 不满足时 fail-closed。\n- 表中的 supported=false 为固定 unsupported；其他条件性 unsupported 见下文。\n\n")
	fmt.Fprintf(&out, "## 业务 Host RPC（%d 项）\n\n| RPC | scope | 级别 | 注册能力 |\n|---|---|---|---|\n", len(inv.HostMethods))
	for _, m := range inv.HostMethods {
		state := "unsupported"
		if m["supported"] == true {
			state = "registered"
		}
		fmt.Fprintf(&out, "| `%s` | `%s` | %s | %s |\n", m["name"], m["scope"], m["level"], state)
	}
	fmt.Fprintf(&out, "\n## 核心 Host RPC（%d 项）\n\n| RPC | scope | 级别 |\n|---|---|---|\n", len(core))
	for _, m := range core {
		fmt.Fprintf(&out, "| `%s` | `%s` | %s |\n", m["name"], m["scope"], m["level"])
	}
	out.WriteString("\n## 广播与链\n\nSDK 提供51个 typed 标识（32广播/19链），其中 N/A 项不接受注册。\n\n| 类型 | 稳定标识 | 精确 scope / 边界 |\n|---|---|---|\n")
	slices.SortFunc(names, func(a, b string) int {
		va, vb := constants[a], constants[b]
		if n := strings.Compare(va[0], vb[0]); n != 0 {
			return n
		}
		return strings.Compare(va[1], vb[1])
	})
	for _, n := range names {
		v := constants[n]
		kind, prefix, known := "广播", "events.subscribe.", contracts.KnownEventType(contracts.EventType(v[1]))
		if v[0] == "ChainEventType" {
			kind, prefix, known = "链", "events.chain.", contracts.KnownChainEventType(contracts.ChainEventType(v[1]))
		}
		scope := "**N/A：契约不接受注册**"
		if known {
			scope = "`" + prefix + v[1] + "`"
		}
		fmt.Fprintf(&out, "| %s | `%s` | %s |\n", kind, v[1], scope)
	}
	policy, footer, ok := strings.Cut(matrixPolicy, "<!-- versions -->\n")
	if !ok {
		return nil, errors.New("matrix policy boundary missing")
	}
	out.WriteString("\n" + policy)
	out.WriteString("## 内置插件与运行范围\n\n| ID | 版本 | 安全配置/使用范围 |\n|---|---|---|\n")
	boundaries := map[string]string{"hello-go": "人工调用示例；不自动运行业务", "site-checkin": "active=false；宿主持有凭据", "library-refresh-notify": "active=false；7日原事实重投/8日完成账本滚动", "seeding-cleanup": "active=false / execute=false / deleteFiles=false", "watch-organize": "active=false；默认深度3/上限5，叶文件size+mtime静默，总项≤200/轮提交≤10", "wishlist-subscribe": "active=false / create=false；公开来源，唯一匹配只创建停用订阅", "torrent-assistant": "active=false / write=false / recheck=false；活跃监控即使不写也可能通知"}
	for _, m := range inv.Manifests {
		bound, ok := boundaries[m.ID]
		if !ok {
			return nil, errors.New("plugin boundary requires review")
		}
		fmt.Fprintf(&out, "| `%s` | %s | %s |\n", m.ID, m.Version, bound)
	}
	out.WriteString(footer)
	return []byte(out.String()), nil
}
