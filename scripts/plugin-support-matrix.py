#!/usr/bin/env python3
"""Freeze public contract metadata only; no plugin factories, DB or credentials."""
import argparse, hashlib, json, re, subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent

def render():
    inventory=json.loads((ROOT/'internal/modules/plugins/v2/library.json').read_text())
    build=(ROOT/'internal/platform/buildinfo/buildinfo.go').read_text()
    version=re.search(r'const Version = "([^"]+)"',build)[1];schema=re.search(r'const Schema int64 = (\d+)',build)[1]
    module=json.loads(subprocess.check_output(['go','list','-m','-json','github.com/dingding229/mediatool-plugin'],cwd=ROOT))
    contracts=Path(module['Dir'])/'sdk/contracts'
    enums=(contracts/'enums.go').read_text()
    constants={name:(kind,value) for p in contracts.glob('*.go') if not p.name.endswith('_test.go') for name,kind,value in re.findall(r'\b(\w+)\s+(EventType|ChainEventType)\s*=\s*"([^"]+)"',p.read_text())}
    def known(func):
        body=enums.split('func '+func+'(',1)[1].split('\nfunc ',1)[0]
        return set(re.findall(r'\b(?:Event|Chain)\w+',body))
    recognized=known('KnownEventType')|known('KnownChainEventType')
    methods=inventory['hostMethods']
    core=[]
    host=(ROOT/'internal/modules/plugins/v2/host.go').read_text().split('func hostScope(',1)[1].split('func (s Store) Host',1)[0]
    for names,scope in re.findall(r'case ([^:]+):\s*return "([^"]+)"',host):
        for name in re.findall(r'"([^"]+)"',names):
            core.append({'name':name,'scope':scope,'level':'W' if name.endswith(('.update','.put','.delete')) or name=='notifications.send' else 'R'})
    snapshot={'methods':methods,'core':core,'events':sorted((name,kind,value,name in recognized) for name,(kind,value) in constants.items())}
    digest=hashlib.sha256(json.dumps(snapshot,sort_keys=True,separators=(',',':')).encode()).hexdigest()
    lines=['# 插件支持矩阵 · R 源码冻结','',f'- 源码候选：{version}；schema {schema}；contractVersion {inventory["contractVersion"]}。',f'- SDK pin：`{inventory["pin"]}`。',f'- 契约摘要：`{digest}`。','- 生成/校验：`python3 scripts/plugin-support-matrix.py [--check]`；仅读取编译期元数据，不执行工厂。',
    '- `registered` 只表示存在窄门面，不保证任意环境可用；actor、adapter、owner、selector、revision 和实时 scope 不满足时 fail-closed。',
    '- 表中的 supported=false 为固定 unsupported；其他条件性 unsupported 见下文。',
    '',f'## 业务 Host RPC（{len(methods)} 项）','','| RPC | scope | 级别 | 注册能力 |','|---|---|---|---|']
    for m in methods:lines.append(f'| `{m["name"]}` | `{m["scope"]}` | {m["level"]} | {"registered" if m["supported"] else "unsupported"} |')
    lines+=['',f'## 核心 Host RPC（{len(core)} 项）','','| RPC | scope | 级别 |','|---|---|---|']
    for m in core:lines.append(f'| `{m["name"]}` | `{m["scope"]}` | {m["level"]} |')
    lines+=['','## 广播与链','','原 MP 映射50项（含4项N/A）之外，宿主另有 `plugin.config.changed` 配置事实；所以当前SDK合计51个typed标识（32广播/19链），不是扩大MP能力声明。','','| 类型 | 稳定标识 | 精确 scope / 边界 |','|---|---|---|']
    for name,(kind,value) in sorted(constants.items(),key=lambda x:(x[1][0],x[1][1])):
        scope=('events.subscribe.' if kind=='EventType' else 'events.chain.')+value
        lines.append(f'| {"广播" if kind=="EventType" else "链"} | `{value}` | '+(f'`{scope}`' if name in recognized else '**N/A：契约不接受注册**')+' |')
    lines+=['','## 拒绝矩阵与证据边界','',
    '- 每个业务 RPC 均经 `TestEveryBusinessMethodLiveScopeAndRevocation`（host_business_test）逐项走成功代理、缺 scope、撤权、unsupported；真实门面的成功/投影/保护另由 plugins/business 单测与集成测试证明，不能把代理成功算真实业务成功。',
    '- 核心配置/KV/状态：concurrency_test、host_test、v2_test 覆盖 KEY SHARE 并发读取、续租、撤权和提交前写 fence；scope 从不缓存。',
    '- 广播：acquisition/organizing/discovery/platform/workflow 对应 map/integration/真实 worker 测试覆盖 commit/rollback、精确 scope、撤权；版本化事实键，不把 requested/unknown 当成功。',
    '- 链：生产每 handler 200ms、整链 2s、最多 8；SDK schema 和宿主再次校验，非法结果只收窄或回落，RPC 不跨写事务，提交前 fence。',
    '- 严格 302：server_test、server_integration_test、media_integration_test 与 worker 接线用例保护 STRM 原文、合法 actor 和无播放/115 URL 投影；插件不得替换 Resolve。',
    '- 管理 API：admin 会话/非 GET CSRF；媒体端口无插件 API。实例 key/public 网关另有精确权限与限流。失败记录只保留固定结果码/requestId，不记正文、配置、operationKey 或原始错误。',
    '', '## 明确 unsupported / N/A（不因 R 扩范围）','',
    '- catalog.home/discover/nextUp/userData 为固定 unsupported；catalog/nativelibrary 等依赖宿主真实 actor，不能采用插件传入的 userId/visibleLibraries 替代。缺失 actor/媒体适配器时其他相关方法也返回 unsupported。',
    '- downloads.retry、fs.media.readText 固定 unsupported。CloudDrive2 没有可靠 mtime 时不退化为仅 size 静默检测。',
    '- 私有 TMDB 收藏/OAuth、真实 auth provider 登录适配器、LLM 对话/自动 Agent 循环不支持；AgentLLMProvider N/A。假 provider 测试不是生产登录能力。',
    '- 不开放任意正文提取、Cookie/Set-Cookie/认证头、TMDB token、代理凭据、115 授权/直链、原始播放 API 或媒体中转。sites.request 只有有界脱敏布尔标记，不返回正文。',
    '- 没有推荐引擎；discover/recommend 只接插件 typed 候选的后台缓存。收集结束不是追剧完成；unknown/缺 TMDB/缺资产不发完成事件。',
    '- workflows 仅管理员批准的有界 DAG，工具仅人工调用；未知外部副作用不自动重放，不恢复旧代码。',
    '', '## 六插件与停用态安全验收','',
    '| ID | 版本 | 当前版本 history（来自 manifest） | 安全配置/当前范围 |','|---|---|---|---|']
    boundaries={'hello-go':'人工框架检查；不自动运行验收业务','site-checkin':'active=false；宿主持有凭据','library-refresh-notify':'active=false；7日原事实重投/8日完成账本滚动','seeding-cleanup':'active=false / execute=false / deleteFiles=false','watch-organize':'active=false；默认深度3/上限5，叶文件size+mtime静默，总项≤200/轮提交≤10','wishlist-subscribe':'active=false / create=false；公开来源，唯一匹配只创建停用订阅','torrent-assistant':'active=false / write=false / recheck=false；活跃监控即使不写也可能通知'}
    for m in inventory['manifests']:
        history=m.get('history',{}).get(m['version'],'未声明').replace('|','\\|').replace('\n',' ')
        lines.append(f'| `{m["id"]}` | {m["version"]} | {history} | {boundaries[m["id"]]} |')
    lines+=['','安装/确认权限后仍停用；JSON 配置可在停用时保存。动态表单/状态页需要显式启用的 worker，不能为渲染自动启动。离线静态 schema 表单仅作后续设计建议，不在 R 实施。',
    '安全模式浏览器验收只证明安装/权限/配置读回/停用隔离；六插件活跃业务分支由合成站点/downloader/TMDB/豆瓣/115 Transport/小文件的真实同 app worker E2E 证明，不能宣称在生产真实账户上签到/删种/整理成功。',
    '保留的 absent 实例和测试快照由用户决定；R 不自动清理。新插件先在插件仓库测试推送并过 CI，再更新主程序精确 pin 和同源 UI 清单，最后经另行批准的备份部署。','']
    return '\n'.join(lines)

if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--check',action='store_true');args=parser.parse_args()
    path=ROOT/'docs/PLUGIN_SUPPORT_MATRIX.md';content=render()
    if args.check:
        if not path.exists() or path.read_text()!=content:raise SystemExit('Plugin support matrix stale; regenerate from the exact pin.')
    else:path.write_text(content)
