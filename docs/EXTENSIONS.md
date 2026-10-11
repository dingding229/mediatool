# 插件扩展合约 v1

本程序不下载安装Python插件、不执行任意Go共享库或shell。内置插件提供0day解析及安全命名；后续Go模块可实现 `plugins.Extension` 接口，外部扩展可导入JSON Manifest。

```json
{"id":"my-helper","name":"整理助手","version":"1.0.0","contractVersion":1,"endpoint":"https://your-domain.example/hook","capabilities":["media.recognize","media.organized"]}
```

新插件停用导入，启用需管理员明确确认将传输的文件名、识别线索、整理后的相对路径/状态及目标。限制HTTPS公有DNS目标，拒绝凭据URL、私网、重定向；每次请求最长3秒，最多三个扩展，响应最多64KiB。不会传递Cookie、API Key、STRM链接、恢复密码或文件内容。

- `media.recognize`：POST `{contractVersion:1,kind:"media.recognize",data:{filename,hint}}`，返回JSON `{title?,year?,mediaType?}`，结果只作待确认线索，仍需校验路径及真实媒体身份。
- `media.organized`：文件操作成功并核对后，在既有主程序Worker处理，发送 `{contractVersion:1,kind:"media.organized",data:{eventId,operation,state,items:[{source,target}]}}`。数据库记录一次性投递状态，未知结果不自动重发。接收方应使用eventId去重。普通站点直搜不会自动调用外部插件。

参考MoviePilot的搜索/缓存/能力分层设计；固定参考提交见本轮计划。未复制其GPLv3 Python代码或插件市场，插件二进制/API并不兼容MoviePilot。
