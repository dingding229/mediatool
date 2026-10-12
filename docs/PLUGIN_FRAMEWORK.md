# 可执行沙箱插件框架（0.11.1）

## 参考与功能范围

参考而非复制MoviePilot的插件功能；不要求Python包/API兼容。固定参考提交：
- MoviePilot v3：899a0eb85b688f473c4e43e1ae02dafbe54c6841（GPL-3.0）。
- MoviePilot-Frontend v3：7de709c124b382ea931730926f4d894c83e3d52e（MIT）。
- MoviePilot-Plugins：c9d6c5785d76f4d34978f8804459430d4cb44086（GPL-3.0）。
- MoviePilot-Resources：a46e2d9780b466f9b268043957a1504fc04cfea0（GPL-3.0）。

功能映射：市场/仓库→HTTPS index.json；安装升级→ZIP摘要+修订确认；get_form→动态设置字段；
init/stop→plugin.init/plugin.stop；get_page→独立Vue页面及隔离自定义UI；get_dashboard→首页卡片；
get_api/get_command→插件实例命名空间内管理员路由/命令；get_service→持久定时任务及单次触发；
事件→媒体识别/整理与声明Outbox事件；持久数据→实例专属加密JSON；分身→独立ID/配置/数据/服务；
运行记录/日志→有界执行历史及主日志module=plugins。

本程序实现以上框架能力，不声称重做MoviePilot市场的每个具体插件，也不实现其Python/Vuetify
模块联邦ABI、任意数据库SQL或宿主Shell。本程序不自动清理或改变用户媒体。

## 执行与权限

Go/WASI代码在同一app进程的wazero解释器运行；64MiB内存、4秒执行、256KiB输出、默认无文件/环境/网络。
ZIP最多8MiB、展开24MiB，16个根目录文件，必须包含manifest.json和有效plugin.wasm，允许settings.json/README.md/ui.html。仅支持WASM代码；不兼容旧Webhook/rules/空runtime或手填Manifest。0day识别和命名模板是核心功能，不在插件列表。开发和MoviePilot接口映射见PLUGIN_DEVELOPMENT.md。
WASM只允许WASI和mediatool/mediatool_host导入，异常/未知结果不自动重复调用。

manifest.outbound列出HTTPS公有origin；明确授权后才提供HTTP host请求，最多3次，每次3秒，拒绝
私网/DNS重绑定/跳转/凭据URL。权限可声明library.list、downloads.list、tmdb.search、notifications.send；
只由受限宿主桥接实现，不传数据库/其他服务密钥。页面、仪表盘、GET API和测试不写插件数据，
不允许POST出站或发送通知。不能凭插件能力绕过既有媒体操作/做种源/路径限制。

## 生命周期与调度

安装/升级和设置保存均停用。启用调用init，失败回到停用；停用调用stop。
服务支持60秒至7天间隔，或五字段Cron/CRON_TZ（不快于每分钟），可用scheduleSetting绑定插件配置字段；主程序默认队列持久调度，单次可明确触发；同服务不积压重复运行。
队列记录冻结配置revision；停用/配置升级后旧待执行任务取消。执行已claim但未确认的结果
保持outcome_unknown，不能自动重放。相同插件运行以PG锁串行保护私有状态。

配置和状态均使用现有主密钥/AAD加密；分身复制代码和配置（包括秘密值）并默认停用，但不复制
运行数据或共享服务。升级不会自动修改分身；需逐实例确认新版本与设置。

## 页面和设置

字段支持string/password/boolean/number/select/textarea/date，可声明必填、默认值、选项。
后台按最新用户要求显示原始设置，不自动脱敏。默认页面/仪表盘是插件返回的JSON
{title,description,metrics:[{label,value}],rows:[...],actions:[{id,label,data}]}，Vue安全渲染。

需要任意交互UI可附自包含ui.html（<=256KiB、内联JS/CSS）；运行在sandbox allow-scripts iframe，
不允许同源、网络、导航或表单提交，不能读取主程序会话。通过postMessage实例桥接访问自己的
data、声明GET API和命令；命令仍需管理员确认。不使用任意远程依赖或主页面v-html。
框架支持独立页面/侧栏、首页仪表盘、配置与数据操作，但不直接加载MoviePilot Vue联邦包。

## API与示例

管理员接口：/api/v1/plugins/{id}/settings、state、runs、data；
/api/v1/plugins/{id}/view/{page|dashboard|command|api}/{key}；services/{key}；clone；ui。
POST写入/命令/服务需同源CSRF及confirm=true，升级需当前revision+预览digest。
市场：/api/v1/plugin-market/repositories、repositories/{id}/catalog、package。
所有接口仍要求会话；没有公开插件执行API。GET不得执行写操作。

示例examples/plugins/automation.zip为实际编译Go/WASI程序，源码含main.go/manifest/ui/README，
展示初始化、独立页面、API、仪表盘、命令、定时服务及持久计数，不修改媒体、不访问网络。
运行python3 scripts/build-plugin-example.py可确定性重建包与examples/plugins/index.json。
SDK见plugin-sdk/go/sdk_wasm.go：Host(operation,data)、HTTP(url,method,headers,body)。
自定义仓库格式{contractVersion:1,plugins:[{id,name,version,author,description,packageUrl,digest}]}；
packageUrl可相对清单，摘要校验包身份/版本，不自动安装或更新。
