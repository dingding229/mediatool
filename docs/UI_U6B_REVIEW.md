# U6b 设置与认证页核对

## 页面与路由

- 设置由 `web/src/views/SettingsWorkspaceView.vue` 挂载全新分区，不再投影旧视图的 query。
- 标签：`general` 通用与访问、`naming` 命名与分类、`automation` 搜索订阅、`api` 外部 API、`mobile` 移动端、`security` 账号安全。
- 旧 `access` → `general`；`library` / `classification` → `naming`；`account` → `security`。
- `settings?tab=general&section=...` 及 section-only 地址迁移到对应新标签，保留其他 query 和 hash；旧缓存入口转运维，TMDB/代理仍归连接页。
- `/classification` → `/settings?tab=naming`，`/external-api` → `/settings?tab=api`。
- 快速前往直接检索六个新标签。首页分类引导也直达 `naming`。
- 13 个主页面现在一律由 `workspaceViews` 挂载新的工作区。`LegacyTabHost` 与 `tabViews` 已删除，没有标签再挂被替代的旧视图。

## 接口逐一核对（无退出接口）

| 原位置 | 接口/能力 | 新位置 |
| --- | --- | --- |
| AccessSettings | GET / PUT `/settings/access`，四个域名、生效地址与监听信息 | `settings/ConfigPanel.vue` + `settings/revision.ts` |
| ApplicationSettings | GET / PUT `/settings/application`，可信反代、命名、资料生成、清理、两种缓存有效期与订阅调度 | 同上，按通用/命名/搜索三个独立表单 |
| ApplicationSettings | POST `/settings/naming-preview` | `settings/ConfigPanel.vue`，防抖实时预览、忽略过期回执 |
| ScrapingSettings | 电影/剧集/季/集全部资料生成选项；无独立接口 | `settings/ConfigPanel.vue` |
| ClassificationView / ClassificationChoices | GET / PUT `/classification`；分组、规则、顺序、启停、题材、国家、原始语言及未知已选值 | `settings/ClassificationPanel.vue` / `RulePicker.vue`，复用纯业务选项目录 |
| ExternalAPIView | GET `/external-api/keys`、GET `/external-api/calls` | `settings/ExternalAccessPanel.vue` |
| ExternalAPIView | POST `/external-api/keys`、POST `/external-api/keys/{id}/revoke` | 同上；创建回执一次展示、撤销确认、未知结果先核对 |
| 新验证入口（既有后端接口） | GET `/api/external/v1/status` | 同上；仅向本站发送 Bearer，不携带管理 Cookie，不存储输入 |
| SettingsView | POST `/auth/password`，经原 auth store 调用；改密撤销所有会话 | `auth/PasswordForm.vue` / `settings/AccountPanel.vue` |
| 账号会话 | GET `/auth/session`、POST `/auth/logout` | `settings/AccountPanel.vue`，退出仍经 auth store |
| PWASettings | 安装提示、安装、更新、安全入口判断、独立窗口判断、安装说明 | `settings/MobilePanel.vue`；沿用 `pwa/install.ts`，无虚构配置接口 |
| SetupGuideControl | 首页引导本地偏好及重新打开 | `settings/AccountPanel.vue`，读写沿用带 try/catch 的偏好函数 |
| LoginView | GET `/auth/setup`、POST `/auth/login` / `/auth/register`，原验证与注册竞态 | 重写的 `views/LoginView.vue`，沿用 auth store 与密码策略 |
| ForcePasswordView | POST `/auth/password` / `/auth/logout`，强制改密路由 | 重写的 `views/ForcePasswordView.vue` + 共享 PasswordForm |

配置接口仍要求提交完整 value + revision：保存前读取最新版本，只把当前表单实际改动的键合并进去；不覆盖其他分区的新值。冲突读取新值并保留草稿，必须由用户选择对照结果，不自动重写。未知保存、202 回执均不视作已保存。开启上传后清理有单独确认。

