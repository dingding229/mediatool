> 0.11插件框架已加入Go/WASM运行、市场、页面/API/定时服务等，当前文档见PLUGIN_FRAMEWORK.md。
> 以下是早期v1规则/HTTPS合约历史说明；其中“仅声明式/不执行代码”和秘密回显描述由0.11及最新用户要求取代。

# 插件扩展合约 v1

本程序不下载安装Python插件、不执行任意Go共享库或shell。内置插件提供0day解析及安全命名；后续Go模块可实现 `plugins.Extension` 接口，外部扩展支持ZIP插件包；旧JSON Manifest入口保留兼容。

```json
{"id":"my-helper","name":"整理助手","version":"1.0.0","contractVersion":1,"endpoint":"https://your-domain.example/hook","capabilities":["media.recognize","media.organized"]}
```

新插件停用导入，启用需管理员明确确认将传输的文件名、识别线索、整理后的相对路径/状态及目标。限制HTTPS公有DNS目标，拒绝凭据URL、私网、重定向；每次请求最长3秒，最多三个扩展，响应最多64KiB。不会传递Cookie、API Key、STRM链接、恢复密码或文件内容。

- `media.recognize`：POST `{contractVersion:1,kind:"media.recognize",data:{filename,hint}}`，返回JSON `{title?,year?,mediaType?}`，结果只作待确认线索，仍需校验路径及真实媒体身份。
- `media.organized`：文件操作成功并核对后，在既有主程序Worker处理，发送 `{contractVersion:1,kind:"media.organized",data:{eventId,operation,state,items:[{source,target}]}}`。数据库记录一次性投递状态，未知结果不自动重发。接收方应使用eventId去重。普通站点直搜不会自动调用外部插件。

参考MoviePilot的搜索/缓存/能力分层设计；固定参考提交见本轮计划。未复制其GPLv3 Python代码或插件市场，插件二进制/API并不兼容MoviePilot。


## ZIP插件包与设置（0.10）

插件ZIP根目录只接受 `manifest.json`、可选 `settings.json`、`rules.json`、`README.md`。
限制压缩2MiB、解包4MiB、16项；拒绝重复、符号链接、路径穿越和未知代码文件，不解压到宿主。
这是本程序的声明式v1合约，**不是MoviePilot可执行插件市场**。完整支持本合约的安装预览、
修订校验安装/升级、设置、明确启停、测试、执行记录（每插件最近100次）和卸载。

- `runtime: "rules"`：Go进程内的有界RE2识别规则，只接受 `media.recognize`。
  使用 `${title}`、`${year}`等正则捕获和 `{{setting:prefix}}`非秘密设置，返回标题/年份/类型线索。
  不访问网络、不修改媒体，也不能将密码字段插入媒体标题。
- `runtime: "webhook"`（旧Manifest省略也兼容）：上述HTTPS事件Hook。
  事件增加 `settings`，仅包含该插件管理员填写的配置；启用界面明确说明这一外传范围。
  系统其他服务凭据不可获得。一次外传失败可能已被接收，执行记录标记unknown，不自动重试。
- 设置字段：`string/password/boolean/number/select`，可设置必填、默认值、选项。
  默认值需匹配类型；秘密设置不能携带默认值。配置经主密钥加密，GET不自动回显秘密。
  密码留空保留；勾选清除才删除。保存设置会停用，重新核对并启用后才会执行。
- 安装/升级均停用。升级仅保留同类型且仍合法的设置。卸载删配置与执行记录、不删媒体。
  手动测试必须确认，只提交输入文本，无实际媒体变更。结果和数据库浏览均脱敏。

可运行示例位于 `examples/plugins/filename-rules/` 和 `examples/plugins/filename-rules.zip`。
重建：`python3 scripts/package-plugin.py examples/plugins/filename-rules output.zip`。
示例输入 `Example.Show.2026.S01E03.mkv`；前缀设置可在测试结果中验证。
