# 可执行沙箱插件开发指南

更新：2026-10-12；只定义当前版本，不提供旧版插件兼容。

## 1. 运行方式

参考MoviePilot-Plugins的V3 `docs/Plugin_Development.md`（固定提交`0f3fdd3c377747727d00528bcb78e472ad8b97ba`）及MoviePilot的代码加载/生命周期实现（`6ef8408f432dde643aee2ba7078f59d2776f84ce`）。参考的是**实际执行插件代码**和宿主提供的扩展点，不是把插件简化为配置规则。

本程序的区别：宿主是Go，插件代码编译为WASI WebAssembly，由app容器内wazero解释器执行；不需要额外插件容器、不启用Python解释器、没有Shell/exec接口，也不兼容MoviePilot Python包。MoviePilot文档描述的是同一Python进程/依赖环境；这里保留同一app进程，但将代码放在能力沙箱。

ZIP根目录必须有`manifest.json`和`plugin.wasm`；允许附加`settings.json`、`README.md`和`ui.html`。只写manifest不能安装。`rules.json`、endpoint、Webhook、规则运行时、空runtime和手填Manifest导入已删除。0day识别与电影/剧集命名模板属于核心功能，不是内置插件。

## 2. 扩展点映射

| MoviePilot开发接口 | 本程序代码调用/声明 | 用途 |
|---|---|---|
| init_plugin(config) | plugin.init，event.settings | 启用后执行代码初始化；配置必须支持重复启用 |
| get_state() | 宿主enabled/revision与插件持久data | 启停状态由宿主管理，插件不能自行越权启用 |
| get_form() | manifest.settings | 声明表单、默认值；执行时将本实例原始配置交给代码 |
| get_page() | plugin.page，manifest.pages | 代码返回详情数据，或隔离ui.html交互页面 |
| get_dashboard() | plugin.dashboard，manifest.dashboard | 代码生成首页卡片内容 |
| get_command() | plugin.command，manifest.commands | 管理员确认后执行代码 |
| get_api() | plugin.api，manifest.routes | 实例命名空间内的会话保护API |
| get_service() | plugin.service，manifest.services | 主程序以五字段Cron调度（Asia/Shanghai）；代码执行实际工作，不支持intervalSeconds或@every |
| 事件注册 | manifest.events，实际事件kind | Outbox投递并执行插件代码，不自动重放未知结果 |
| stop_service() | plugin.stop，宿主修订门禁 | 停用时执行清理代码，旧修订待执行服务取消 |
| save_data/get_data | 输入data/输出data | 实例专属加密JSON；命令/服务可以持久写入 |
| SDK访问业务/HTTP | plugin-sdk/go的Host、HTTP | 声明和授权后的受限能力桥接 |

声明是宿主注册、校验和授权的边界，不替代插件业务代码。不能在初始化中建立宿主线程、进程或直接连接数据库。每次调用创建新沙箱、完成后关闭；跨调用状态通过data持久化。页面、GET API、仪表盘和测试都是只读，不写data、不发通知、不执行POST出站。

## 3. 输入输出与真实代码

宿主通过stdin发送JSON：

```json
{"contractVersion":1,"pluginId":"example-automation","event":{"contractVersion":1,"kind":"plugin.command","data":{"command":"increment"},"settings":{"name":"我的工作台"}},"data":{"count":1}}
```

插件代码由WASI `_start`执行，从stdin解析请求、进行运算/调用授权SDK，再向stdout输出一份JSON：

```json
{"result":{"count":2},"data":{"count":2},"logs":[{"level":"info","message":"计数任务完成"}]}
```

示例`examples/plugins/automation/main.go`是实际Go代码：初始化写入状态，命令/定时服务增加持久计数，API读取状态，页面/仪表盘返回内容，测试尝试读取宿主文件/环境并证明访问不可用。不是预设响应的静态规则。对应ZIP随源码提供，可在隔离测试实例安装、启用并执行。不要为了验收而自动向正式实例安装示例。

## 4. 开发和打包

```sh
GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' -o /tmp/plugin.wasm ./examples/plugins/automation
python3 scripts/build-plugin-example.py
```

第二条命令确定性重建示例ZIP和市场索引摘要；临时WASM不会保存在仓库。自定义包先在自己的构建目录准备可执行文件，再用`scripts/package-plugin.py`打包。后端校验ZIP路径、大小、Manifest字段、WASM入口、内存和导入；安装/升级默认停用，启用需明确授权。分身复制代码和配置，但数据/任务与原实例隔离。

完整Manifest见`examples/plugins/automation/manifest.json`；授权业务桥接见`plugin-sdk/go/sdk_wasm.go`。只使用本实例配置和声明权限，不能读取主程序其他账号的Cookie/密钥、任意PG SQL或Redis数据。

## 5. 沙箱与故障行为

- 64MiB线性内存、4秒执行、256KiB stdout、16KiB stderr、128KiB持久data。
- 无宿主目录挂载、不继承环境变量、无直接网络/进程执行；WASI的文件/套接字接口不会自动获得宿主资源。
- 宿主导入仅接受`mediatool.http_request`和`mediatool_host.request`，禁止外部内存导入；有效WASI `_start`必须存在。
- HTTP仅访问manifest列出的HTTPS公有origin，限制调用次数/超时，拒绝私网、重绑定、重定向和含凭据URL。
- 插件不能绕过主程序媒体身份、路径、来源证明、账号权限或原STRM302策略。
- 内存越界、输出过量、错误JSON或超时执行失败，记录日志/执行状态；外部副作用不能保证撤销，因此未知结果不自动重试。
- 沙箱是能力限制，不是完整虚拟机或对任意不可信代码的安全保证。依赖升级、资源限制和宿主桥接仍需持续审计。

## 6. 验收

必须真正安装、授权、执行代码，而不只是编译/Manifest校验。隔离测试覆盖：初始化→命令→持久状态→API/页面/仪表盘→定时服务→停用→旧任务取消→升级/分身隔离；超时、中断、内存上限和无宿主文件/环境，以及无可执行代码/旧包拒绝。

不能把当前框架称为实现MoviePilot文档的全部扩展能力：Agent、认证提供者、插件独立SQL模型、浏览器自动化和任意模块替换不在本程序当前开放权限中，需要单独定义安全宿主接口后再扩展。
