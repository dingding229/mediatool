# 前端接口使用清单

与[流程说明](FRONTEND_API.md)及[OpenAPI](../api/openapi.yaml)配套。管理端/原生媒体端分列listener。完整291 paths / 621 operations；HEAD/OPTIONS/prefix展开计入，不是625个独立业务功能。

## 统计与方法

| 分类 | operations |
|---|---:|
| 未检出静态使用 | 432 |
| 前端直接调用 | 163 |
| 资源/链接使用 | 4 |
| 前端经适配调用 | 23 |
| 仅适配层调用 | 3 |

使用TypeScript AST扫描当前web/src的request/post/fetch调用，按实际第二参数中的method（含常量三元分支）识别，不跨越下一次调用猜method；模板参数按路径形状匹配。识别当前api适配函数被导入并调用的痕迹，同时补充href/资源URL字面量。排除测试和通用transport。

“前端直接调用”包含页面、组件和store；“经适配调用”表示具体adapter导出函数有前端引用调用；“仅适配层”尚未检出其页面调用。“未检出静态使用”不是后端死码或可删证明：后端返回的海报/icon URL、运行期descriptor、SSE封装、外部凭据客户端、原生播放器、链接和别名均可能不出现字面量。静态形状匹配不是运行期覆盖率；代码变化后须重新核对。原生媒体协议不应为追求前端“已用”而迁移到管理端口。

