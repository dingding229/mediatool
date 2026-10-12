# 功能型插件示例

此插件只更改自己的计数数据，不访问网络、不修改媒体。

安装ZIP后设置标题，明确启用。可进入独立插件页面、执行“增加计数”命令、查询status API、查看仪表盘。
服务启用后每60秒运行一次；停用/升级/保存设置后不会继续执行旧修订任务。

构建：`GOOS=wasip1 GOARCH=wasm go build -o /tmp/plugin.wasm ./examples/plugins/automation`。
将 manifest.json、README.md 和 plugin.wasm 放到ZIP根目录。

这不是MoviePilot Python包。通过 plugin-sdk/go 提供的Host/HTTP接口申请宿主能力，必须在manifest声明并由管理员授权。
