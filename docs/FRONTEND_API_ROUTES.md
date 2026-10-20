# 前端接口使用清单（Q5.5）

2026-10-21；与[流程说明](FRONTEND_API.md)及[OpenAPI](../api/openapi.yaml)配套。管理端/原生媒体端分列listener。完整291 paths / 625 operations；HEAD/OPTIONS/prefix展开计入，不是625个独立业务功能。

## 统计与方法

| 分类 | operations |
|---|---:|
| 未检出静态使用 | 434 |
| 前端直接调用 | 162 |
| 资源/链接使用 | 3 |
| 前端经适配调用 | 23 |
| 仅适配层调用 | 3 |

使用TypeScript AST扫描当前web/src的request/post/fetch调用，按实际第二参数中的method（含常量三元分支）识别，不跨越下一次调用猜method；模板参数按路径形状匹配。识别当前api适配函数被导入并调用的痕迹，同时补充href/资源URL字面量。排除测试和通用transport。

“前端直接调用”包含页面、组件和store；“经适配调用”表示具体adapter导出函数有前端引用调用；“仅适配层”尚未检出其页面调用。“未检出静态使用”不是后端死码或可删证明：后端返回的海报/icon URL、运行期descriptor、SSE封装、外部凭据客户端、原生播放器、链接和别名均可能不出现字面量。静态形状匹配不是运行期覆盖率；代码变化后须重新核对。原生媒体协议不应为追求前端“已用”而迁移到管理端口。

U2 更新：主页、服务连接的证据已按新位置核对；密钥默认隐藏，管理员可主动显示已保存值。