一次展示是本次前端策略：后端列表接口仍返回 token（本任务禁止修改后端），前端在接收列表时立即丢弃 token，不进入组件列表状态、DOM 或浏览器存储。只有 201 创建回执允许展示；关闭/离开销毁字段与明文。复制失败保留当前一次性字段供手动选择。

## 已删除

- `views/SettingsView.vue`、`ClassificationView.vue`、`ExternalAPIView.vue`。
- `components/ApplicationSettings.vue`、`AccessSettings.vue`、`ScrapingSettings.vue`、`PWASettings.vue`、`ClassificationChoices.vue`。
- `pages/LegacyTabHost.vue` 及其过渡测试；`dashboard/SetupGuideControl.vue`。

## 剩余组件/视图核对及保留理由

被新页面替代的旧视图/专属组件：**无剩余**。下列保留项不是旧页面的过渡挂载：

1. `components/Official115AuthDialog.vue`：115 授权扫码、轮询和状态流程按前阶段要求保持原样。
2. `plugins/PluginSurface.vue`、`PluginForm.vue`、`PluginAuth.vue`、`PluginAutomation.vue`、`PluginSnapshots.vue`、`PluginTools.vue`、`PluginMetadata.vue`、`PluginDashboards.vue`、`ReadFeedback.vue` 及其运行时工具：安全边界明确要求不动；逐文件哈希守卫仍适用。
3. `components/StatusBadge.vue`：订阅、连接和就绪组件共用的状态显示，不是旧页面容器。
4. `components/NavigationMenu.vue`、`QuickNavigator.vue`、`ThemeToggle.vue`：U1 已重写的导航基础设施，不是过渡视图。
5. `ui/`：U1 公共组件，未修改其公共 API。
6. `views/LoginView.vue`、`ForcePasswordView.vue`：文件名保留，布局已重写，认证业务仍沿用原 store；没有恢复密码来源提示。
7. `views/ResourceResultsView.vue`、`PluginDetailView.vue`、`PluginSurfaceView.vue`：前阶段已重写的独立结果/详情/自定义页，不挂在主页面标签内。
8. `workflows/FileBatchDialog.vue`、`OutputRecoveryDialog.vue` 及获取、整理、文件等模块：前阶段新建的共用流程，不存在两份旧/新恢复组件。
9. `classification/options.ts` / `languages.json`、auth store、密码策略、PWA 安装工具等：复用纯业务与平台能力，无旧视图挂载。

没有后端改动、新依赖、公共 ui API 改动或接口降级分类。管理员接口目前不提供会话列表、逐条会话撤销或会话 IP，本页面不虚构；保留当前会话状态、退出和改密撤销全部会话。

## 验证与截图

- Vitest 覆盖分区合并、预读冲突、竞争 409、未知保存、202、输入保留、旧路由、一次性凭据、权限、撤销确认、复制失败、认证、预览代次与清理确认。
- E2E 继续保留真实隔离认证、域名、命名资料、分类、外部 API 验证与撤销等业务断言，更新新页面选择器；另补 U6b 冲突、一次性凭据、组间错误隔离、认证文案与主题截图。
- 截图仅来自 mock：`/tmp/media-tool-u6-screenshots/settings-{general,naming,security}-{1440-light,375-dark}.png` 及 `login-{1440-light,375-dark}.png`。
- 数据库测试串行沿用官方隔离锁，仅 `media_tool_test`、Redis DB15；Go 缓存限定 `/tmp/media-tool-u6b-go-cache`，结束后清理。

## 最终结果

- `npm run typecheck`：通过。
- `npm test`：76 个测试文件、447 个单测通过。
- `npm run build`：通过。
- `npx playwright test`：204 通过，1 个 Linux 专用用例按既有条件跳过；未重试掩盖失败。
- 8 张指定 mock 截图已生成并检查浅/深色及窄屏；页面宽度断言通过。
- 本任务 Go 缓存已精确清理；保留 U6a 截图及两份原有未跟踪计划文件，不推送、不部署。