密钥默认隐藏，管理员可主动显示支持字段的已保存值。

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
| GET | `/api/external/v1/status` | management | 前端直接调用 | `web/src/api/client.ts:29`、`web/src/settings/ExternalAccessPanel.vue:89` |
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
| GET | `/api/v1/auth/session` | management | 前端直接调用 | `web/src/stores/auth.ts:8`、`web/src/settings/AccountPanel.vue:18` |
| HEAD | `/api/v1/auth/session` | management | 未检出静态使用 |  |
| GET | `/api/v1/auth/setup` | management | 前端直接调用 | `web/src/views/LoginView.vue:8` |
| HEAD | `/api/v1/auth/setup` | management | 未检出静态使用 |  |
| GET | `/api/v1/cache/{kind}` | management | 前端直接调用 | `web/src/operations/CachePanel.vue:17` |
| HEAD | `/api/v1/cache/{kind}` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/cache/{kind}/{id}` | management | 前端直接调用 | `web/src/operations/CachePanel.vue:58` |
| GET | `/api/v1/cache/{kind}/{id}` | management | 前端直接调用 | `web/src/operations/CachePanel.vue:32`、`web/src/operations/CachePanel.vue:46` |
| HEAD | `/api/v1/cache/{kind}/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/cache/{kind}/clear` | management | 前端直接调用 | `web/src/operations/CachePanel.vue:58` |
| GET | `/api/v1/classification` | management | 前端直接调用 | `web/src/settings/ClassificationPanel.vue:32`、`web/src/settings/revision.ts:25`、`web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/classification` | management | 未检出静态使用 |  |
| PUT | `/api/v1/classification` | management | 前端直接调用 | `web/src/settings/ClassificationPanel.vue:32`、`web/src/settings/revision.ts:93` |
| POST | `/api/v1/classification/preview` | management | 未检出静态使用 |  |
| GET | `/api/v1/cloud/accounts/info` | management | 前端直接调用 | `web/src/views/ConnectionsWorkspaceView.vue:38` |
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
| POST | `/api/v1/cloudStorage/diagnose` | management | 前端直接调用 | `web/src/files/LinkDiagnostics.vue:40` |
| GET | `/api/v1/cloudStorage/strm302` | management | 资源/链接使用 | `web/src/files/LinkDiagnostics.vue:80` |
| HEAD | `/api/v1/cloudStorage/strm302` | management | 未检出静态使用 |  |
| POST | `/api/v1/cloudStorage/test` | management | 未检出静态使用 |  |
| GET | `/api/v1/diagnostics/playback/preflight` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/diagnostics/playback/preflight` | management | 未检出静态使用 |  |
| GET | `/api/v1/downloads` | management | 前端直接调用 | `web/src/acquisition/useDelivery.ts:65`、`web/src/acquisition/useDownloads.ts:35` |
| HEAD | `/api/v1/downloads` | management | 未检出静态使用 |  |
| POST | `/api/v1/downloads` | management | 前端直接调用 | `web/src/acquisition/useDelivery.ts:86` |
| POST | `/api/v1/downloads/{id}/cleanup` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:112` |
| POST | `/api/v1/downloads/{id}/cleanup/reconcile` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:167` |
| POST | `/api/v1/downloads/{id}/reconfirm-source` | management | 前端直接调用 | `web/src/acquisition/SourceRecoveryDialog.vue:62` |
| GET | `/api/v1/downloads/{id}/source-preview` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/downloads/{id}/source-preview` | management | 未检出静态使用 |  |
| GET | `/api/v1/downloads/manual` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/acquisition/useManual.ts:26` |
| HEAD | `/api/v1/downloads/manual` | management | 未检出静态使用 |  |
| POST | `/api/v1/downloads/manual` | management | 前端直接调用 | `web/src/acquisition/useManual.ts:95` |
| POST | `/api/v1/downloads/manual/{id}/inspect` | management | 前端直接调用 | `web/src/acquisition/useManual.ts:119` |
| POST | `/api/v1/downloads/manual/{id}/retry` | management | 前端直接调用 | `web/src/acquisition/useManual.ts:143` |
| POST | `/api/v1/downloads/manual/{id}/import` | management | 前端直接调用 | `web/src/acquisition/ManualWorkspace.vue:21` |
| POST | `/api/v1/downloads/sync` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:74`、`web/src/acquisition/useManual.ts:49` |
| POST | `/api/v1/emby/check` | management | 未检出静态使用 |  |
| POST | `/api/v1/emby/test` | management | 前端直接调用 | `web/src/connections/GatewayDiagnostics.vue:21` |
| GET | `/api/v1/external-api/calls` | management | 前端直接调用 | `web/src/settings/ExternalAccessPanel.vue:21` |
| HEAD | `/api/v1/external-api/calls` | management | 未检出静态使用 |  |
| GET | `/api/v1/external-api/keys` | management | 前端直接调用（仅元数据和前缀；无令牌/哈希） | `web/src/settings/ExternalAccessPanel.vue:21` |
| HEAD | `/api/v1/external-api/keys` | management | 未检出静态使用 |  |
| POST | `/api/v1/external-api/keys` | management | 前端直接调用（201仅此一次返回明文令牌） | `web/src/settings/ExternalAccessPanel.vue:42` |
| POST | `/api/v1/external-api/keys/{id}/revoke` | management | 前端直接调用 | `web/src/settings/ExternalAccessPanel.vue:59` |
| GET | `/api/v1/files` | management | 前端直接调用 | `web/src/files/useBrowser.ts:18`、`web/src/organize/ScanPanel.vue:56` |
| HEAD | `/api/v1/files` | management | 未检出静态使用 |  |
| GET | `/api/v1/files/batches/{id}` | management | 前端直接调用 | `web/src/workflows/FileBatchDialog.vue:160` |
| HEAD | `/api/v1/files/batches/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/files/batches/{id}/approve` | management | 前端直接调用 | `web/src/workflows/FileBatchDialog.vue:141` |
| POST | `/api/v1/files/batches/{id}/reconcile` | management | 前端直接调用 | `web/src/organize/HistoryPanel.vue:76`、`web/src/workflows/FileBatchDialog.vue:160` |
| POST | `/api/v1/files/batches/preview` | management | 前端直接调用 | `web/src/workflows/FileBatchDialog.vue:113` |
| GET | `/api/v1/files/operations` | management | 前端直接调用 | `web/src/files/useBrowser.ts:70`、`web/src/files/FileActionDialog.vue:118` |
| HEAD | `/api/v1/files/operations` | management | 未检出静态使用 |  |
| POST | `/api/v1/files/operations/{id}/execute` | management | 前端直接调用 | `web/src/files/FileActionDialog.vue:95` |
| POST | `/api/v1/files/operations/{id}/reconcile` | management | 前端直接调用 | `web/src/files/FileActionDialog.vue:145`、`web/src/organize/HistoryPanel.vue:78` |
| POST | `/api/v1/files/preview` | management | 前端直接调用 | `web/src/files/FileActionDialog.vue:73` |
| GET | `/api/v1/files/roots` | management | 前端直接调用 | `web/src/files/useBrowser.ts:41`、`web/src/organize/ScanPanel.vue:38` |
| HEAD | `/api/v1/files/roots` | management | 未检出静态使用 |  |
| GET | `/api/v1/integrations` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:31`、`web/src/connections/useConnections.ts:16`、`web/src/acquisition/useSearch.ts:19`、`web/src/acquisition/useResults.ts:111`、`web/src/media/useCatalog.ts:15`、`web/src/sites/TemplatesPanel.vue:11`、`web/src/sites/AccountsPanel.vue:18` |
| HEAD | `/api/v1/integrations` | management | 未检出静态使用 |  |
| POST | `/api/v1/integrations` | management | 前端直接调用 | `web/src/connections/useConnections.ts:56` |
| DELETE | `/api/v1/integrations/{id}` | management | 前端直接调用 | `web/src/connections/useConnections.ts:150`、`web/src/sites/AccountsPanel.vue:64` |
| PUT | `/api/v1/integrations/{id}` | management | 前端直接调用 | `web/src/connections/useConnections.ts:56`、`web/src/sites/AccountsPanel.vue:64` |
| POST | `/api/v1/integrations/{id}/115/auth/cancel` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:33` |
| POST | `/api/v1/integrations/{id}/115/auth/poll` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:14` |
| POST | `/api/v1/integrations/{id}/115/auth/start` | management | 前端直接调用 | `web/src/components/Official115AuthDialog.vue:27` |
| POST | `/api/v1/integrations/{id}/secrets/reveal` | management | 前端直接调用 | `web/src/connections/useSecretReveal.ts:21` |
| POST | `/api/v1/integrations/{id}/test` | management | 前端直接调用 | `web/src/connections/useConnections.ts:108`、`web/src/sites/AccountsPanel.vue:72`、`web/src/dashboard/useReadinessTest.ts:30` |
| GET | `/api/v1/jobs` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/jobs` | management | 未检出静态使用 |  |
| POST | `/api/v1/jobs` | management | 前端直接调用 | `web/src/operations/QueuePanel.vue:35` |
| GET | `/api/v1/jobs/{id}` | management | 前端直接调用 | `web/src/operations/QueuePanel.vue:25`、`web/src/operations/QueuePanel.vue:45` |
| HEAD | `/api/v1/jobs/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/jobs/browse` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/operations/QueuePanel.vue:18` |
| HEAD | `/api/v1/jobs/browse` | management | 未检出静态使用 |  |
| GET | `/api/v1/jobs/events` | management | 经适配调用 | `web/src/api/client.ts:22`、`web/src/operations/QueuePanel.vue:69` |
| HEAD | `/api/v1/jobs/events` | management | 未检出静态使用 |  |
| GET | `/api/v1/library/activity` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:35`、`web/src/organize/HistoryPanel.vue:26`、`web/src/views/OrganizeWorkspaceView.vue:51` |
| HEAD | `/api/v1/library/activity` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/activity/remove` | management | 前端直接调用 | `web/src/organize/HistoryPanel.vue:57` |
| GET | `/api/v1/library/archive-cleanups/{id}` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:125`、`web/src/organize/ArchivePanel.vue:150` |
| HEAD | `/api/v1/library/archive-cleanups/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archive-cleanups/{id}/retry` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:173` |
| GET | `/api/v1/library/archive-policy` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:21`、`web/src/organize/ArchivePanel.vue:57` |
| HEAD | `/api/v1/library/archive-policy` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archive-policy` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:48` |
| GET | `/api/v1/library/archives` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:21` |
| HEAD | `/api/v1/library/archives` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/archives/cleanup-confirm` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:112` |
| POST | `/api/v1/library/archives/cleanup-preview` | management | 前端直接调用 | `web/src/organize/ArchivePanel.vue:86` |
| GET | `/api/v1/library/artifact-repairs` | management | 前端直接调用 | `web/src/organize/useOrganize.ts:22` |
| HEAD | `/api/v1/library/artifact-repairs` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/artifact-repairs/{id}/retry` | management | 前端直接调用 | `web/src/views/OrganizeWorkspaceView.vue:83` |
| GET | `/api/v1/library/candidates` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/organize/useOrganize.ts:14`、`web/src/organize/IdentityEditor.vue:231` |
| HEAD | `/api/v1/library/candidates` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/candidates/{id}/confirm` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/candidates/{id}/recognize` | management | 前端直接调用 | `web/src/organize/IdentityEditor.vue:52` |
| POST | `/api/v1/library/candidates/batch-confirm` | management | 前端直接调用 | `web/src/organize/IdentityEditor.vue:188` |
| POST | `/api/v1/library/organize-selection` | management | 前端直接调用 | `web/src/workflows/FileBatchDialog.vue:45` |
| GET | `/api/v1/library/outputs` | management | 前端直接调用 | `web/src/files/ManagedSTRMPanel.vue:20`、`web/src/files/STRMRepairPanel.vue:38`、`web/src/views/OrganizeWorkspaceView.vue:51` |
| HEAD | `/api/v1/library/outputs` | management | 未检出静态使用 |  |
| GET | `/api/v1/library/outputs/{id}/content` | management | 前端直接调用 | `web/src/files/ManagedSTRMPanel.vue:69` |
| HEAD | `/api/v1/library/outputs/{id}/content` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/outputs/{id}/emby` | management | 前端直接调用 | `web/src/files/ManagedSTRMPanel.vue:90` |
| GET | `/api/v1/library/scan-schedules` | management | 前端直接调用 | `web/src/organize/useOrganize.ts:20`、`web/src/organize/ScanPanel.vue:184`、`web/src/organize/ScanPanel.vue:202` |
| HEAD | `/api/v1/library/scan-schedules` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scan-schedules` | management | 前端直接调用 | `web/src/organize/ScanPanel.vue:172` |
| GET | `/api/v1/library/scans` | management | 前端直接调用 | `web/src/organize/useOrganize.ts:18` |
| HEAD | `/api/v1/library/scans` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scans` | management | 前端直接调用 | `web/src/organize/ScanPanel.vue:80` |
| GET | `/api/v1/library/scans/{id}` | management | 前端直接调用 | `web/src/organize/ScanPanel.vue:106` |
| HEAD | `/api/v1/library/scans/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/library/scans/{id}/{action}` | management | 前端直接调用 | `web/src/organize/ScanPanel.vue:140` |
| GET | `/api/v1/logs` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/logs` | management | 未检出静态使用 |  |
| POST | `/api/v1/logs/clear` | management | 前端直接调用 | `web/src/operations/LogsPanel.vue:45` |
| POST | `/api/v1/logs/level` | management | 前端直接调用 | `web/src/operations/LogsPanel.vue:51` |
| GET | `/api/v1/logs/query` | management | 前端直接调用 | `web/src/operations/LogsPanel.vue:27` |
| HEAD | `/api/v1/logs/query` | management | 未检出静态使用 |  |
| GET | `/api/v1/logs/summary` | management | 前端直接调用 | `web/src/operations/LogsPanel.vue:28` |
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
| GET | `/api/v1/media-server/providers` | management | 前端直接调用 | `web/src/playback/ServicePanel.vue:11` |
| HEAD | `/api/v1/media-server/providers` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users` | management | 前端经适配调用 | `web/src/api/mediaServer.ts:7`、`web/src/playback/UserEditor.vue:43` |
| HEAD | `/api/v1/media-server/users` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/media-server/users/{id}` | management | 前端直接调用 | `web/src/playback/UsersPanel.vue:33` |
| GET | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 | `web/src/playback/SessionsPanel.vue:15`、`web/src/playback/SessionsPanel.vue:37`、`web/src/playback/SessionsPanel.vue:51` |
| HEAD | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 |  |
| PUT | `/api/v1/media-server/users/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users/{id}/devices` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/users/{id}/devices` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users/{id}/devices/{deviceId}/revoke` | management | 未检出静态使用 |  |
| GET | `/api/v1/media-server/users/{id}/history` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media-server/users/{id}/history` | management | 未检出静态使用 |  |
| POST | `/api/v1/media-server/users/{id}/password` | management | 前端直接调用 | `web/src/playback/UserEditor.vue:85` |
| POST | `/api/v1/media-server/users/{id}/sessions/revoke` | management | 未检出静态使用 | `web/src/playback/SessionsPanel.vue:52` |
| GET | `/api/v1/media/assets` | management | 前端直接调用 | `web/src/organize/useOrganize.ts:16`、`web/src/organize/IdentityEditor.vue:220` |
| HEAD | `/api/v1/media/assets` | management | 未检出静态使用 |  |
| POST | `/api/v1/media/assets/{id}/grant/revoke` | management | 前端直接调用 | `web/src/views/OrganizeWorkspaceView.vue:87` |
| POST | `/api/v1/media/assets/{id}/grant/rotate` | management | 前端直接调用 | `web/src/views/OrganizeWorkspaceView.vue:85` |
| POST | `/api/v1/media/assets/{id}/identity` | management | 前端直接调用 | `web/src/organize/IdentityEditor.vue:188` |
| POST | `/api/v1/media/assets/{id}/identity-preview` | management | 前端直接调用 | `web/src/organize/IdentityEditor.vue:154` |
| GET | `/api/v1/media/assets/{id}/strm` | management | 经适配调用 | `web/src/api/client.ts:35`、`web/src/media/model.ts:52` |
| HEAD | `/api/v1/media/assets/{id}/strm` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/emby/items/{id}` | management | 前端直接调用 | `web/src/media/useCatalog.ts:153` |
| HEAD | `/api/v1/media/emby/items/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library` | management | 前端直接调用 | `web/src/media/useCatalog.ts:28` |
| HEAD | `/api/v1/media/library` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/emby/{id}/image` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/library/emby/{id}/image` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/emby/home` | management | 前端直接调用 | `web/src/media/useCatalog.ts:40` |
| HEAD | `/api/v1/media/library/emby/home` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/sources` | management | 前端直接调用 | `web/src/acquisition/useDiscovery.ts:16` |
| HEAD | `/api/v1/media/library/sources` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/organize-history` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/organize-history` | management | 未检出静态使用 |  |
| POST | `/api/v1/media/recognize-name` | management | 前端直接调用 | `web/src/files/FileActionDialog.vue:45`、`web/src/workflows/FileBatchDialog.vue:90` |
| GET | `/api/v1/media/tmdb/image` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/tmdb/image` | management | 未检出静态使用 |  |
| POST | `/api/v1/messaging/command-actions` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/details` | management | 前端直接调用 | `web/src/subscriptions/useSubscriptionWorkspace.ts:165`、`web/src/media/useCatalog.ts:158`、`web/src/acquisition/MediaContext.vue:28`、`web/src/acquisition/useDiscovery.ts:42` |
| HEAD | `/api/v1/metadata/details` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/episodes` | management | 前端直接调用 | `web/src/organize/IdentityEditor.vue:130` |
| HEAD | `/api/v1/metadata/episodes` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/image/{size}/{file}` | management | 资源/链接使用 | `web/src/utils/tmdb-image.ts:3` |
| HEAD | `/api/v1/metadata/image/{size}/{file}` | management | 未检出静态使用 |  |
| GET | `/api/v1/metadata/search` | management | 前端直接调用 | `web/src/acquisition/useDelivery.ts:49`、`web/src/organize/IdentityEditor.vue:109` |
| HEAD | `/api/v1/metadata/search` | management | 未检出静态使用 |  |
| POST | `/api/v1/notification-deliveries/{id}/retry` | management | 前端直接调用 | `web/src/connections/NotificationHistory.vue:42` |
| GET | `/api/v1/notifications` | management | 前端直接调用 | `web/src/connections/NotificationHistory.vue:23` |
| HEAD | `/api/v1/notifications` | management | 未检出静态使用 |  |
| GET | `/api/v1/ops` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/operations/MaintenancePanel.vue:17`、`web/src/operations/PolicyPanel.vue:12` |
| HEAD | `/api/v1/ops` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/alerts/{code}/ack` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/backups` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/backups/{id}/delete` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| GET | `/api/v1/ops/backups/{id}/download` | management | 资源/链接使用 | `web/src/operations/MaintenancePanel.vue:124` |
| HEAD | `/api/v1/ops/backups/{id}/download` | management | 未检出静态使用 |  |
| POST | `/api/v1/ops/backups/{id}/drill` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/backups/{id}/pin` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/drills/{id}/cleanup` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| PUT | `/api/v1/ops/policy` | management | 前端直接调用 | `web/src/operations/PolicyPanel.vue:40` |
| POST | `/api/v1/ops/requests/{id}/approve` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:71` |
| POST | `/api/v1/ops/requests/{id}/cancel` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/requests/{id}/verify` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| POST | `/api/v1/ops/requests/preview` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:57` |
| POST | `/api/v1/ops/resume` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:41` |
| GET | `/api/v1/ops/trends` | management | 前端直接调用 | `web/src/operations/MaintenancePanel.vue:17` |
| HEAD | `/api/v1/ops/trends` | management | 未检出静态使用 |  |
| POST | `/api/v1/pipeline/{kind}/{id}/retry` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:114` |
| GET | `/api/v1/player-access/events` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:11` |
| HEAD | `/api/v1/player-access/events` | management | 未检出静态使用 |  |
| GET | `/api/v1/player-access/observations` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:11` |
| HEAD | `/api/v1/player-access/observations` | management | 未检出静态使用 |  |
| POST | `/api/v1/player-access/observations/{id}/allow` | management | 未检出静态使用 | `web/src/playback/AccessPanel.vue:46` |
| POST | `/api/v1/player-access/observations/clear` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:64` |
| GET | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:11` |
| HEAD | `/api/v1/player-access/policy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:46`、`web/src/playback/AccessPanel.vue:64` |
| GET | `/api/v1/player-access/users` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:11` |
| HEAD | `/api/v1/player-access/users` | management | 未检出静态使用 |  |
| DELETE | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| OPTIONS | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| PATCH | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| PUT | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| TRACE | `/api/v1/plugin/{instance}/{path}` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins` | management | 前端直接调用 | `web/src/api/plugins.ts:14`、`web/src/dashboard/useDashboard.ts:5`、`web/src/acquisition/useDiscovery.ts:17` |
| HEAD | `/api/v1/plugins` | management | 未检出静态使用 |  |
| GET | `/api/v1/plugins/{id}` | management | 前端经适配调用 | `web/src/api/plugins.ts:15` |
| HEAD | `/api/v1/plugins/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/api-keys` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/{id}/api-keys/{key}/revoke` | management | 未检出静态使用 |  |
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
| GET | `/api/v1/plugins/{id}/ui/{kind}` | management | 前端经适配调用 | `web/src/api/plugins.ts:25` |
| HEAD | `/api/v1/plugins/{id}/ui/{kind}` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/agent/chat` | management | 未检出静态使用 |  |
| POST | `/api/v1/plugins/agent/run` | management | 前端经适配调用 | `web/src/api/plugins.ts:22` |
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
| GET | `/api/v1/pt/accounts` | management | 前端直接调用 | `web/src/sites/AccountEditor.vue:41` |
| HEAD | `/api/v1/pt/accounts` | management | 未检出静态使用 |  |
| POST | `/api/v1/pt/accounts` | management | 前端直接调用 | `web/src/sites/AccountEditor.vue:97` |
| PUT | `/api/v1/pt/accounts/{id}` | management | 前端直接调用 | `web/src/sites/AccountEditor.vue:97` |
| GET | `/api/v1/pt/templates` | management | 前端直接调用 | `web/src/sites/AccountEditor.vue:41`、`web/src/sites/TemplatesPanel.vue:11` |
| HEAD | `/api/v1/pt/templates` | management | 未检出静态使用 |  |
| GET | `/api/v1/pt/templates/{id}/icon` | management | 前端资源/链接使用 | `web/src/sites/TemplatesPanel.vue:80`、`web/src/sites/AccountsPanel.vue:135` |
| HEAD | `/api/v1/pt/templates/{id}/icon` | management | 未检出静态使用 |  |
| POST | `/api/v1/pt/templates/load` | management | 前端直接调用 | `web/src/sites/TemplatesPanel.vue:43` |
| GET | `/api/v1/pt/ua` | management | 前端直接调用 | `web/src/sites/AccountEditor.vue:41`、`web/src/sites/AccountEditor.vue:84` |
| HEAD | `/api/v1/pt/ua` | management | 未检出静态使用 |  |
| GET | `/api/v1/readiness` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/views/ConnectionsWorkspaceView.vue:25` |
| HEAD | `/api/v1/readiness` | management | 未检出静态使用 |  |
| GET | `/api/v1/search` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/acquisition/useSearch.ts:28` |
| HEAD | `/api/v1/search` | management | 未检出静态使用 |  |
| POST | `/api/v1/search` | management | 前端直接调用 | `web/src/acquisition/useSearch.ts:102`、`web/src/acquisition/useResults.ts:93` |
| GET | `/api/v1/search/{id}` | management | 前端直接调用 | `web/src/acquisition/useSearch.ts:74`、`web/src/acquisition/useResults.ts:27` |
| HEAD | `/api/v1/search/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/search/{id}/media` | management | 前端直接调用 | `web/src/acquisition/useResults.ts:69` |
| GET | `/api/v1/search/media` | management | 前端直接调用 | `web/src/acquisition/useSearch.ts:54`、`web/src/acquisition/MediaContext.vue:55`、`web/src/subscriptions/SubscriptionComposer.vue:28` |
| HEAD | `/api/v1/search/media` | management | 未检出静态使用 |  |
| GET | `/api/v1/settings/access` | management | 前端直接调用 | `web/src/settings/ConfigPanel.vue:11`、`web/src/settings/revision.ts:25` |
| HEAD | `/api/v1/settings/access` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/access` | management | 前端直接调用 | `web/src/settings/ConfigPanel.vue:11`、`web/src/settings/revision.ts:93` |
| GET | `/api/v1/settings/application` | management | 前端直接调用 | `web/src/settings/revision.ts:25`、`web/src/workflows/FileBatchDialog.vue:51`、`web/src/workflows/FileBatchDialog.vue:85` |
| HEAD | `/api/v1/settings/application` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/application` | management | 前端直接调用 | `web/src/settings/ConfigPanel.vue:11`、`web/src/settings/revision.ts:93` |
| POST | `/api/v1/settings/naming-preview` | management | 前端直接调用 | `web/src/settings/ConfigPanel.vue:40` |
| GET | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/connections/ProxySettings.vue:22`、`web/src/connections/ProxySettings.vue:60` |
| HEAD | `/api/v1/settings/network-proxy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/connections/ProxySettings.vue:51` |
| GET | `/api/v1/sites` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/sites` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/definitions` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/sites/definitions` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/metrics` | management | 前端直接调用 | `web/src/sites/AccountsPanel.vue:18` |
| HEAD | `/api/v1/sites/metrics` | management | 未检出静态使用 |  |
| GET | `/api/v1/sites/metrics/schedule` | management | 前端直接调用 | `web/src/sites/SchedulePanel.vue:10` |
| HEAD | `/api/v1/sites/metrics/schedule` | management | 未检出静态使用 |  |
| PUT | `/api/v1/sites/metrics/schedule` | management | 前端直接调用 | `web/src/sites/SchedulePanel.vue:30` |
| POST | `/api/v1/sites/metrics/sync` | management | 前端直接调用 | `web/src/sites/AccountsPanel.vue:77` |
| POST | `/api/v1/strm/batch-preview` | management | 前端直接调用 | `web/src/files/LinkDiagnostics.vue:65` |
| POST | `/api/v1/strm/preview` | management | 前端直接调用 | `web/src/files/LinkDiagnostics.vue:40` |
| GET | `/api/v1/strm/repairs/{id}` | management | 前端直接调用 | `web/src/files/STRMRepairPanel.vue:87` |
| HEAD | `/api/v1/strm/repairs/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/strm/repairs/{id}/approve` | management | 前端直接调用 | `web/src/files/STRMRepairPanel.vue:124` |
| POST | `/api/v1/strm/repairs/{id}/retry` | management | 前端直接调用 | `web/src/files/STRMRepairPanel.vue:124` |
| POST | `/api/v1/strm/repairs/preview` | management | 前端直接调用 | `web/src/files/STRMRepairPanel.vue:65` |
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
| GET | `/api/v1/system/operations` | management | 前端直接调用 | `web/src/operations/RuntimeChecks.vue:18` |
| HEAD | `/api/v1/system/operations` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/runtime` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5` |
| HEAD | `/api/v1/system/runtime` | management | 未检出静态使用 |  |
| GET | `/api/v1/system/status` | management | 前端直接调用 | `web/src/stores/system.ts:7`、`web/src/connections/GatewayDiagnostics.vue:8` |
| HEAD | `/api/v1/system/status` | management | 未检出静态使用 |  |
| GET | `/api/v1/transfers` | management | 前端直接调用 | `web/src/dashboard/useDashboard.ts:5`、`web/src/acquisition/useDownloads.ts:35` |
| HEAD | `/api/v1/transfers` | management | 未检出静态使用 |  |
| POST | `/api/v1/transfers/{id}/control` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:116`、`web/src/acquisition/useDownloads.ts:147` |
| GET | `/api/v1/transfers/{id}/output-preview` | management | 前端直接调用 | `web/src/workflows/OutputRecoveryDialog.vue:28` |
| HEAD | `/api/v1/transfers/{id}/output-preview` | management | 未检出静态使用 |  |
| POST | `/api/v1/transfers/{id}/resume-import` | management | 前端直接调用 | `web/src/workflows/OutputRecoveryDialog.vue:58` |
| GET | `/api/v1/transfers/{id}/sessions` | management | 前端直接调用 | `web/src/acquisition/useDownloads.ts:187` |
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

## B2 新增管理端接口

| 方法 | 路径 | 监听器 | 使用情况 | 前端证据 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/auth/sessions` | management | 前端直接调用 | `web/src/settings/AdministratorSessions.vue:12` |
| HEAD | `/api/v1/auth/sessions` | management | 未检出静态使用 |  |
| POST | `/api/v1/auth/sessions/{id}/revoke` | management | 前端直接调用 | `web/src/settings/AdministratorSessions.vue:15` |
| GET | `/api/v1/tasks/{kind}/{id}` | management | 前端直接调用 | `web/src/workflows/TaskControl.vue:12` |
| HEAD | `/api/v1/tasks/{kind}/{id}` | management | 未检出静态使用 |  |
| POST | `/api/v1/tasks/{kind}/{id}/{action}` | management | 前端直接调用 | `web/src/workflows/TaskControl.vue:14` |