## 完整 path / method 清单

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| DELETE | `/` | management | 未检出静态使用 |  |
| GET | `/` | management | 未检出静态使用 |  |
| HEAD | `/` | management | 未检出静态使用 |  |
| OPTIONS | `/` | management | 未检出静态使用 |  |
| PATCH | `/` | management | 未检出静态使用 |  |
| POST | `/` | management | 未检出静态使用 |  |
| PUT | `/` | management | 未检出静态使用 |  |
| TRACE | `/` | management | 未检出静态使用 |  |
| DELETE | `/{path}` | management | 未检出静态使用 |  |
| GET | `/{path}` | management | 未检出静态使用 |  |
| HEAD | `/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/{path}` | management | 未检出静态使用 |  |
| PATCH | `/{path}` | management | 未检出静态使用 |  |
| POST | `/{path}` | management | 未检出静态使用 |  |
| PUT | `/{path}` | management | 未检出静态使用 |  |
| TRACE | `/{path}` | management | 未检出静态使用 |  |
| DELETE | `/api/` | management | 未检出静态使用 |  |
| GET | `/api/` | management | 未检出静态使用 |  |
| HEAD | `/api/` | management | 未检出静态使用 |  |
| OPTIONS | `/api/` | management | 未检出静态使用 |  |
| PATCH | `/api/` | management | 未检出静态使用 |  |
| POST | `/api/` | management | 未检出静态使用 |  |
| PUT | `/api/` | management | 未检出静态使用 |  |
| TRACE | `/api/` | management | 未检出静态使用 |  |
| DELETE | `/api/{path}` | management | 未检出静态使用 |  |
| GET | `/api/{path}` | management | 未检出静态使用 |  |
| HEAD | `/api/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/api/{path}` | management | 未检出静态使用 |  |
| PATCH | `/api/{path}` | management | 未检出静态使用 |  |
| POST | `/api/{path}` | management | 未检出静态使用 |  |
| PUT | `/api/{path}` | management | 未检出静态使用 |  |
| TRACE | `/api/{path}` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/classification` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/classification` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/downloads` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/downloads` | management | 未检出静态使用 |  |
| POST | `/api/external/v1/downloads` | management | 未检出静态使用 |  |
| POST | `/api/external/v1/layout/preview` | management | 未检出静态使用 |  |
| POST | `/api/external/v1/search` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/search/{id}` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/search/{id}` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/status` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/status` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/strm` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/strm` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/strm/{id}/content` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/strm/{id}/content` | management | 未检出静态使用 |  |
| GET | `/api/external/v1/transfers` | management | 未检出静态使用 |  |
| HEAD | `/api/external/v1/transfers` | management | 未检出静态使用 |  |
| POST | `/api/v1/auth/login` | management | 前端直接调用 | `web/src/stores/auth.ts:9` |
| POST | `/api/v1/auth/logout` | management | 前端直接调用 | `web/src/stores/auth.ts:11` |
| POST | `/api/v1/auth/password` | management | 前端直接调用 | `web/src/stores/auth.ts:12` |
| POST | `/api/v1/auth/register` | management | 前端直接调用 | `web/src/stores/auth.ts:10` |
| GET | `/api/v1/auth/session` | management | 前端直接调用 | `web/src/stores/auth.ts:8` |
| HEAD | `/api/v1/auth/session` | management | 未检出静态使用 |  |
| GET | `/api/v1/auth/setup` | management | 前端直接调用 | `web/src/views/LoginView.vue:8` |
| HEAD | `/api/v1/auth/setup` | management | 未检出静态使用 |  |
| GET | `/api/v1/cache/{kind}` | management | 前端直接调用 | `web/src/views/CacheView.vue:52` |
| HEAD | `/api/v1/cache/{kind}` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/cache/{kind}/{id}` | management | 前端直接调用 | `web/src/views/CacheView.vue:87` |
| GET | `/api/v1/cache/{kind}/{id}` | management | 前端直接调用 | `web/src/views/CacheView.vue:70` |
| HEAD | `/api/v1/cache/{kind}/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/cache/{kind}/clear` | management | 前端直接调用 | `web/src/views/CacheView.vue:104` |
| GET | `/api/v1/classification` | management | 前端直接调用 | `web/src/views/ClassificationView.vue:76`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/classification` | management | 未检出静态使用 |  |
| PUT | `/api/v1/classification` | management | 前端直接调用 | `web/src/views/ClassificationView.vue:177` |
| POST | `/api/v1/classification/preview` | management | 未检出静态使用 |  |
| GET | `/api/v1/cloud/accounts/info` | management | 前端直接调用 | `web/src/views/ConnectionsWorkspaceView.vue:35` |
| HEAD | `/api/v1/cloud/accounts/info` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| GET | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| OPTIONS | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| PATCH | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| PUT | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| TRACE | `/api/v1/cloudStorage` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| GET | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| OPTIONS | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| PATCH | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| PUT | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| TRACE | `/api/v1/cloudStorage/` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| GET | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| PATCH | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| PUT | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| TRACE | `/api/v1/cloudStorage/{path}` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage/diagnose` | management | 前端直接调用 | `web/src/views/StrmToolsView.vue:12` |
| GET | `/api/v1/cloudStorage/strm302` | management | 资源/链接使用 | `web/src/views/StrmToolsView.vue:17` |
| HEAD | `/api/v1/cloudStorage/strm302` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage/test` | management | 未检出静态使用 |  |
| GET | `/api/v1/data/postgres/tables` | management | 前端直接调用 | `web/src/views/DataView.vue:9` |
| HEAD | `/api/v1/data/postgres/tables` | management | 未检出静态使用 |  |
| GET | `/api/v1/data/postgres/tables/{table}` | management | 前端直接调用 | `web/src/views/DataView.vue:9` |
| HEAD | `/api/v1/data/postgres/tables/{table}` | management | 未检出静态使用 |  |
| GET | `/api/v1/data/redis/keys` | management | 前端直接调用 | `web/src/views/DataView.vue:9` |
| HEAD | `/api/v1/data/redis/keys` | management | 未检出静态使用 |  |
| GET | `/api/v1/data/redis/value` | management | 前端直接调用 | `web/src/views/DataView.vue:11` |
| HEAD | `/api/v1/data/redis/value` | management | 未检出静态使用 |  |
| GET | `/api/v1/diagnostics/playback/preflight` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/diagnostics/playback/preflight` | management | 未检出静态使用 |  |
| GET | `/api/v1/downloads` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:154` |
| HEAD | `/api/v1/downloads` | management | 未检出静态使用 |  |
| POST | `/api/v1/downloads` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:305` |
| POST | `/api/v1/downloads/{id}/cleanup` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:66` |
| POST | `/api/v1/downloads/{id}/cleanup/reconcile` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:48` |
| POST | `/api/v1/downloads/{id}/reconfirm-source` | management | 前端直接调用 | `web/src/components/DownloadSourceDialog.vue:11` |
| GET | `/api/v1/downloads/{id}/source-preview` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/downloads/{id}/source-preview` | management | 未检出静态使用 |  |
| GET | `/api/v1/downloads/manual` | management | 前端直接调用 | `web/src/components/ManualDownloads.vue:49`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/downloads/manual` | management | 未检出静态使用 |  |
| POST | `/api/v1/downloads/manual` | management | 前端直接调用 | `web/src/components/ManualDownloads.vue:101` |
| POST | `/api/v1/downloads/manual/{id}/inspect` | management | 前端直接调用 | `web/src/components/ManualDownloads.vue:118` |
| POST | `/api/v1/downloads/manual/{id}/retry` | management | 前端直接调用 | `web/src/components/ManualDownloads.vue:130` |
| POST | `/api/v1/downloads/sync` | management | 前端直接调用 | `web/src/components/ManualDownloads.vue:60`、`web/src/views/DownloadsView.vue:150` |
| POST | `/api/v1/emby/check` | management | 未检出静态使用 |  |
| POST | `/api/v1/emby/test` | management | 前端直接调用 | `web/src/connections/GatewayDiagnostics.vue:20` |
| GET | `/api/v1/external-api/calls` | management | 前端直接调用 | `web/src/views/ExternalAPIView.vue:62` |
| HEAD | `/api/v1/external-api/calls` | management | 未检出静态使用 |  |
| GET | `/api/v1/external-api/keys` | management | 前端直接调用 | `web/src/views/ExternalAPIView.vue:61` |
| HEAD | `/api/v1/external-api/keys` | management | 未检出静态使用 |  |
| POST | `/api/v1/external-api/keys` | management | 前端直接调用 | `web/src/views/ExternalAPIView.vue:77` |
| POST | `/api/v1/external-api/keys/{id}/revoke` | management | 前端直接调用 | `web/src/views/ExternalAPIView.vue:98` |
| GET | `/api/v1/files` | management | 前端直接调用 | `web/src/views/FilesView.vue:21` |
| HEAD | `/api/v1/files` | management | 未检出静态使用 |  |
| GET | `/api/v1/files/batches/{id}` | management | 前端直接调用 | `web/src/components/FileBatchWorkbench.vue:133` |
| HEAD | `/api/v1/files/batches/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/files/batches/{id}/approve` | management | 前端直接调用 | `web/src/components/FileBatchWorkbench.vue:118` |
| POST | `/api/v1/files/batches/{id}/reconcile` | management | 前端直接调用 | `web/src/components/FileBatchWorkbench.vue:146` |
| POST | `/api/v1/files/batches/preview` | management | 前端直接调用 | `web/src/components/FileBatchWorkbench.vue:93` |
| GET | `/api/v1/files/operations` | management | 前端直接调用 | `web/src/views/FilesView.vue:22` |
| HEAD | `/api/v1/files/operations` | management | 未检出静态使用 |  |
| POST | `/api/v1/files/operations/{id}/execute` | management | 前端直接调用 | `web/src/views/FilesView.vue:30` |
| POST | `/api/v1/files/operations/{id}/reconcile` | management | 前端直接调用 | `web/src/views/FilesView.vue:31` |
| POST | `/api/v1/files/preview` | management | 前端直接调用 | `web/src/views/FilesView.vue:29` |
| GET | `/api/v1/files/roots` | management | 前端直接调用 | `web/src/views/FilesView.vue:23` |
| HEAD | `/api/v1/files/roots` | management | 未检出静态使用 |  |
| GET | `/api/v1/integrations` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:178`、`web/src/views/SearchView.vue:12`、`web/src/subscriptions/useSubscriptionWorkspace.ts:31`、`web/src/connections/useConnections.ts:16`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/integrations` | management | 未检出静态使用 |  |
| POST | `/api/v1/integrations` | management | 前端直接调用 | `web/src/connections/useConnections.ts:56` |
| DELETE | `/api/v1/integrations/{id}` | management | 前端直接调用 | `web/src/connections/useConnections.ts:150` |
| PUT | `/api/v1/integrations/{id}` | management | 前端直接调用 | `web/src/connections/useConnections.ts:56` |
| POST | `/api/v1/integrations/{id}/115/auth/cancel` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:33` |
| POST | `/api/v1/integrations/{id}/115/auth/poll` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:14` |
| POST | `/api/v1/integrations/{id}/115/auth/start` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:27` |
| POST | `/api/v1/integrations/{id}/secrets/reveal` | management | 前端直接调用 | `web/src/connections/useSecretReveal.ts:21` |
| POST | `/api/v1/integrations/{id}/test` | management | 前端直接调用 | `web/src/connections/useConnections.ts:108` |
| GET | `/api/v1/jobs` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/jobs` | management | 未检出静态使用 |  |
| POST | `/api/v1/jobs` | management | 前端直接调用 | `web/src/views/JobsView.vue:147` |
| GET | `/api/v1/jobs/{id}` | management | 前端直接调用 | `web/src/views/JobsView.vue:158` |
| HEAD | `/api/v1/jobs/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/jobs/browse` | management | 前端直接调用 | `web/src/views/JobsView.vue:112`、`web/src/views/JobsView.vue:158`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/jobs/browse` | management | 未检出静态使用 |  |
| GET | `/api/v1/jobs/events` | management | 前端直接调用 | `web/src/views/JobsView.vue:158` |
| HEAD | `/api/v1/jobs/events` | management | 未检出静态使用 |  |
| GET | `/api/v1/library/activity` | management | 前端直接调用 | `web/src/views/OrganizeHistoryView.vue:137`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/library/activity` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/activity/remove` | management | 前端直接调用 | `web/src/views/OrganizeHistoryView.vue:100` |
| GET | `/api/v1/library/archive-cleanups/{id}` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:18` |
| HEAD | `/api/v1/library/archive-cleanups/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archive-cleanups/{id}/retry` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:19` |
| GET | `/api/v1/library/archive-policy` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:14` |
| HEAD | `/api/v1/library/archive-policy` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archive-policy` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:15` |
| GET | `/api/v1/library/archives` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:14` |
| HEAD | `/api/v1/library/archives` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archives/cleanup-confirm` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:17` |
| POST | `/api/v1/library/archives/cleanup-preview` | management | 前端直接调用 | `web/src/components/ArchiveManager.vue:16` |
| GET | `/api/v1/library/artifact-repairs` | management | 前端直接调用 | `web/src/views/LibraryView.vue:261` |
| HEAD | `/api/v1/library/artifact-repairs` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/artifact-repairs/{id}/retry` | management | 前端直接调用 | `web/src/views/LibraryView.vue:546` |
| GET | `/api/v1/library/candidates` | management | 前端直接调用 | `web/src/views/LibraryView.vue:216`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/library/candidates` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/candidates/{id}/confirm` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/candidates/{id}/recognize` | management | 前端直接调用 | `web/src/views/LibraryView.vue:316` |
| POST | `/api/v1/library/candidates/batch-confirm` | management | 前端直接调用 | `web/src/views/LibraryView.vue:486` |
| POST | `/api/v1/library/organize-selection` | management | 前端直接调用 | `web/src/views/LibraryView.vue:27` |
| GET | `/api/v1/library/outputs` | management | 前端直接调用 | `web/src/components/ManagedOutputWorkbench.vue:45`、`web/src/components/STRMBatchTools.vue:108` |
| HEAD | `/api/v1/library/outputs` | management | 未检出静态使用 |  |
| GET | `/api/v1/library/outputs/{id}/content` | management | 前端直接调用 | `web/src/components/ManagedOutputWorkbench.vue:69` |
| HEAD | `/api/v1/library/outputs/{id}/content` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/outputs/{id}/emby` | management | 前端直接调用 | `web/src/components/ManagedOutputWorkbench.vue:82` |
| GET | `/api/v1/library/scan-schedules` | management | 前端直接调用 | `web/src/views/LibraryView.vue:252` |
| HEAD | `/api/v1/library/scan-schedules` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scan-schedules` | management | 前端直接调用 | `web/src/views/LibraryView.vue:590` |
| GET | `/api/v1/library/scans` | management | 前端直接调用 | `web/src/views/LibraryView.vue:243` |
| HEAD | `/api/v1/library/scans` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scans` | management | 前端直接调用 | `web/src/views/LibraryView.vue:281` |
| GET | `/api/v1/library/scans/{id}` | management | 前端直接调用 | `web/src/components/ScanWorkbench.vue:11` |
| HEAD | `/api/v1/library/scans/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scans/{id}/{action}` | management | 前端直接调用 | `web/src/components/ScanWorkbench.vue:10` |
| GET | `/api/v1/logs` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/logs` | management | 未检出静态使用 |  |
| POST | `/api/v1/logs/clear` | management | 前端直接调用 | `web/src/views/LogsView.vue:37` |
| POST | `/api/v1/logs/level` | management | 前端直接调用 | `web/src/views/LogsView.vue:39` |
| GET | `/api/v1/logs/query` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/logs/query` | management | 未检出静态使用 |  |
| GET | `/api/v1/logs/summary` | management | 前端直接调用 | `web/src/views/LogsView.vue:21` |
| HEAD | `/api/v1/logs/summary` | management | 未检出静态使用 |  |
| GET | `/api/v1/media` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/libraries` | management | 前端经适配调用 | `web/src/api/mediaServer.ts:8` |
| HEAD | `/api/v1/media-server/libraries` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/native/libraries` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/native/libraries` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/native/library-mappings` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/native/library-mappings` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/native/probe/{assetId}/retry` | management | 仅适配层调用 | `web/src/api/mediaServer.ts:11` |
| GET | `/api/v1/media-server/native/processing` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/native/processing` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/native/processing/gc` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/native/processing/retry` | management | 未检出静态使用 |  |
| PUT | `/api/v1/media-server/native/processing/settings` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/native/processing/tasks` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/native/processing/tasks` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/native/sessions` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/native/sessions` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/native/settings` | management | 前端经适配调用 | `web/src/api/mediaServer.ts:47` |
| HEAD | `/api/v1/media-server/native/settings` | management | 未检出静态使用 |  |
| PUT | `/api/v1/media-server/native/settings` | management | 前端经适配调用 | `web/src/api/mediaServer.ts:48` |
| POST | `/api/v1/media-server/native/tmdb/refresh` | management | 仅适配层调用 | `web/src/api/mediaServer.ts:10` |
| GET | `/api/v1/media-server/providers` | management | 前端直接调用 | `web/src/views/MediaServerView.vue:13` |
| HEAD | `/api/v1/media-server/providers` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users` | management | 前端经适配调用 | `web/src/api/mediaServer.ts:7` |
| HEAD | `/api/v1/media-server/users` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/media-server/users/{id}` | management | 前端直接调用 | `web/src/views/MediaUsersView.vue:27` |
| GET | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 |  |
| PUT | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users/{id}/devices` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/users/{id}/devices` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users/{id}/devices/{deviceId}/revoke` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users/{id}/history` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/users/{id}/history` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users/{id}/password` | management | 前端直接调用 | `web/src/views/MediaUsersView.vue:26` |
| POST | `/api/v1/media-server/users/{id}/sessions/revoke` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/assets` | management | 前端直接调用 | `web/src/views/LibraryView.vue:230` |
| HEAD | `/api/v1/media/assets` | management | 未检出静态使用 |  |
| POST | `/api/v1/media/assets/{id}/grant/revoke` | management | 前端直接调用 | `web/src/views/LibraryView.vue:532` |
| POST | `/api/v1/media/assets/{id}/grant/rotate` | management | 前端直接调用 | `web/src/views/LibraryView.vue:532` |
| POST | `/api/v1/media/assets/{id}/identity` | management | 前端直接调用 | `web/src/views/LibraryView.vue:468`、`web/src/views/LibraryView.vue:508` |
| POST | `/api/v1/media/assets/{id}/identity-preview` | management | 前端直接调用 | `web/src/views/LibraryView.vue:456` |
| GET | `/api/v1/media/assets/{id}/strm` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/assets/{id}/strm` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/emby/items/{id}` | management | 前端直接调用 | `web/src/views/MediaLibraryView.vue:17` |
| HEAD | `/api/v1/media/emby/items/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library` | management | 前端直接调用 | `web/src/views/MediaLibraryView.vue:13` |
| HEAD | `/api/v1/media/library` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/emby/{id}/image` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/library/emby/{id}/image` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/emby/home` | management | 前端直接调用 | `web/src/components/EmbyHome.vue:8` |
| HEAD | `/api/v1/media/library/emby/home` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/sources` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/library/sources` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/organize-history` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/organize-history` | management | 未检出静态使用 |  |
| POST | `/api/v1/media/recognize-name` | management | 前端直接调用 | `web/src/components/FileBatchWorkbench.vue:60`、`web/src/views/FilesView.vue:28` |
| GET | `/api/v1/media/tmdb/image` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/tmdb/image` | management | 未检出静态使用 |  |
| POST | `/api/v1/messaging/command-actions` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/details` | management | 前端直接调用 | `web/src/components/SearchMediaContext.vue:38`、`web/src/subscriptions/useSubscriptionWorkspace.ts:165` |
| HEAD | `/api/v1/metadata/details` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/episodes` | management | 前端直接调用 | `web/src/views/LibraryView.vue:426` |
| HEAD | `/api/v1/metadata/episodes` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/image/{size}/{file}` | management | 资源/链接使用 | `web/src/utils/tmdb-image.ts:3` |
| HEAD | `/api/v1/metadata/image/{size}/{file}` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/search` | management | 前端直接调用 | `web/src/views/LibraryView.vue:365`、`web/src/views/SearchResultsView.vue:292` |
| HEAD | `/api/v1/metadata/search` | management | 未检出静态使用 |  |
| POST | `/api/v1/notification-deliveries/{id}/retry` | management | 前端直接调用 | `web/src/connections/NotificationHistory.vue:41` |
| GET | `/api/v1/notifications` | management | 前端直接调用 | `web/src/connections/NotificationHistory.vue:22` |
| HEAD | `/api/v1/notifications` | management | 未检出静态使用 |  |
| GET | `/api/v1/ops` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:154`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/ops` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/alerts/{code}/ack` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:53` |
| POST | `/api/v1/ops/backups` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:233` |
| POST | `/api/v1/ops/backups/{id}/delete` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:247` |
| GET | `/api/v1/ops/backups/{id}/download` | management | 资源/链接使用 | `web/src/views/MaintenanceView.vue:427` |
| HEAD | `/api/v1/ops/backups/{id}/download` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/backups/{id}/drill` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:240` |
| POST | `/api/v1/ops/backups/{id}/pin` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/drills/{id}/cleanup` | management | 未检出静态使用 |  |
| PUT | `/api/v1/ops/policy` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:191` |
| POST | `/api/v1/ops/requests/{id}/approve` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:260` |
| POST | `/api/v1/ops/requests/{id}/cancel` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/requests/{id}/verify` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:272` |
| POST | `/api/v1/ops/requests/preview` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:252` |
| POST | `/api/v1/ops/resume` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:287` |
| GET | `/api/v1/ops/trends` | management | 前端直接调用 | `web/src/views/MaintenanceView.vue:170` |
| HEAD | `/api/v1/ops/trends` | management | 未检出静态使用 |  |
| POST | `/api/v1/pipeline/{kind}/{id}/retry` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:172` |
| GET | `/api/v1/player-access/events` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:69` |
| HEAD | `/api/v1/player-access/events` | management | 未检出静态使用 |  |
| GET | `/api/v1/player-access/observations` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:124` |
| HEAD | `/api/v1/player-access/observations` | management | 未检出静态使用 |  |
| POST | `/api/v1/player-access/observations/{id}/allow` | management | 未检出静态使用 |  |
| POST | `/api/v1/player-access/observations/clear` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:277` |
| GET | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:123` |
| HEAD | `/api/v1/player-access/policy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:148` |
| GET | `/api/v1/player-access/users` | management | 前端直接调用 | `web/src/views/PlayerAccessView.vue:73` |
| HEAD | `/api/v1/player-access/users` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| PATCH | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| PUT | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| TRACE | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins` | management | 前端直接调用 | `web/src/api/plugins.ts:14`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/plugins` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}` | management | 前端经适配调用 | `web/src/api/plugins.ts:15` |
| HEAD | `/api/v1/plugins/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/api-keys` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/api-keys/{key}/revoke` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}/auth/status` | management | 前端经适配调用 | `web/src/api/plugins.ts:42` |
| HEAD | `/api/v1/plugins/{id}/auth/status` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}/capabilities` | management | 仅适配层调用 | `web/src/api/plugins.ts:46` |
| HEAD | `/api/v1/plugins/{id}/capabilities` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/events/replay` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}/extensions` | management | 前端经适配调用 | `web/src/api/plugins.ts:20` |
| HEAD | `/api/v1/plugins/{id}/extensions` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}/review` | management | 前端经适配调用 | `web/src/api/plugins.ts:13` |
| HEAD | `/api/v1/plugins/{id}/review` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/run` | management | 前端经适配调用 | `web/src/api/plugins.ts:22` |
| GET | `/api/v1/plugins/{id}/runs` | management | 前端经适配调用 | `web/src/api/plugins.ts:21` |
| HEAD | `/api/v1/plugins/{id}/runs` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}/state-snapshots` | management | 前端经适配调用 | `web/src/api/plugins.ts:29` |
| HEAD | `/api/v1/plugins/{id}/state-snapshots` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/state-snapshots` | management | 前端经适配调用 | `web/src/api/plugins.ts:30` |
| GET | `/api/v1/plugins/{id}/tools` | management | 前端经适配调用 | `web/src/api/plugins.ts:32` |
| HEAD | `/api/v1/plugins/{id}/tools` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/tools/call` | management | 前端经适配调用 | `web/src/api/plugins.ts:33` |
| GET | `/api/v1/plugins/{id}/ui/{kind}` | management | 前端经适配调用 | `web/src/api/plugins.ts:25` |
| HEAD | `/api/v1/plugins/{id}/ui/{kind}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/agent/chat` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/agent/run` | management | 前端经适配调用 | `web/src/api/plugins.ts:22` |
| POST | `/api/v1/plugins/auth/begin` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/auth/bindings` | management | 前端经适配调用 | `web/src/api/plugins.ts:43` |
| GET | `/api/v1/plugins/catalog` | management | 前端经适配调用 | `web/src/api/plugins.ts:11`、`web/src/api/plugins.ts:15` |
| HEAD | `/api/v1/plugins/catalog` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/events/retention` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/plugins/events/retention` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/operations` | management | 前端经适配调用 | `web/src/api/plugins.ts:16` |
| GET | `/api/v1/plugins/resources` | management | 前端经适配调用 | `web/src/api/plugins.ts:15` |
| HEAD | `/api/v1/plugins/resources` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/ui-surfaces` | management | 前端经适配调用 | `web/src/api/plugins.ts:15`、`web/src/api/plugins.ts:27` |
| HEAD | `/api/v1/plugins/ui-surfaces` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/workflows` | management | 前端经适配调用 | `web/src/api/plugins.ts:15`、`web/src/api/plugins.ts:37`、`web/src/api/plugins.ts:38`、`web/src/api/plugins.ts:39` |
| HEAD | `/api/v1/plugins/workflows` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/workflows` | management | 前端经适配调用 | `web/src/api/plugins.ts:40` |
| GET | `/api/v1/pt/accounts` | management | 前端直接调用 | `web/src/components/PTAccountDialog.vue:60` |
| HEAD | `/api/v1/pt/accounts` | management | 未检出静态使用 |  |
| POST | `/api/v1/pt/accounts` | management | 未检出静态使用 |  |
| PUT | `/api/v1/pt/accounts/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/pt/templates` | management | 前端直接调用 | `web/src/components/PTAccountDialog.vue:59`、`web/src/views/SiteTemplatesView.vue:46` |
| HEAD | `/api/v1/pt/templates` | management | 未检出静态使用 |  |
| GET | `/api/v1/pt/templates/{id}/icon` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/pt/templates/{id}/icon` | management | 未检出静态使用 |  |
| POST | `/api/v1/pt/templates/load` | management | 前端直接调用 | `web/src/views/SiteTemplatesView.vue:60` |
| GET | `/api/v1/pt/ua` | management | 前端直接调用 | `web/src/components/PTAccountDialog.vue:61`、`web/src/components/PTAccountDialog.vue:118` |
| HEAD | `/api/v1/pt/ua` | management | 未检出静态使用 |  |
| GET | `/api/v1/readiness` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/views/ConnectionsWorkspaceView.vue:22` |
| HEAD | `/api/v1/readiness` | management | 未检出静态使用 |  |
| GET | `/api/v1/search` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:181`、`web/src/views/SearchResultsView.vue:218`、`web/src/views/SearchView.vue:12`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/search` | management | 未检出静态使用 |  |
| POST | `/api/v1/search` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:199`、`web/src/views/SearchView.vue:14` |
| GET | `/api/v1/search/{id}` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:247` |
| HEAD | `/api/v1/search/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/search/{id}/media` | management | 前端直接调用 | `web/src/views/SearchResultsView.vue:77`、`web/src/views/SearchResultsView.vue:208` |
| GET | `/api/v1/search/media` | management | 前端直接调用 | `web/src/components/SearchMediaContext.vue:56`、`web/src/views/SearchResultsView.vue:247`、`web/src/views/SearchView.vue:13`、`web/src/subscriptions/SubscriptionComposer.vue:28` |
| HEAD | `/api/v1/search/media` | management | 未检出静态使用 |  |
| GET | `/api/v1/settings/access` | management | 前端直接调用 | `web/src/components/AccessSettings.vue:29` |
| HEAD | `/api/v1/settings/access` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/access` | management | 前端直接调用 | `web/src/components/AccessSettings.vue:40` |
| GET | `/api/v1/settings/application` | management | 前端直接调用 | `web/src/components/ApplicationSettings.vue:49`、`web/src/components/ApplicationSettings.vue:81`、`web/src/components/FileBatchWorkbench.vue:47` |
| HEAD | `/api/v1/settings/application` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/application` | management | 前端直接调用 | `web/src/components/ApplicationSettings.vue:110` |
| POST | `/api/v1/settings/naming-preview` | management | 前端直接调用 | `web/src/components/ApplicationSettings.vue:30` |
| GET | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/connections/ProxySettings.vue:15` |
| HEAD | `/api/v1/settings/network-proxy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/connections/ProxySettings.vue:37` |
| GET | `/api/v1/sites` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/sites` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/definitions` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/sites/definitions` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/metrics` | management | 前端直接调用 | `web/src/components/SiteMetrics.vue:79` |
| HEAD | `/api/v1/sites/metrics` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/metrics/schedule` | management | 前端直接调用 | `web/src/components/SiteMetricSchedule.vue:23` |
| HEAD | `/api/v1/sites/metrics/schedule` | management | 未检出静态使用 |  |
| PUT | `/api/v1/sites/metrics/schedule` | management | 前端直接调用 | `web/src/components/SiteMetricSchedule.vue:44` |
| POST | `/api/v1/sites/metrics/sync` | management | 前端直接调用 | `web/src/components/SiteMetrics.vue:89` |
| POST | `/api/v1/strm/batch-preview` | management | 前端直接调用 | `web/src/components/STRMBatchTools.vue:68` |
| POST | `/api/v1/strm/preview` | management | 前端直接调用 | `web/src/views/StrmToolsView.vue:13` |
| GET | `/api/v1/strm/repairs/{id}` | management | 前端直接调用 | `web/src/components/STRMBatchTools.vue:150` |
| HEAD | `/api/v1/strm/repairs/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/strm/repairs/{id}/approve` | management | 前端直接调用 | `web/src/components/STRMBatchTools.vue:171` |
| POST | `/api/v1/strm/repairs/{id}/retry` | management | 前端直接调用 | `web/src/components/STRMBatchTools.vue:198` |
| POST | `/api/v1/strm/repairs/preview` | management | 前端直接调用 | `web/src/components/STRMBatchTools.vue:132` |
| GET | `/api/v1/subscription-runs/{id}` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:108` |
| HEAD | `/api/v1/subscription-runs/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/subscriptions` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:31` |
| HEAD | `/api/v1/subscriptions` | management | 未检出静态使用 |  |
| POST | `/api/v1/subscriptions` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:46` |
| DELETE | `/api/v1/subscriptions/{id}` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:86` |
| PUT | `/api/v1/subscriptions/{id}` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:46` |
| POST | `/api/v1/subscriptions/{id}/evaluate` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:140` |
| GET | `/api/v1/subscriptions/{id}/history` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:95` |
| HEAD | `/api/v1/subscriptions/{id}/history` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/modules` | management | 前端直接调用 | `web/src/stores/system.ts:7` |
| HEAD | `/api/v1/system/modules` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/operations` | management | 前端直接调用 | `web/src/views/JobsView.vue:128` |
| HEAD | `/api/v1/system/operations` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/runtime` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/system/runtime` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/status` | management | 前端直接调用 | `web/src/stores/system.ts:7`、`web/src/connections/GatewayDiagnostics.vue:7` |
| HEAD | `/api/v1/system/status` | management | 未检出静态使用 |  |
| GET | `/api/v1/transfers` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:155`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/transfers` | management | 未检出静态使用 |  |
| POST | `/api/v1/transfers/{id}/control` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:186`、`web/src/views/DownloadsView.vue:220`、`web/src/views/DownloadsView.vue:246` |
| GET | `/api/v1/transfers/{id}/output-preview` | management | 前端直接调用 | `web/src/components/OutputRecoveryDialog.vue:8` |
| HEAD | `/api/v1/transfers/{id}/output-preview` | management | 未检出静态使用 |  |
| POST | `/api/v1/transfers/{id}/resume-import` | management | 前端直接调用 | `web/src/components/OutputRecoveryDialog.vue:10` |
| GET | `/api/v1/transfers/{id}/sessions` | management | 前端直接调用 | `web/src/views/DownloadsView.vue:229` |
| HEAD | `/api/v1/transfers/{id}/sessions` | management | 未检出静态使用 |  |
| GET | `/Branding/Configuration` | native-media | 未检出静态使用 |  |
| HEAD | `/Branding/Configuration` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Branding/Configuration` | native-media | 未检出静态使用 |  |
| GET | `/Branding/Css` | native-media | 未检出静态使用 |  |
| HEAD | `/Branding/Css` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Branding/Css` | native-media | 未检出静态使用 |  |
| GET | `/Branding/Css.css` | native-media | 未检出静态使用 |  |
| HEAD | `/Branding/Css.css` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Branding/Css.css` | native-media | 未检出静态使用 |  |
| GET | `/DisplayPreferences` | native-media | 未检出静态使用 |  |
| HEAD | `/DisplayPreferences` | native-media | 未检出静态使用 |  |
| OPTIONS | `/DisplayPreferences` | native-media | 未检出静态使用 |  |
| GET | `/DisplayPreferences/{id}` | native-media | 未检出静态使用 |  |
| HEAD | `/DisplayPreferences/{id}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/DisplayPreferences/{id}` | native-media | 未检出静态使用 |  |
| DELETE | `/emby` | management | 未检出静态使用 |  |
| GET | `/emby` | management | 未检出静态使用 |  |
| HEAD | `/emby` | management | 未检出静态使用 |  |
| OPTIONS | `/emby` | management | 未检出静态使用 |  |
| PATCH | `/emby` | management | 未检出静态使用 |  |
| POST | `/emby` | management | 未检出静态使用 |  |
| PUT | `/emby` | management | 未检出静态使用 |  |
| TRACE | `/emby` | management | 未检出静态使用 |  |
| DELETE | `/emby/` | management | 未检出静态使用 |  |
| GET | `/emby/` | management | 未检出静态使用 |  |
| HEAD | `/emby/` | management | 未检出静态使用 |  |
| OPTIONS | `/emby/` | management | 未检出静态使用 |  |
| PATCH | `/emby/` | management | 未检出静态使用 |  |
| POST | `/emby/` | management | 未检出静态使用 |  |
| PUT | `/emby/` | management | 未检出静态使用 |  |
| TRACE | `/emby/` | management | 未检出静态使用 |  |
| DELETE | `/emby/{path}` | management | 未检出静态使用 |  |
| GET | `/emby/{path}` | management | 未检出静态使用 |  |
| HEAD | `/emby/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/emby/{path}` | management | 未检出静态使用 |  |
| PATCH | `/emby/{path}` | management | 未检出静态使用 |  |
| POST | `/emby/{path}` | management | 未检出静态使用 |  |
| PUT | `/emby/{path}` | management | 未检出静态使用 |  |
| TRACE | `/emby/{path}` | management | 未检出静态使用 |  |
| GET | `/Genres` | native-media | 未检出静态使用 |  |
| HEAD | `/Genres` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Genres` | native-media | 未检出静态使用 |  |
| GET | `/healthz` | management | 未检出静态使用 |  |
| HEAD | `/healthz` | management | 未检出静态使用 |  |
| GET | `/Items` | native-media | 未检出静态使用 |  |
| HEAD | `/Items` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}/Download` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}/Download` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}/Download` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}/Images/{type}` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}/Images/{type}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}/Images/{type}` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}/Images/{type}/{index}` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}/Images/{type}/{index}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}/Images/{type}/{index}` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}/PlaybackInfo` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}/PlaybackInfo` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}/PlaybackInfo` | native-media | 未检出静态使用 |  |
| POST | `/Items/{id}/PlaybackInfo` | native-media | 未检出静态使用 |  |
| GET | `/Items/{id}/Similar` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/{id}/Similar` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/{id}/Similar` | native-media | 未检出静态使用 |  |
| GET | `/Items/Counts` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/Counts` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/Counts` | native-media | 未检出静态使用 |  |
| GET | `/Items/Filters` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/Filters` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/Filters` | native-media | 未检出静态使用 |  |
| GET | `/Items/Latest` | native-media | 未检出静态使用 |  |
| HEAD | `/Items/Latest` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Items/Latest` | native-media | 未检出静态使用 |  |
| GET | `/Library/MediaFolders` | native-media | 未检出静态使用 |  |
| HEAD | `/Library/MediaFolders` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Library/MediaFolders` | native-media | 未检出静态使用 |  |
| GET | `/Localization/{wildcard}` | native-media | 未检出静态使用 |  |
| HEAD | `/Localization/{wildcard}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Localization/{wildcard}` | native-media | 未检出静态使用 |  |
| GET | `/readyz` | management | 未检出静态使用 |  |
| HEAD | `/readyz` | management | 未检出静态使用 |  |
| GET | `/Search/Hints` | native-media | 未检出静态使用 |  |
| HEAD | `/Search/Hints` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Search/Hints` | native-media | 未检出静态使用 |  |
| GET | `/Sessions` | native-media | 未检出静态使用 |  |
| HEAD | `/Sessions` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Capabilities` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Capabilities` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Capabilities/Full` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Capabilities/Full` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Logout` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Logout` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Playing` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Playing` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Playing/Ping` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Playing/Ping` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Playing/Progress` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Playing/Progress` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Sessions/Playing/Stopped` | native-media | 未检出静态使用 |  |
| POST | `/Sessions/Playing/Stopped` | native-media | 未检出静态使用 |  |
| GET | `/Shows/{id}/Episodes` | native-media | 未检出静态使用 |  |
| HEAD | `/Shows/{id}/Episodes` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Shows/{id}/Episodes` | native-media | 未检出静态使用 |  |
| GET | `/Shows/{id}/Seasons` | native-media | 未检出静态使用 |  |
| HEAD | `/Shows/{id}/Seasons` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Shows/{id}/Seasons` | native-media | 未检出静态使用 |  |
| GET | `/Shows/NextUp` | native-media | 未检出静态使用 |  |
| HEAD | `/Shows/NextUp` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Shows/NextUp` | native-media | 未检出静态使用 |  |
| GET | `/socket` | native-media | 未检出静态使用 |  |
| HEAD | `/socket` | native-media | 未检出静态使用 |  |
| OPTIONS | `/socket` | native-media | 未检出静态使用 |  |
| GET | `/System/Endpoint` | native-media | 未检出静态使用 |  |
| HEAD | `/System/Endpoint` | native-media | 未检出静态使用 |  |
| OPTIONS | `/System/Endpoint` | native-media | 未检出静态使用 |  |
| GET | `/System/Info` | native-media | 未检出静态使用 |  |
| HEAD | `/System/Info` | native-media | 未检出静态使用 |  |
| OPTIONS | `/System/Info` | native-media | 未检出静态使用 |  |
| GET | `/System/Info/Public` | native-media | 未检出静态使用 |  |
| HEAD | `/System/Info/Public` | native-media | 未检出静态使用 |  |
| OPTIONS | `/System/Info/Public` | native-media | 未检出静态使用 |  |
| GET | `/System/Ping` | native-media | 未检出静态使用 |  |
| HEAD | `/System/Ping` | native-media | 未检出静态使用 |  |
| OPTIONS | `/System/Ping` | native-media | 未检出静态使用 |  |
| POST | `/System/Ping` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Configuration` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Configuration` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Configuration` | native-media | 未检出静态使用 |  |
| DELETE | `/Users/{userId}/FavoriteItems/{itemId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/FavoriteItems/{itemId}` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/FavoriteItems/{itemId}` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Items` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Items` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Items` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Items/{itemId}` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Items/{itemId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Items/{itemId}` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Items/{itemId}/UserData` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Items/{itemId}/UserData` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Items/{itemId}/UserData` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/Items/{itemId}/UserData` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Items/Latest` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Items/Latest` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Items/Latest` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Items/Resume` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Items/Resume` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Items/Resume` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Password` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/Password` | native-media | 未检出静态使用 |  |
| DELETE | `/Users/{userId}/PlayedItems/{itemId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/PlayedItems/{itemId}` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/PlayedItems/{itemId}` | native-media | 未检出静态使用 |  |
| DELETE | `/Users/{userId}/PlayingItems/{itemId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/PlayingItems/{itemId}` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/PlayingItems/{itemId}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/PlayingItems/{itemId}/Progress` | native-media | 未检出静态使用 |  |
| POST | `/Users/{userId}/PlayingItems/{itemId}/Progress` | native-media | 未检出静态使用 |  |
| GET | `/Users/{userId}/Views` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/{userId}/Views` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/{userId}/Views` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/AuthenticateByName` | native-media | 未检出静态使用 |  |
| POST | `/Users/AuthenticateByName` | native-media | 未检出静态使用 |  |
| GET | `/Users/Public` | native-media | 未检出静态使用 |  |
| HEAD | `/Users/Public` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Users/Public` | native-media | 未检出静态使用 |  |
| GET | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | native-media | 未检出静态使用 |  |
| HEAD | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | native-media | 未检出静态使用 |  |
| GET | `/Videos/{id}/original.{ext}` | native-media | 未检出静态使用 |  |
| HEAD | `/Videos/{id}/original.{ext}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Videos/{id}/original.{ext}` | native-media | 未检出静态使用 |  |
| GET | `/Videos/{id}/stream` | native-media | 未检出静态使用 |  |
| HEAD | `/Videos/{id}/stream` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Videos/{id}/stream` | native-media | 未检出静态使用 |  |
| GET | `/Videos/{id}/stream.{ext}` | native-media | 未检出静态使用 |  |
| HEAD | `/Videos/{id}/stream.{ext}` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Videos/{id}/stream.{ext}` | native-media | 未检出静态使用 |  |
| GET | `/Videos/{id}/subtitles/{index}/stream` | native-media | 未检出静态使用 |  |
| HEAD | `/Videos/{id}/subtitles/{index}/stream` | native-media | 未检出静态使用 |  |
| OPTIONS | `/Videos/{id}/subtitles/{index}/stream` | native-media | 未检出静态使用 |  |
| GET | `/watch` | management | 未检出静态使用 |  |
| HEAD | `/watch` | management | 未检出静态使用 |  |
| GET | `/watch/{rest}` | management | 未检出静态使用 |  |
| HEAD | `/watch/{rest}` | management | 未检出静态使用 |  |
