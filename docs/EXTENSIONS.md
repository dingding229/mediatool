# 插件扩展（当前合约）

2026-10-12：仅支持可执行WASM插件，不保留旧Webhook、声明式规则、空runtime或手填Manifest入口。旧格式说明已移除；0day解析和电影/剧集命名模板属于主程序核心功能。

- 运行方式与MoviePilot开发接口映射：PLUGIN_DEVELOPMENT.md。
- 权限、调度、页面、API、数据和安装：PLUGIN_FRAMEWORK.md。
- 实际Go代码：examples/plugins/automation/main.go。
- Manifest：examples/plugins/automation/manifest.json。
- 可安装包：examples/plugins/automation.zip。
- 确定性重建：`python3 scripts/build-plugin-example.py`。
- 宿主能力SDK：plugin-sdk/go/sdk_wasm.go。

ZIP必须含manifest.json与plugin.wasm，代码在app进程内wazero沙箱执行。安装/升级默认停用；启用需管理员授权。只有声明被授权的业务/HTTP能力才能调用，不允许宿主Shell或任意文件、环境、SQL访问。

HDSKY精简配置是站点模板，不是插件；见examples/sites/README.md。