### B3 已有路由语义补全（未删除旧接口）

| 方法 | 路径 | listener | 使用 | 证据 |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/media/library` | management | 前端直接调用；作品→季→集，每层独立分页 | `web/src/media/useCatalog.ts:30` |
| GET | `/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}` | native-media | 原生客户端资源；实际UA、用户鉴权、仅原格式302 | `internal/modules/embyapi/playback_api.go` |
| GET | `/Videos/{id}/subtitles/{index}/stream` | native-media | 原生客户端资源；MediaSourceId可选当前可播放版本 | `internal/modules/embyapi/playback_api.go` |

### 网页恢复（B4，旧宿主维护接口保留）

| Method | Path | Surface | Usage | Evidence |
| --- | --- | --- | --- | --- |
| GET | `/api/v1/ops/restores` | management | 前端直接调用 | `web/src/operations/WebRestorePanel.vue:14` |
| POST | `/api/v1/ops/restores/preview` | management | 前端直接调用 | `web/src/operations/WebRestorePanel.vue:15` |
| POST | `/api/v1/ops/restores/{id}/execute` | management | 前端直接调用 | `web/src/operations/WebRestorePanel.vue:16` |
| POST | `/api/v1/ops/restores/{id}/reconcile` | management | 前端直接调用 | `web/src/operations/WebRestorePanel.vue:17` |

B4b（schema77）：上述恢复路径不变；preview新增warnings与restore.tableChanges。全部业务表随备份回退；401后以备份账号登录并GET执行记录，不能自动重放execute。

F5b：日志 `/logs/query` 默认 200 条，page/pageSize 与 before 时间快照分页；`/settings/application` 的 logging 保存 1–90 天保留策略和最多 24 小时 Debug 截止时间。文件日志保留 JSONL 实现，不写入日志 SQL 表；每日清理按 1000 条小批执行。

F5b 分页：管理端 `/downloads`、`/downloads/manual`、`/transfers`、`/subscriptions`（view/q/status）、`/subscriptions/{id}/history`、`/notifications`、`/media-server/users`、`/library/scans` 带 page/pageSize 时返回 `{items,total,page,pageSize}`，SQL 限定当前页；不带参数的后台/插件调用保留数组契约。所有 count/排序/表名均为代码常量。每页 1–100 条；日志独立上限 200。页码按列表写入 URL，翻页不清除筛选。

F5b 图标：新增 `GET /pt/templates/{id}/favicon`，管理员会话保护，固定 pin 内站点 favicon 地址；复用 DNS 固定的网络代理客户端，Redis 缓存 24 小时（缺图 5 分钟），返回图片二进制与内容 ETag，不接受请求 URL、Host 推导地址或凭据。旧静态 `/icon` 保留其模板标识用途，不再用于站点账号行。
