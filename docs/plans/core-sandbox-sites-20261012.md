> **superseded（历史阶段记录）**：不是当前操作指令。当前入口、部署与测试约束见 [CLI/API 映射](../CLI_REFERENCE.md) 和 [文档索引](../INDEX.md)；保留原审计时事实，不作现版本已完成/已部署的证据。

# 核心功能、纯沙箱插件与HDSKY精简模板（2026-10-12）

## 范围与原则

- 0day文件名识别、电影/剧集命名模板是主程序基本功能，不属于插件，不出现在插件列表/市场。
- 插件必须具有可执行代码；只接受ZIP中的Go/WASI WebAssembly（runtime=wasm），不提供旧Webhook、rules、空runtime或手填Manifest的兼容入口。不兼容MoviePilot Python ABI。
- 继续单一app容器、wazero解释器能力沙箱：64MiB线性内存、4秒执行、有限输出，默认没有宿主文件、环境变量、网络或Shell。声明权限与管理员授权不等于任意宿主访问。
- 用户进一步确认不会再有旧版插件：不实现旧数据退役、迁移或兼容层。仅添加WASM写入约束；若意外存在不符合契约的数据，明确报错，不自动删除/转换。
- HDSKY模板仅含实际消费的登录校验、搜索/订阅字段、取种链接、账号统计/等级/做种统计与真实下一页链接；不包含签到、验证码、站内信、字幕、评论、账号秘密。
- 不变更正式PT账号/定义，不执行真实下载/媒体整理。保护原STRM→115 CDN302及账号、115授权、密钥和外部容器。

## 实施顺序

1. [x] 修改后端契约/执行路径，强制安装有效WASM，删除旧规则/Webhook实现与示例。
2. [x] 数据库WASM写入约束；验证不符合契约的写入被拒绝，不添加旧数据兼容逻辑。
3. [x] 插件UI仅展示可运行扩展，移除手填导入，清晰展示沙箱限制/权限/空状态。
4. [x] 生成独立HDSKY JSON与使用说明，离线验证最小字段、搜索/取种、统计和不完整结果策略。
5. [x] 越权导入/超时/内存/输出及生命周期回归；Go race/vet、Vue、Python、E2E串行执行。
6. [ ] GitHub提交推送→Actions AMD64→不可变Docker Hub镜像→服务器保护性更新与只读业务验证。
7. [ ] 更新状态/计划/发布说明；区分本地自动化通过与真实站点/播放器验收。

## MoviePilot插件运行方式补充

核对v3固定提交6ef8408f432dde643aee2ba7078f59d2776f84ce的sdk/plugin/base、runtime/extensions/plugin/loader与lifecycle。参考其实际代码加载、init_plugin、get_state、get_form、get_service、get_api、get_command、get_page、get_dashboard、stop_service，而非静态配置替代代码。保持本程序独立的WASI代码ABI与权限沙箱。ZIP无可执行代码直接拒绝。

核对用户指定Plugin_Development.md固定提交0f3fdd3c377747727d00528bcb78e472ad8b97ba；完整映射及未开放权限见PLUGIN_DEVELOPMENT.md。未复制MoviePilot源码。

## 本地验收

Vue32项与构建、Go全量race/vet、插件新增拒绝写入/导入测试、18个Python脚本测试、52个隔离E2E均通过。界面已从真实E2E截图核对。发布尚待本轮Actions/镜像/服务器回执确认，不能把本地通过当成正式部署。
