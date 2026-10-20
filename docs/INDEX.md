# 现行文档索引

此索引区分当前规范与历史证据。旧独立 cmd、外部市场/远程站点源、HLS/relay/转码验收、多 worker/多 Compose 部署指令不再适用。

## 入口与不变约束

1. [README](../README.md)：根 Compose，唯一 app/postgres/redis，发布与安装。
2. [CLI/API 映射](CLI_REFERENCE.md)：三个二进制、19 工具迁移、Go 测试锁、运维与严格302诊断。
3. [部署指南](NATIVE_DEPLOYMENT.md)、[发布指南](WORKBENCH_RELEASE_GUIDE.md)：操作者私有直接值、Hub immutable AMD64、备份与验证。
4. [架构](ARCHITECTURE.md)、[严格直链](DIRECT_PLAY_ONLY.md)、[官方115](OFFICIAL_115_PLAYBACK.md)：Go/Vue/PG/Redis，稳定 STRM、真实 UA、官方链接与302。
5. [原生站点](NATIVE_SITE_TEMPLATES.md)、[Go站点定义](PT_GO_TEMPLATES.md)：固定 module pin、仅显式选择刷新。
6. [插件开发](PLUGIN_DEVELOPMENT.md)、[框架V2](PLUGIN_FRAMEWORK.md)、[支持矩阵](PLUGIN_SUPPORT_MATRIX.md)：同 pin 的 SDK/宿主/UI、权限审批、worker隔离和数据保留。

## 当前业务专题

- 获取/订阅：[工作区](ACQUIRE_WORKSPACE.md)、[RSS/代理](ACQUIRE_RESULTS_PROXY_RSS.md)、[订阅搜索](SUBSCRIPTION_SEARCH_UI.md)、[站点指标](SITE_METRIC_REFRESH.md)。
- 文件/媒体：[文件管理](FILE_MANAGEMENT.md)、[库工作台](FILES_LIBRARY_WORKBENCH.md)、[命名刮削](CLOUD_NAMING_SCRAPING.md)、[媒体分类/API](MEDIA_LAYOUT_AND_EXTERNAL_API.md)。
- 原生服务：[Emby兼容入口](NATIVE_EMBY_SERVER.md)、[原生媒体](NATIVE_MEDIA_SERVER.md)、[客户端序列](NATIVE_CLIENT_SEQUENCE_MATRIX.md)。
- 安全运维：[账号](ADMIN_ACCOUNTS.md)、[系统操作](SYSTEM_OPERATIONS.md)、[日志备份](RAW_LOGS_AND_BACKUP.md)、[离线保护](OFFLINE_HARDENING.md)。
- 测试边界见 [tests/README](../tests/README.md)；真实用户/设备/生产恢复不作为合成测试夹具。

## 历史证据

`docs/plans/` 全部早期阶段文件已标记 **superseded**，仅供追溯，不能证明当前版本已发布、已部署或通过新的质量门。

Q0 的基线、D1–D5 的所有者审核决策和本阶段改动见 [Q1/Q2 执行记录](plans/20261020-q1-q2-consolidation.md)。Q3 仍采用66起 append-only，不因本轮移除代码而擅自删除 schema65 的表或凭据。Q4 journal容量问题仍为发布前必修。
