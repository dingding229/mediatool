# 自动检查命令

Go 检查与代码同包，前端检查位于 `web/src`，浏览器检查位于 `web/e2e`。

```sh
npm test --prefix web
bash scripts/test-go.sh
bash scripts/test-e2e.sh
```

修改数据库的检查必须串行，且只使用专用 `media_tool_test` 与 Redis DB15。不得使用业务数据库、真实媒体、生产密码恢复或其他项目凭据作为输入。

这些命令不等于实际设备播放或外部账号可用；使用者操作见[文档目录](../docs/INDEX.md)。
