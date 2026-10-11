# 测试边界

Go 测试与代码同包，前端单元测试在 web/src。当前覆盖配置安全、会话、CSRF、凭据分离、路径/查询编码、原子写入与 symlink 逃逸、302/HEAD/请求头限制、基础 Emby 认证透传。

`internal/modules/jobs/jobs_integration_test.go` 只有设置 TEST_DATABASE_URL 时执行；必须使用迁移后的临时数据库且队列为空。当前不会创建或覆盖用户数据库。

后续目录：PT 脱敏 fixture、CloudDrive2 版本能力 fixture、Emby 客户端兼容矩阵。真实集成测试需用户授权凭据，不将真实 Cookie/Passkey/临时直链提交仓库。

M0 已新增真实 Go 协议代码的本地服务 fixture 串联：gRPC → 固定 STRM → HTTPS 媒体 → Emby 静态流权限映射。fixture 不是 115/Emby 实际安装或客户端，不能作为真实播放验收记录。

真实环境只读检查：python3 scripts/verify-m0.py，缺少配置时退出 2，输出 needs_configuration；真实播放器矩阵须人工验证。
