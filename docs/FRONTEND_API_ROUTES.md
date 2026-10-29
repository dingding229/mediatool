# 前端接口使用清单

与[流程说明](FRONTEND_API.md)及[OpenAPI](../api/openapi.yaml)配套。管理端/原生媒体端分列listener。路由总量以生成的 OpenAPI 为准；本文记录静态前端使用与客户端协议用途，HEAD/OPTIONS/prefix 展开不等于独立业务功能。

## 统计与方法

| 分类 | operations |
|---|---:|
| 未检出静态使用 | 433 |
| 前端直接调用 | 169 |
| 资源/链接使用 | 4 |
| 前端经适配调用 | 24 |
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
| GET | `/api/external/v1/status` | management | 外部业务能力（非验证表单） | `internal/platform/httpapi/external_api.go` |
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
| GET | `/api/v1/external-api/calls` | management | 前端直接调用 | `web/src/api/external-credentials.ts:11` |
| HEAD | `/api/v1/external-api/calls` | management | 未检出静态使用 |  |
| GET | `/api/v1/external-api/keys` | management | 前端直接调用 | `web/src/api/external-credentials.ts:6` |
| HEAD | `/api/v1/external-api/keys` | management | 未检出静态使用 |  |
| POST | `/api/v1/external-api/keys` | management | 前端直接调用 | `web/src/api/external-credentials.ts:7` |
| POST | `/api/v1/external-api/keys/{id}/revoke` | management | 前端直接调用 | `web/src/api/external-credentials.ts:9` |
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
| POST | `/api/v1/files/identify` | management | 前端经适配调用 | `web/src/api/fileIdentity.ts:4` |
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
| POST | `/api/v1/library/artifact-repairs/{id}/retry` | management | 前端直接调用 | `web/src/organize/HistoryPanel.vue:105` |
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
| GET | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| HEAD | `/api/v1/media/assets/{id}/validation-ref` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library` | management | 前端直接调用 | `web/src/media/useCatalog.ts:28` |
| HEAD | `/api/v1/media/library` | management | 未检出静态使用 |  |
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
| POST | `/api/v1/player-access/observations/{id}/allow` | management | 前端直接调用 | `web/src/api/playerAccess.ts:8` |
| POST | `/api/v1/player-access/observations/{id}/block` | management | 前端直接调用 | `web/src/api/playerAccess.ts:8` |
| POST | `/api/v1/player-access/observations/clear` | management | 前端直接调用 | `web/src/playback/AccessPanel.vue:64` |
| GET | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/api/playerAccess.ts:6` |
| HEAD | `/api/v1/player-access/policy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/player-access/policy` | management | 前端直接调用 | `web/src/api/playerAccess.ts:7` |
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
| GET | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/api/networkProxy.ts:5` |
| HEAD | `/api/v1/settings/network-proxy` | management | 未检出静态使用 |  |
| PUT | `/api/v1/settings/network-proxy` | management | 前端直接调用 | `web/src/api/networkProxy.ts:6` |
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
| GET | `/api/v1/auth/sessions` | management | 前端直接调用 | `web/src/settings/AdministratorSessions.vue:15` |
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

## F6 文件整理识别

`POST /api/v1/files/identify` 仅识别文件名或目录名，不创建候选资产或移动文件；自动识别复用入库的文件名解析、插件识别钩子和 TMDB 唯一精确匹配，分类使用 TMDB 详情的类型、题材、地区和语言。未识别仍返回已启用分类选项，供管理员手动填写。批次与单文件共用 `web/src/files/FileIdentityFields.vue`；预览保留 TMDB ID，服务端重新核对详情。

默认目标 `.` 使用所选分类与自动入库同一命名渲染器；显式目标目录仍为人工覆盖，不擅自追加分类。电影不提交季集，服务端清零并拒绝电影模板的季集变量。剧集未知季集需人工核对，不固定为 1。目录操作保留目录及子文件原名，不递归猜测子文件身份。预览展示最终路径、分类及命名结果；执行仍需单独确认与文件证据核验。

首页不再渲染任何插件仪表盘。插件详情只读取当前运行实例、且在同 pin manifest 中声明的仪表盘；停用实例不启动、不读取。`ui-surfaces` 仍用于全局导航，插件 UI API 仍用于详情，不删除这些共享 API。

## F7 播放统计与公网 IP

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| GET | `/api/v1/media-server/playback-statistics` | management | 前端经适配调用 | `web/src/api/playback-statistics.ts:10` |
| GET | `/api/v1/media-server/playback-events` | management | 前端经适配调用 | `web/src/api/playback-statistics.ts:11` |
| GET | `/api/v1/media-server/playback-ips` | management | 前端经适配调用 | `web/src/api/playback-statistics.ts:12` |
| GET | `/api/v1/ip-geolocation` | management | 前端经适配调用 | `web/src/api/ip-geolocation.ts:12` |

以上均经过管理员认证。`days=7/30/90/0`（0 为全部），`startIndex/limit` 有界分页，按开始时间倒序。IP 汇总 `group=ip/country/province`，用户/设备过滤使用服务器 UUID。归属地在本地查询，库不存在时保留公网 IP，历史私网地址在显示层隐藏。设置中的归属地状态模块和专用状态 API 已移除；离线查询保留，不需要输入密钥。
| GET | `/api/v1/media-server/device-ips` | management | 前端经适配调用 | `web/src/api/playback-statistics.ts:14` |

## F8a 构建版本与只读更新检查

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| GET | `/api/v1/system/version` | management | 前端经适配调用 | `web/src/api/updates.ts:7` |
| GET | `/api/v1/system/updates` | management | 前端经适配调用 | `web/src/api/updates.ts:8` |
| PUT | `/api/v1/system/updates/settings` | management | 前端经适配调用 | `web/src/api/updates.ts:10` |
| POST | `/api/v1/system/updates/check` | management | 前端经适配调用 | `web/src/api/updates.ts:9` |
| POST | `/api/v1/system/updates/notice` | management | 前端经适配调用 | `web/src/api/updates.ts:11` |

以上全部为管理员端接口，媒体用户令牌不可使用；没有部署/拉取镜像或 Docker API。GET 只读取缓存。手动检查最多每分钟一次网络检查，后台间隔可设置为 1–168 小时（默认12），可关闭。notice 对每个可信新版本原子认领一次。公开 API 固定 GitHub/Docker Hub 仓库，响应不可信或 digest 不匹配不提示更新；不接受自定义 URL。

## F8b 在线更新执行与状态

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| GET | `/api/v1/system/online-update` | management | 前端经适配调用 | `web/src/api/updates.ts:14` |
| POST | `/api/v1/system/online-update/control` | management | 前端经适配调用 | `web/src/api/updates.ts:17` |
| POST | `/api/v1/system/online-update` | management | 前端经适配调用 | `web/src/api/updates.ts:15` |

| HEAD | `/api/v1/system/online-update` | management | 未检出静态使用 | GET 自动派生，仅管理员 |

POST 必须当前管理员密码与 confirm=true，version/digest 仅用于匹配重新核验的最新 Release，不能指定镜像/URL/命令。GET 只返回可公开给管理员的状态、阶段与备份路径，不返回 Docker inspect、Compose 内容或私有执行计划。详见 [在线更新设计](ONLINE_UPDATE.md)。


## H3a：按物理版本管理字幕（schema 82）

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| GET | `/api/v1/media/assets/{id}/subtitles` | management | 前端经适配调用 | `web/src/api/subtitles.ts:7` |
| POST | `/api/v1/media/assets/{id}/subtitles` | management | 前端经适配调用 | `web/src/api/subtitles.ts:8` |
| PUT | `/api/v1/media/assets/{id}/subtitles/{subtitleId}` | management | 前端经适配调用 | `web/src/api/subtitles.ts:9` |
| DELETE | `/api/v1/media/assets/{id}/subtitles/{subtitleId}` | management | 前端经适配调用 | `web/src/api/subtitles.ts:10` |
| GET | `/api/v1/media/assets/{id}/subtitles/{subtitleId}/download` | management | 资源/链接使用 | `web/src/api/subtitles.ts:11` |
| HEAD | `/api/v1/media/assets/{id}/subtitles/{subtitleId}/download` | management | 未检出静态使用 | GET 自动派生，仅管理员 |

上传为 multipart file/encoding/uploadId；更新使用 revision；删除需要 confirm=true。
管理员下载使用同源 cookie 包装 URL。原生播放器的 DeliveryUrl 则保持 F2 媒体用户令牌认证、物理版本/字幕身份/库权限/UA 准入；只有本程序上传的字幕可返回文本，自动字幕与视频仍零正文 302，内嵌字幕不提取。见 [字幕管理](MANAGED_SUBTITLES.md)。

| GET | `/api/v1/media/library/managed/{id}` | management | 经适配调用；持久化详情聚合，不触发播放/探测 | `web/src/api/mediaCatalog.ts:5` |
| HEAD | `/api/v1/media/library/managed/{id}` | management | 未检出静态使用 |  |
| GET | `/api/v1/media/library/managed/{id}/metadata` | management | 经适配调用；元数据基础值、覆盖锁、修订号与未匹配编号；unmatchedPageSize 可选10/25/50/100，缺省50 | `web/src/api/mediaMetadata.ts:9` |
| HEAD | `/api/v1/media/library/managed/{id}/metadata` | management | 未检出静态使用 | |
| PATCH | `/api/v1/media/library/managed/{id}/metadata` | management | 经适配调用；原子编辑／逐项恢复；修订冲突409 | `web/src/api/mediaMetadata.ts:9` |

## T4 图片覆盖（schema 84）

| 方法 | 路径 | listener | 用途 | 前端证据 |
|---|---|---|---|---|
| GET | /api/v1/media/library/managed/{id}/images | management | 前端经适配调用 | `web/src/api/mediaImages.ts:9` |
| POST | /api/v1/media/library/managed/{id}/images | management | 前端经适配调用 | `web/src/api/mediaImages.ts:12` |
| PATCH | /api/v1/media/library/managed/{id}/images | management | 前端经适配调用 | `web/src/api/mediaImages.ts:11` |
| GET | /api/v1/media/library/managed/{id}/images/candidates | management | 前端经适配调用 | `web/src/api/mediaImages.ts:10` |
| GET | /api/v1/media/library/managed/{id}/images/{kind} | management | 资源/链接使用 | `web/src/media/ImageEditor.vue:34` |

## T9 Cron 预览

| 方法 | 路径 | 监听器 | 使用情况 | 前端证据 |
|---|---|---|---|---|
| POST | `/api/v1/scheduling/cron/preview` | management | 前端直接调用：Cron实时校验与接下来5次预览（只读） | `web/src/api/scheduling.ts:4` |

## T11 IP 黑名单（管理端）

单 IP 预检自动扩为 IPv4 /24、IPv6 /64；显式 CIDR 规范化后保留前缀。check 可传 excludeId，返回 candidateNetwork、affectsCurrent、coveredBy；完整覆盖新增由写事务再次校验，备注限制仍为 500 字符。
| 方法 | 路径 | listener | 使用 | 证据 |
|---|---|---|---|---|
| GET | `/api/v1/player-access/ip-blocklist` | management | 前端直接调用 | `web/src/api/ip-blocklist.ts:7` |
| HEAD | `/api/v1/player-access/ip-blocklist` | management | 未检出静态使用 |  |
| POST | `/api/v1/player-access/ip-blocklist` | management | 前端直接调用 | `web/src/api/ip-blocklist.ts:9` |
| PUT | `/api/v1/player-access/ip-blocklist/{id}` | management | 前端直接调用 | `web/src/api/ip-blocklist.ts:9` |
| DELETE | `/api/v1/player-access/ip-blocklist/{id}` | management | 前端直接调用 | `web/src/api/ip-blocklist.ts:10` |
| POST | `/api/v1/player-access/ip-blocklist/check` | management | 前端直接调用 | `web/src/api/ip-blocklist.ts:8` |

T12：`GET /api/v1/auth/sessions` 不分页，一次返回全部有效会话，仍排除过期、撤销、超过创建后30天、凭据版本失效及临时密码失效的会话；不读取历史无效会话。站点账号编辑的 `values` 沿用加密存储，允许管理员修改 `cookie` / `userAgent` / `passkey`，RSS 地址仍经已有站点验证；不写入审计明文。

T13：`PUT /system/updates/settings` 同时要求 enabled 与 intervalHours（1–168 的整数）；保存不覆盖检查结果。在线更新常开，不再返回 executionEnabled；`POST /system/online-update/control` 仅接受 confirm_recovery，旧 enable/disable 返回400。更新执行及人工恢复确认仍要求当前管理员密码与 confirm=true；F8b 核心安全约束不变。

### T14 闲时自动更新

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| GET | `/api/v1/system/online-update/idle` | management | 前端直接调用 | `web/src/api/updates.ts:22` |
| PUT | `/api/v1/system/online-update/idle` | management | 前端直接调用 | `web/src/api/updates.ts:23` |
| POST | `/api/v1/system/online-update/idle/check` | management | 前端直接调用 | `web/src/api/updates.ts:24` |

### T15 JWT 凭据

| method | path | listener | 当前前端使用 | 证据（源码行） |
|---|---|---|---|---|
| POST | `/api/v1/external-api/keys/{id}/reveal` | management | 前端直接调用 | `web/src/api/external-credentials.ts:8` |
| DELETE | `/api/v1/external-api/keys/{id}` | management | 前端直接调用 | `web/src/api/external-credentials.ts:10` |

旧专用验证UI/客户端函数删除，status.read等十条正式业务路由不变。reveal动作仅view/copy；先提交披露审计再返回token。

### I1 可选分页条数

- `GET /api/v1/cache/{kind}`：`pageSize` 可选 10/25/30/50/100，缺省 30。
- `GET /api/v1/plugins/workflows`（列表及 `view=runs`）：`pageSize` 可选 10/20/25/50/100，缺省 20。
- `GET /api/v1/media/library/managed/{id}/metadata`：`unmatchedPageSize` 可选 10/25/50/100，缺省 50。
- 上述参数非法、为空或重复时返回 400；GET 对应的 HEAD 采用相同契约。
