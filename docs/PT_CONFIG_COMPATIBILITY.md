# PT 配置兼容与快照恢复

更新：2026-10-11。请求体/有界分页、混合来源证明、声明/加密账号头与origin边界，以及本轮执行契约保护已完成回归及保护发布（pt-execution-contract/schema22）；**仍不是实际 PT 账号、搜索或取种验收**。

## 固定参考与审计范围

- 上游：mantou568/pre-dessert-sites。
- 本轮固定提交：`37bf5c5064ae437f54e26b57406ca84d480daab5`（提交时间2026-09-21）。
- 本地参考共有194个JSON文件：1个公共配置和193个站点配置。
- 静态审计：193个均加载；158个`parser_supported`，35个`partial`，0个无效。
- `accountVerified=false`、`staticConfigurationOnly=true`。这些数字只说明加载和已识别的规则，不能证明真实站点页面、账号、搜索或取种成功。
- 上游配置与图标不进入Docker镜像。参考文件位于被排除的`data/reference`，应用只在用户确认授权后导入数据库；这不是对上游许可义务的豁免。

## 本轮已实现

1. JSON请求体递归占位符替换，保留数字/布尔/数组类型；URL参数与JSON/表单请求体分开处理。
2. 占位符单次替换：搜索文本中的`{passkey}`是用户数据，不会再次展开成账号凭据。
3. `array`字段逐项过滤，`count`仍为整组计数；标签过滤不再把数组字符串化，未知/空标签不显示。
4. `append_field_right`按依赖取值；支持前向依赖，拒绝循环、缺失依赖与过深依赖。
5. `flat_map_fields`展开分组搜索：继承父组标题/详情/标签，以子种子的ID、大小、做种人数、促销和H&R覆盖；全局最多500条。
6. 显式HTML编码；多日期格式依次尝试、无时区日期以UTC确定解析；尺寸与数据库整数边界检查。`trim_chars`不再将时间戳/布尔/数字改成字符串。
7. 无效、非有限或无法识别的促销值保持未知，不推断为免费；无法识别的H&R天数不推断为无H&R。完整搜索仍可清除旧促销，不能借`Partial`保留过期免费状态。
8. `reuse_schema`作为缺省框架标识；条件分支覆盖会清除互斥的选择器/备选分支。
9. 条件分支按源JSON声明顺序执行，默认`*`仅作回退，0/false不会被当成空值。
10. 保存前将字段条件对象规范化为内部有序分支数组，PostgreSQL `jsonb`重排对象键时不会改变条件优先级。普通JSON请求数据、`case`查表过滤器的参数不变。
11. 旧`jsonb`快照如含多分支条件对象而没有顺序证据，显示`partial`并拒绝猜测优先级；账号实测必须显式重新导入，不能自动取新版本补证据。
12. 整批下载、验证、ID核对后在一个事务中写入。单项失效不留下半批更新；提交结果未知时要求读回检查，不自动重试。

## 有意保留的限制

- 上游`timezone`使用算术形式，甚至出现`+19`，不能直接当IANA/常见UTC时区，也不继续把无操作实现标为支持。现在明确报告限制，若实际求值则拒绝。
- 搜索只求值业务必需字段及其依赖；未使用的显示时间字段不阻断资源发现，但仍出现在兼容限制中，不伪造日期。
- Go正则不支持所有PCRE语义；无法编译的规则明确标为部分支持。
- 浏览器挑战、专用框架、真实登录字段、API版本仍需已授权账号联调。
- `parser_supported`不是“193个站点全部实测兼容”。原计划中的宽泛语义、真实站点和业务链路门槛保留。

## 操作与恢复

Vue“PT站点配置”页新增可选完整提交SHA及“按此快照重新导入”。必须保持用户主动确认上游授权；不代用户勾选或创建PT账号。

`POST /api/v1/sites/definitions/import`保持既有输入，并可增加`sourceCommit`：

```json
{"licenseAccepted":true,"ids":["站点ID"],"sourceCommit":"原记录的完整40位提交SHA"}
```

- 未指定提交：解析一次上游main，再固定本次提交。
- 指定提交：只读指定不可变地址，不查询main，不悄悄升级。
- 最多30个不重复的有效站点ID；文件ID须和请求ID一致。
- 旧快照没有可复现提交时，不能把测试标识或分支名当作恢复SHA。

## 离线审计工具

`cmd/site-compat-audit`仅读取显式指定的本地参考文件，不连接数据库、PT或账号。Docker提供`/app/site-compat-audit`，参考配置需要使用者自行只读挂载，不包含在镜像内。

```sh
go run ./cmd/site-compat-audit \
  -directory data/reference/pre-dessert-sites/site_config \
  -source-commit 37bf5c5064ae437f54e26b57406ca84d480daab5
```

输出包含固定来源、计数和站点ID/状态/问题数。当前本地候选还提供固定枚举`issueCodes`及按站点去重的`issueDefinitionCounts`，不包含原始解析器名、字段名、域名、选择器、请求体、URL或凭据；正式镜像是否包含新增字段以其发布记录为准。

## 验证与证据

- 自建HTTP/HTML/JSON样本：请求体、标签、跨字段、分组种子、促销/H&R未知值、编码、格式、输入替换与脱敏。
- 真实隔离PostgreSQL `media_tool_test`：`jsonb`往返保持分支顺序、旧快照拒绝猜测、同快照恢复、批次不留下部分更新。
- Vue固定引用、输入校验、授权门槛与桌面/移动视图使用**fixture**，不假称真实账号或上游导入。
- 本地审计报告：`data/diagnostics/site-compatibility-audit.json`。
- 回归、构建与浏览器证据：`data/diagnostics/sites-*.log`、`web/test-results/screenshots/sites-snapshot-*.png`；最终结果在WORK_LOG补充。

## 首轮业务验收仍需

将最终镜像安全发布到指定项目后，使用用户自己的合法资源和PT凭据走搜索→取种→qBittorrent→保留做种→CD2上传→STRM/NFO→Emby→通知。真实订阅、网页续播与客户端矩阵仍为单独门槛，不因本轮静态审计和fixture通过而勾选。


## 发布收尾（2026-10-10）

- 全量隔离Go `-p 1 -race -count=1`、Go vet、Vue7项单测/类型检查/构建、13项Chrome E2E、15项Python/Node脚本测试通过。首次许可证checkbox测试超时、一次登录网络失败和移动端过渡帧截图问题保留在历史日志；最终13项重新完整运行通过，不靠跳过测试或强制点击掩盖。
- 双架构镜像构建完成。本地原生arm64及服务器原生amd64的最终Docker镜像，在UID65532/只读根文件系统/只读参考文件/无网络条件下，均得到193加载、159核心规则支持、34部分支持、0无效。没有把“构建”当作“运行”。
- 本地aarch64 Docker里的amd64模拟运行仍出现Go map/runtime异常；关闭CPU特性诊断也未解决。单协程最小map样本两架构都成功，真实x86_64裸工具和最终Docker镜像均成功。以上只能界定已观察范围，**未认定具体模拟器/编译器根因，更未宣称该环境问题已修复**。未更改Go版本或生产CPU参数。
- 最终镜像`media-tool:sites-compat-20261010`已发布到`/root/docker/mediatool`，只重建server和三个Worker；未重放迁移，PG仍17；Emby/CD2、账号密码、主密钥、已有媒体与115授权均未改变。
- 发布后公开前端JS含来源提交/同快照恢复入口；两个媒体资产、一份authorized官方会话和23个其他容器保持。未代确认上游授权、自动导入配置或配置PT账号（本项目站点配置仍0）。
- 默认Sen UA的original/Static重新得到302→cdnfhnfile.115cdn.net、CDN206/1KiB，错误Token401无Location；明确管理员登录会话、realPlayerDecoded=false，不代表非管理员或实际Sen全操作通过。
- 回退镜像：`media-tool:before-sites-compat-20261010`；数据库备份：`/root/docker/mediatool/private/database-before-sites-compat-20261010.sql.gz`；旧源码：`/root/docker/mediatool/private/source-before-sites-compat-20261010`。

部署与核验报告：`data/remote-deploy/sites-compat-{release-manifest,deployment,before,after,official115-check}.json`、`sites-image-physical-check.json`及`data/diagnostics/sites-dual-architecture-runtime.json`。目标仍为全范围首轮可用，不因本轮PT解析与发布而删掉真实PT业务/网页续播/客户端门槛。


## 2026-10-11 RSS订阅修复（已发布，真实账号待验收）

上一部署的RSS解析器只从查询参数提取数字种子ID，实际会忽略UNIT3D式数字GUID与路径下载地址；同时可把HTTP 200的登录HTML/XML当成成功空源。已用output-recovery源码包中的原RSS实现，在隔离HTTP样本中复现两项行为，见data/diagnostics/rss-previous-behavior-20261011.{json,log}。未使用真实PT凭据或请求用户站点。

本轮根据RSS 2.0规范、Atom RFC 4287，以及UNIT3D固定提交8b88f4c8182eb3d3912ffef425c3224dcfd596f4的resources/views/rss/show.blade.php与routes/rss.php实现：

- 识别rss/单channel和Atom 1.0命名空间feed；空源可成功，登录/挑战页、缺channel、多根、截断、DTD、错误作用域、超过500项或64层嵌套明确失败。完整token流先校验，entry子树不能跳过边界。
- 保留NexusPHP查询参数ID、支持已核对的/torrent/download/数字ID.rsskey、/torrents/download/数字ID、/torrents/数字ID和/detail/数字ID。GUID按不透明标识处理：不能取域名/密钥里“第一个数字”代替种子ID，已识别的Nexus ID不被其GUID覆盖；UNIT3D键路径的数字GUID按已知上游契约交叉核对。
- 原RSS entry整体检查，未知种子身份、同项ID冲突/重复参数、越界目标/片段、同种子不同下载目标报固定脱敏错误，不返回半份结果建立基线。识别后的重复项合并；accountID+种子ID摘要保持既有分区，RSS Key变化不制造新的历史项。
- RSS/Atom enclosure或已核对的下载型link成为加密download_hint，而不是公开detailsUrl；公开详情仅使用已知详情路径。相对链接按重定向后的真实feed地址及feed/entry/link的xml:base解析，仍限制配置域；Atom self/license/related和外命名空间扩展不代替下载/标题/身份，XHTML标题/摘要保留可见文本。
- UNIT3D contentlength作为报告体积。无显式站点定义时enclosure.length仅是附件长度，不能假定为媒体体积；pre-dessert NexusPHP明确声明的RSS size字段仍执行其过滤器，不丢弃该库以enclosure.length报告体积的既有约定。声明的size/免费/H&R/labels规则在单条规范化entry上执行，未用日期/未知timezone不参与自动猜测。该有限元数据读取不等于整个RSS配置DSL都已支持（自定义element/list、任意URL模式仍是独立兼容工作）。
- RSS-only资源不再合并上一次运行的过期免费/H&R/labels/做种/体积信息；同一run内已有完整搜索提供的元数据仍保留。此处“同一run”是评估快照范围，不宣称并发运行和长时间重试已具备独立不可变资源快照或长期促销有效期证明。
- 无效源令订阅及对应账号结果明确失败，消息固定且说明未更新基线；既有成功提交seen/account基线不清除，无效源不提交新基线/不改已有加密下载提示。

公开契约参考（只作实现研究，不进入运行时镜像）：
- https://www.rssboard.org/rss-specification
- https://www.rfc-editor.org/rfc/rfc4287
- https://github.com/HDInnovations/UNIT3D-Community-Edition/blob/8b88f4c8182eb3d3912ffef425c3224dcfd596f4/resources/views/rss/show.blade.php
- https://github.com/HDInnovations/UNIT3D-Community-Edition/blob/8b88f4c8182eb3d3912ffef425c3224dcfd596f4/routes/rss.php

PostgreSQL回归覆盖首次错误源无基线、恢复后首次只建历史、后续路径种子落库/加密提示/已有媒体去重、重复源不再次评估、已有基线后的错误源保留seen/提示，及RSS-only不继承旧免费信息。实测前仍不标记真实PT订阅通过。静态审计仍193加载/159核心支持/34部分/0无效，accountVerified=false；没有因为RSS修复猜测timezone或提高静态支持计数。

正式部署已更新为media-tool:rss-core-20261011，数据库仍20，无新迁移；没有自动导入配置/接受许可证/创建账号/启用订阅，原STRM固定URL、实际UA取链、302与不转码策略保持。完整本地/原生amd64验证、备份恢复、发布和保护证据见rss-*报告及WORK_LOG。


### 历史RSS候选构建收尾（2026-10-11，发布前记录）

已完成linux/arm64与linux/amd64候选Docker构建；原生aarch64 Docker运行配置审计通过（UID65532、只读根/只读参考卷、无网络，193/159/34/0、accountVerified=false）。首次SSD bind挂载被Colima拒绝，保留失败日志；改为复制到本轮独立参考卷并只读挂载，未改原参考文件权限，临时容器/卷已删除。此处不是原生amd64运行、RSS实际账号或NAS设备验收。

准备了linux/amd64的sites/search/subscriptions三份独立测试二进制及校验摘要，下一步在指定x86_64服务器的临时网络/独立PostgreSQL库运行，再进行受保护发布。源码/镜像/测试manifest见data/remote-deploy/rss-core-candidate-manifest.json；诊断见data/diagnostics/rss-round-audit-20261011.json。镜像构建后的补充只涉及文档状态和一个负例XML转义修正（避免跨域URL负例先在XML语法处失败），运行代码未变；修正后的RSS回归单独通过并重新交叉构建sites测试二进制。初始源码包/构建证据保留，最终当前源码与构建源码差异由manifest明确列出，不谎称两者全部文件逐字相同。当前未创建SSH身份、未连接或更改正式部署，原STRM/不转码策略保持。


## RSS发布收尾：2026-10-11

media-tool:rss-core-20261011已发布至指定项目，仅更新server/worker/transfer-worker/library-worker，无生产迁移、PG20不变。原生x86_64测试使用全新内部网络与独立PostgreSQL容器/卷，sites/search/subscriptions共32项顶层测试（28/1/3）全部通过，无跳过；未访问生产库或使用真实PT身份。镜像配置加载审计193/159/34/0通过，静态accountVerified仍false。32项跨编译测试不带race，本地全量race证据另行记录，不混为同一次验证。

发布前备份在另一全新PostgreSQL容器恢复，并对8类密文表执行离线dry-run/回滚检查；无生产改密或主密钥轮换。所有临时容器/卷/网络和含密钥的临时目录已清理。发布后12项原STRM媒体详情及GET/POST PlaybackInfo逐字一致，四组原链接/UA请求302→cdnfhnfile.115cdn.net→206，HLS/转码仍拒绝；不声称实际设备解码或非管理员矩阵通过。密码记录/.env/compose、资产/绑定/输出/播放授权/扫描计划、5份磁盘输出和Emby/CD2/其他23容器由前后摘要核对保持。

证据：data/remote-deploy/rss-{native-pg-check,backup-restore-check,deployment,preservation-check,original-strm-check}.json与三份已校验native日志。回退镜像media-tool:before-rss-core-20261011；数据库备份/root/docker/mediatool/private/database-before-rss-core-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-rss-core-20261011。仅回退镜像/代码不自动恢复或覆盖生产数据库。

本次发布后文档使用独立docs-supplement同步，保留原源码包/镜像/manifest，只更新文档、不再重建服务。本项目PT/qBittorrent/Telegram连接、站点定义与订阅当前仍0，没有借用其他项目账号。下一步继续PT请求契约和RSS重试/多站点一致性，同时待用户真实账号完成合法资源搜索→下载→保留做种→CD2→STRM/NFO→Emby→通知、缺集/促销及设备权限/HTTPS/持续稳定性验收。整体目标保持active/未完成。


## 每轮快照与重试候选：2026-10-11（未发布）

搜索页/订阅不再把全局resources行当不可变历史证据；候选迁移21在search_run_resources保存公共快照、独立AES-GCM提示/AAD、账号revision与实际定义/继承定义/提交的SHA-256指纹、观测时间。HTTP来源证明由实际请求使用的Client捕获，保存时用短事务共享锁复核；不在网络调用期间持有账号锁。每run部分RSS只合并同一run的冻结搜索字段，而非另一run修改后的全局字段；旧无快照链接明确要求新搜索，不猜测回填。所有新下载入口和订阅预检使用该run提示，Worker不自行改用其他搜索结果；已存在的旧任务内部入口暂保留原行为。

订阅收集与决策分离：失败收集在下一次尝试清空本run临时links/seen/account和计数，若已有决策/下载效果则不自动删除；完整集合冻结后，决策阶段错误的重试保留原集合。配置变动或快照证据超过10分钟进入needs_refresh并暂停任务，成功空源也校验其原账号/定义证明。默认窗口只限制本程序使用旧证据，不保证上游免费/H&R状态在窗口内不变；真实促销仍需账号验收。

这仍是候选，不代表恢复流程已完整：暂停的已排队下载需要补专用重新确认/重绑定路径，避免通用重试永久使用旧来源；不同媒体/InfoHash的重复请求也待完整复核。正式数据库仍20，未创建真实PT账号/订阅、未重启服务器，仍须新迁移/密钥轮换/原生与设备矩阵验收。


## 2026-10-11 来源重新确认候选补充

候选已提供downloads/{id}/source-preview与reconfirm-source流程，允许对来源暂停的原pending任务选择新的有效run快照，核对PT InfoHash及qB自身按hash状态。不同hash/不同媒体/季集、配置与路径变化、活动租约、订阅停用/规则变化被拒绝；未确认旧身份的任务单独明确首次身份，不当同种子恢复。私有下载URL仅用在内部，预览/审批只返回公开证据和opaque digest。新的手动创建也预检并pinHash，确认重放（包含并发）不会重复重置原job。原订阅过滤规则与恢复共享实现，不通过重绑自动放宽配额/H&R限制。真实站点促销随时间变化仍须独立验证，10分钟快照窗不是免费永久有效承诺。

本地全量race与新恢复fixture通过，不等于已发布/真实PT可用。正式库仍20，候选21/22/9类rekey在隔离库；下一步包括原生与新schema恢复/竞争验证和受保护发布，未代导入定义或配置用户账号。


## 来源恢复发布收尾：2026-10-11

已发布media-tool:source-reconfirm-20261011，schema20→22，仅停止/更新server、worker、transfer-worker、library-worker；账号/主密钥/115授权/媒体身份不改，无PT账户或规则自动导入。全量本地race/vet、Vue9项/构建、18脚本、23E2E；原生x86_64五包30项顶层测试无跳过，使用新内部网络/独立PostgreSQL，非生产库/非真实PT资源。原生跨编译fixture不带race，本地race证据另列。ARM64原生只读无网络配置审计通过，193/159/34/0仍只是静态支持，不提高accountVerified。

补验证qB接受但返回503的未知结果按已落库Hash核对、无第二次Add；旧receipt不恢复新暂停代，来源revision/同run提示在网络读取中变化被拒绝；读取无效task详情后不以空结构覆盖原数据。初始候选不包含最后这一读取失败保护，故保留初始包并生成validated独立源码/镜像/fixture清单；正式切换的是validated版本，不替换或冒充历史证据。

发布前备份已在另一个独立PG恢复，升级22并执行9类密文dry-run回滚，再结构降20/升22验证。第一次SSH克隆派发失败后读取证实无marker/process/目录/容器/网络/卷；第二次执行输出丢失，后续发现服务器已完成的passed回执，采用回执而没有第三次执行。发布亦以持久化receipt核对，不盲目重放。临时资源、含密钥目录与SSH材料在收尾清理。

发布后12项原STRM媒体详情/GET/POST PlaybackInfo逐字一致，四组代表性播放器UA头请求302→cdnfhnfile.115cdn.net→206，HLS/转码拒绝，验证窗口无新增FFmpeg日志；不是实际设备解码或非管理员权限验收。原5份磁盘输出、密码记录/.env/compose、资产绑定/播放授权/扫描计划与Emby/CD2/其他23容器由摘要核对保持。API仍要求管理员登录，首次注册已关闭，密码新设置6–20位。

回退镜像media-tool:before-source-reconfirm-20261011；备份/root/docker/mediatool/private/database-before-source-reconfirm-20261011.sql.gz；旧源码/root/docker/mediatool/private/source-before-source-reconfirm-20261011。发布/原生/克隆/保护证据为data/remote-deploy/source-reconfirm-*报告与validated-release-manifest。文档收尾另用docs-supplement，不修改运行代码或再次重启。

真实PT/qB/外部通知连接与订阅当前仍0；合法资源搜索→下载→做种保留→CD2→STRM/NFO→Emby→通知、真实缺集/促销与设备/权限/HTTPS/容量稳定性仍未完成。下一步继续pre-dessert请求体/分页/框架等更宽契约，以及上述实际验收，整体目标active。


## 2026-10-11 请求契约与分页候选（未发布，无新迁移）

依据固定pre-dessert提交及当前解释器，先用HTTP样本复现mTorrent硬编码JSON覆盖明确body/form_data的错误。候选现规定：明确body或form_data优先，二者同时存在报配置冲突；没有声明载荷时才保留既有mTorrent默认mode/keyword/pageNumber/pageSize/categories结构。JSON typed/null、空表单、单次变量替换和query/body分离保持，明确User-Agent（含空值）不被默认UA覆盖。无新默认类别/筛选、没有代导入或接受许可证。

搜索结果JsonPath必须在声明位置取得数组；缺失/null/布尔/对象或尾随另一个JSON文档不是成功空搜索。资源字段仍只执行所需依赖，未用timezone语义仍不猜测。已声明{page}请求（path/query/body/form）或mTorrent现有默认支持页码时，最多跟进5页、500个不同ID，直到实际空列表；不按“短页”猜最后页。重复页、资源/页数上限、后续失败为partial，保留已验证结果；认证失败或明确取消不保留可使用的集合。没有分页声明仍仅请求一页，非空响应也提示partial/未检索其他页面，而不是把固定offset改成推测分页。保留既有第一页变量1，不宣称已验证所有站点零/一基分页起点；任意next-link/cursor和专用框架仍独立工作。

Worker同account重试先清本run未消费临时结果/计数，不联合旧集合；冻结的订阅或已被download_tasks选中的run不可重写。search/站点partial与HTTP失败区分，订阅可使用有效部分结果，但不把分页不完整改写为完整成功；both模式的RSS成功不会掩盖search分页警告。预演/下载仍遵守原身份、免费/H&R及种子预检，不因部分结果降低规则。

下载新增精确sourceSnapshotDigest绑定，不只记录可被重查的runID；旧无此字段的run-bound任务需要明确来源重新确认。证明按解密后的来源语义/ID/revision/定义指纹/时间计算，而非随机cipher字节，因此9类离线rekey不应把未变来源误当修改。证明/预览不公开hint，密文仍按独立AAD保存。当前正式记录基线是source-reconfirm/PG22，本轮未连接或改动服务器。

静态审计额外显式指出此前未执行的required-header鉴权：193加载、158核心规则支持、35partial、0invalid、accountVerified=false。sunny因X-Csrf-Token限制从parser_supported改为partial，zhuque补同类notes；这是真实已有解释器限制的揭示，不是撤回已实现的登录能力或用删除note提高计数。timezone、浏览器与专用鉴权仍不因fixture获得“实测兼容”。证据见pt-contract-*及pt-pagination-*日志，后续须原生与保护发布及真实PT链路验收。


## 2026-10-11 混合搜索/RSS来源证明加固（候选，未部署）

有效HTTP采集的来源证明与站点搜索状态分离：both模式中RSS成功但搜索保留failed/partial警告时，空RSS也必须参与revision、配置指纹、观察时间校验。失败搜索且没有任何成功证明仍不被假作成功采集；不完整证明明确拒绝，异常未来时间也拒绝。

同一run/account的后续采集只可使用相同来源revision/定义指纹；即使两次都为空，也不以较新的证明覆盖旧搜索证据。相同来源保留最早observed_at，不通过RSS刷新早期证据的有效期。没有来源记录/没有完整HTTP client证明时不落公共资源。

收集冻结和最终RSS基线/seen提交前在同一事务内再次校验全部成功来源，并持有run、来源记录、账号与现存定义行的共享锁直到提交。覆盖/决策阶段修改空来源账号或定义，运行进入needs_refresh，基线/seen不落库；不是只在第一次HTTP请求结束时检查。使用真实PostgreSQL关系锁形成确定性提交窗口，不向正式逻辑植入测试回调。

旧run-bound下载任务若没有新sourceSnapshotDigest，未确定远端身份时须暂停、预览并明确批准新来源；不能自动补摘要或取最新全局提示。专用批准复用原Task/Job且重复收据不重复Add。已落已知InfoHash且qB实际确认存在的未知提交，则无需回取过期来源/缺少摘要的提示，按原身份核对即可恢复。

本地全量隔离race/vet、Vue9项/构建、脚本18项通过；首次E2E为23/24，扫码状态等待超时保留。补加fixture轮询状态/可见性诊断后完整24项通过，未修改授权运行逻辑、延长断言或宣称真实115授权故障已定位。只跑授权测试未包含首次管理员注册，因此该次5个login失败不能归为授权轮询复现；其日志明确保留。原生amd64、双架构候选、保护发布与真实账号链路仍以独立证据为准，不沿用上一个镜像的测试来证明本候选。


## 2026-10-11 13:28 候选发布完成

上述请求/分页与混合来源候选已部署到指定项目；native AMD64六包83顶层测试无跳过，sites39/search13/subscriptions8/acquisition17/secrets4/协议迁移2。静态193/158/35/0只是规则范围审计，仍accountVerified=false；未导入站点、借用账号或替代许可确认。备份克隆九类密文dry-run保持原字节，schema22不变，发布后固定STRM/四组UA302/CDN206及拒绝HLS/转码保持。旧“未发布”段落为历史阶段记录，当前以pt-mixed-proof-*证据为准。完整合法PT资源链路/实际订阅仍需本项目自己的配置与验收。


## 2026-10-11 14:xx 声明请求头与HTTP origin候选（未部署）

固定提交当前sunny要求Authorization（6个请求），zhuque要求X-Csrf-Token（8个请求）；之前部分过程记录把sunny也泛称CSRF不精确，以实际固定JSON为准。该快照没有声明通用header生产请求，因此不能根据名字发明登录URL、meta选择器、Bearer前缀或CSRF取得方式。缺少值的站点仍partial；timezone和专用浏览器鉴权不因此获得“已验证”状态。

先用自有HTTP/TLS提供方复现：非APIKey的required_headers即使由headers明确提供仍ErrConfig；允许域名间重定向会带出Authorization/CSRF/自定义头；HTTPS可跳HTTP；307会重发原JSON私密数据；origin忽略scheme/大小写/默认端口。候选现在执行明确且完整的header声明，缺值/缺模板变量为鉴权失败；类型、重复大小写、非法name/CRLF/Host覆盖/无效required数组等在HTTP前拒绝，不在错误中输出值。APIKey原有字段优先，不猜其他凭据。

origin按scheme、DNS主机名和有效端口识别。Cookie限定账号origin；APIKey必需头限定当前声明的请求base origin。同origin正常重定向保持凭据；允许列表内的跨origin GET/HEAD仍可取公开内容，但声明头、required头、Cookie/Authorization/APIKey/CSRF/Referer等被清除，之后返回初始origin也不会重新复制。跨origin原请求体/非GET/HEAD重发及HTTPS降级拒绝；调用者CookieJar不会绕过显式账号Cookie边界，且不修改Jar本身。未授权的域名不访问，下载/CDN提示仍需实际允许的origin，不使用公共伪样本来替代真实站点。

本地四包隔离PG/full race/full vet、24E2E通过；静态193/158/35/0、accountVerified=false未提高。此为PT请求协议候选，不修改已发布STRM解析/115 UA/无转码规则，未连接服务器或自动导入配置。原生/保护发布和缺少producer的实际授权仍单独验收；不能把已声明头执行修复写成sunny/zhuque自动登录已实现。


本候选最终补检还复现初始绝对additional-domain请求携带已声明头/私密Body的漏洞：不是只有重定向需要origin边界。现当请求target不同于所选base/API origin，只有无声明头、无Body的GET/HEAD公开取链可继续；任何定义私密材料或写请求在HTTP前拒绝。最终代码再次全量隔离race/vet通过。第一次最终E2E为22通过/2个UI等待超时（登录和稀疏集合行点击），已保留原错误上下文；确定旧进程/端口已清理后原样完整复核24通过，没有增加超时、修改UI或强制点击，未据后一次通过宣称最初根因已解决。


## 2026-10-11 附加鉴权头账号入口（候选，未部署）

在PT服务连接增加`secrets.requestHeaders`，为完整JSON对象字符串，仅进入现有AES-GCM凭据，不进入公开settings；例如Authorization需用户输入完整值，程序不补Bearer前缀、不把其中{passkey}当模板。16项/总16KiB/单值8KiB界限，重复JSON键及大小写别名、非字符串、空值/控制字符、Host/Cookie/代理/转发/传输控制头等在加密前拒绝，错误不含值。Cookie仍用独立字段。

每次请求仅提取其`required_headers`明确列出的名字，限定所选base/API origin；原APIKey字段优先，其次账号附加头，再其次配置明确头。未声明名字不发送，跨来源不转发，不自动获取/刷新CSRF或Bearer。留空UI输入省略并保留原凭据；`{}`明确清除该整组，不轮换其他Cookie/APIKey。改endpoint仍要求替换或清除旧凭据；任何账号修订使旧采集证据失效，不能用新头覆盖旧run证明。

管理接口只返回requestHeaders字段存在与否，不回显头名和值；旧账号无此字段保持原路径，不增加迁移或加密表。隔离PG验证密文/DTO/审计脱敏、来源修订保护、编辑保留/明确清除和原9类rekey dry-run。Vue入口默认掩码、支持显式显示、JSON/重复键错误贴近字段；长表单滚动保持关闭入口可见，桌面及375px采样另记。此不会把sunny/zhuque缺少自动生产者改标为已实测支持；固定库仍193/158/35/0、accountVerified=false。


本候选补齐“先验证后启用”PT流程：明确的管理连接检查可只读验证未启用账号，但Site业务入口仍拒绝未启用账号，不改变Enabled或账号revision，不创建搜索/下载/订阅。隔离PG正例先复现旧检查失败，再按私有只读构造路径修复；测试验证检查前后业务拒绝和编辑开启后的凭据/来源保护。最终代码再跑全量隔离race/vet，E2E/新原生与候选发布状态以本轮记录为准。之前一次系统检查UI等待超时后读取隔离库，job实际在约2秒内succeeded；这不是证明UI定时/SSE根因已解决。追加只在失败时输出无secret状态/页面可见性诊断，未延长等待或改变Worker行为，之后完整25项通过。


## 2026-10-11 16:21 请求头入口/origin候选发布完成

前述候选整体已部署，不仅是测试里的headers变量：管理员可在PT连接显式输入附加鉴权JSON；现有AES-GCM保存/脱敏、修改/清除、读取、未启用账号只读验证与必需头注入链路已落地。原生七包115项无Skip（sites56/integrations15/acquisition17/search13/subscriptions8/secrets4/协议迁移2）、备份克隆九类密文dry-run及生产保护回归通过。来源证明/无转码/原STRM链路不变。

程序依然不补sunny Authorization或zhuque CSRF生产规则，manual值只执行实际声明required名字和origin；静态193/158/35/0及accountVerified=false保持，不能把手工入口等同真实站点授权或自动刷新。无自动站点导入/许可接受、真实PT取种/订阅仍需用户自己账号。新证据为pt-request-headers-*，老候选段落只是历史过程记录。


## 2026-10-11 PT执行契约候选（本地修复，尚未部署）

固定配置的35个partial原因已逐项核对：21个站点含未验证的timezone算术语义、13个专用框架需真实验证、2个缺少明确鉴权生产者、1个声明浏览器执行；分类存在重叠，不相加冒充站点总数。无配置/账号依据时不实现猜测时区、PCRE替代、浏览器挑战或自动Bearer/CSRF。静态范围仍193加载/158解析支持/35部分支持/0无效，accountVerified=false。

本轮先复现并修复另一项确定问题：chrome/disabled及parser声明曾被普通HTTP/HTML-CSS默认路径忽略。新增独立execution_contract模块，chrome=true、disabled=true、非布尔执行标记、未知/非字符串解析器和缺失请求规则在HTTP前拒绝；搜索预检位于账号验证之前。None只用于原始取链/文件字节，不用于搜索/登录/令牌文档字段提取。保留未声明/空字符串的既有CSS缺省和受支持解析器大小写别名。显式RSS URL也不能绕过已声明浏览器/禁用规则；没有rss定义的手工RSS仍可使用。管理员明确检查未启用账号的旧能力不变，这与定义中的disabled请求标记不同。

兼容判定从Repository拆为独立模块，新增排序去重的固定issueCodes；离线审计增加issueDefinitionCounts，按涉及站点数而不是同一站点重复字段数统计。审计不输出原始解析器名、URL、字段名、选择器、请求体、头名/值；筛选器只扫描fields/flat_map_fields及any分支，不再把请求JSON/参数/constant/case/default中的同名filters数据当执行指令。旧jsonb分支顺序缺失的限制仍保留，新增安全原因码不会恢复顺序或自动重导入。

回归包括HTTP前零请求、缺失令牌不能请求站点根路径、浏览器登录不能假阳性、RSS声明保护、字段/普通数据边界、原因统计脱敏。隔离PG搜索回归确认上述失败为failed且不产生空集合的source_revision/definition_fingerprint/observed_at证明，不创建可用资源。schema保持22，无新增迁移；播放器/Emby/115/CD2代码未修改，原STRMURL→按实际UA取115临时链接→302原则不变。

当前本地全量隔离race与vet、Vue14单位测试/构建、Python18脚本测试通过；完整E2E及新的双架构/原生发布证据另行记录，不能复用上一版本的原生115项证明。本轮未建立SSH会话或变更指定生产服务器。当前正式发布仍pt-request-headers-20261011/41ac.../schema22。实际PT/qB/做种/CD2/STRM-NFO/Emby/通知、真实订阅/设备/权限/HTTPS/容量门槛仍待完成。


本候选补充验证：当前完整E2E25项通过，未修改现有E2E用例、超时、重试或期望；未增加UI改动，也不将fixture页面等同真实管理员浏览器登录。全部测试在本地隔离media_tool_test/Redis15顺序执行。第一轮完整Go测试日志各包通过，但外层zsh使用只读status变量导致结果收集命令退出；已保留该日志并原样重新执行完整race及vet，第二轮显式exit=0。没有据包装命令问题改变业务逻辑或放宽测试。


本候选构建/收尾：ARM64与AMD64 Docker镜像已生成，当前源码与打包内容391文件在构建结束逐项一致；源包、两个镜像归档及8个AMD64 CGO0测试二进制校验均通过。ARM64新镜像在UID65532/只读根/无权限提升下启动，loopback随机端口health与login资源200；断网静态审计原因计数与193/158/35/0一致，参考配置不在runtime镜像，任务临时容器/volume已移除。这是缺依赖启动/静态资源检查，不是管理员浏览器登录或依赖全链路。AMD64仅完成构建/交叉编译，物理x86_64的8包原生执行尚待进行。

候选manifest：data/remote-deploy/pt-execution-contract-build-manifest.json；ARM64运行证据：pt-execution-contract-local-image-check.json；本轮综合审计：data/diagnostics/pt-execution-contract-round-audit-20261011.json。没有SSH或生产更改，正式版本仍pt-request-headers/schema22。本段为本地构建后的文档补充，不重写源包/镜像/测试二进制；文档差异单独保存在pt-execution-contract-local-docs-supplement.json，运行代码与打包版本不变。

下一步先执行新的物理AMD64原生8包测试（含离线审计新回归），再重读指定服务器状态、恢复验证新备份及九类密文dry-run、保护发布并回归原STRM链接302到115 CDN与无转码/媒体中转。不得复用旧镜像115项原生证明；真实账号链路、订阅/设备/权限/HTTPS/容量仍保留为整体验收门槛。


## 2026-10-11 18:31 PT执行契约候选已完成指定服务器发布

本轮按用户“部署到服务器测试”执行，正式镜像`media-tool:pt-execution-contract-20261011`，实际ID`sha256:3305cc9054775d72f4c001aea93bafad502ceb0f708984de7fa181caaf90c0e2`。schema仍22，无新迁移；只更新server与三个Worker，既有管理员密码/主密钥配置、115授权身份、2资产/绑定/播放Key、原5个输出文件与Emby/CD2/PG/Redis/migrate保持。新状态/保护证据均为pt-execution-contract-*，不能用上一版本的原生115项代替本次验证。

新源码包391文件与两个架构归档校验通过。指定物理x86_64、默认CPU条件下，fresh内部PG/network/volume、应用只读UID65532，8包128个顶层测试全部通过且无Skip：sites66/integrations15/acquisition17/search14/subscriptions8/secrets4/协议迁移2/离线审计2。8份原始fixture日志独立取回并逐项核验测试名/摘要；CGO0交叉编译后二进制原生执行不是race，本地完整race/vet证据单列。固定站点参考194文件逐字摘要匹配；静态193/158/35/0及accountVerified=false不虚增，不自动导入站点或接受许可。

新生产备份在独立PG恢复成功，候选migrate幂等保持22；九类现有密文默认dry-run可读、committed=false，全部原密文字节及原事实保持，不轮换或导出生产主密钥。回退镜像`media-tool:before-pt-execution-contract-20261011`；旧源码`/root/docker/mediatool/private/source-before-pt-execution-contract-20261011`；备份`/root/docker/mediatool/private/database-before-pt-execution-contract-20261011.sql.gz`，SHA256 `a42ff260d9b21e8e0599e7d3332a9b9fbcf90e19e63c693a754a1398757ec963`。本次原生、克隆与切换都单次派发并记录PID出生时间/持久回执，遇到未知输出只读回查，不盲目重复切换。

切换后12项媒体信息/PlaybackInfo逐项返回原始STRM固定URL，不改为25015/Videos/.../stream；4组代表UA分别原URL302→cdnfhnfile.115cdn.net→206/1024字节Range通过。坏EmbyToken401、HLS/转码409且无新FFmpeg日志。仅限直链/元数据及小范围字节验证，不等于实际播放器解码、全部音轨字幕/长seek或非管理员验收。公网管理ready/login及Emby网关公开信息200；管理接口仍需登录，注册关闭，新密码6–20位。服务资源含原加密附加头入口，未绕过登录或读取已存头值；没有改UI或宣称实际管理员浏览器操作。

初始上传窗口其他容器ID摘要发生变化，部署保护先暂停：只读元数据发现其他项目duanju/juku在18:05:46重建，早于本次18:15原生Docker测试开始，任务此前仅上传文件，未操作该项目。触发方/原因未确定，历史Docker事件不足以恢复全过程，原观察及差异保留。MediaTool全部保护项前后一直相同；随后两次切换前清单稳定，切换后24个非本项目容器ID与切换前一致。不能写成“整个上传窗口所有其他容器ID从未变化”。相关证据为unrelated-change/created-observation、unrelated-drift-reconciliation及precutover/after。

18:30正式读取PT账号/qBittorrent/外部Telegram/站点定义/订阅仍0，3Worker在线、1个已授权115身份。真实合法PT→qB→保留做种→CD2→STRM/NFO→Emby→外部通知、真实订阅、设备/非管理员权限/HTTPS/NAS容量门槛仍未完成。现有115/Emby/CD2配置无需重设。当前批准的公网HTTP不是传输加密；未悄悄改访问范围或开启中转/转码。整体项目未宣布验收完成。


## 2026-10-11 文件/媒体管理六项更新已发布

指定服务器当前正式镜像`media-tool:files-management-20261011`，ID`sha256:1391e89d6c90a6b83fb900e2e0da9083d42fb8429042fa9f62fbfc6f81ff6ce2`，数据库22→23。六项用户更新均已接通：115/CD2影视自定义命名/模板/分类移动及侧车、独立服务配置路由、115与明确本地根目录管理/回收恢复、程序与Emby媒体库浏览、115无Token保存待授权连接、显式会话/CSRF/no-store密钥回显。具体入口、界限和操作方式见FILE_MANAGEMENT.md。程序不会自动重命名用户现有影视，必须选择文件预览并确认；无永久删除、无视频中转或转码。

本轮最终全量隔离race/vet、Vue17项、Python18项及完整28E2E通过；桌面/375px截图已实际核对，手机文件操作改为卡片，预览取消没有文件副作用。新物理x86_64默认CPU、只读UID65532、fresh内部PG+独立Redis15、无测试端口发布，13包207个顶层测试全部通过无Skip：sites66/integrations16/acquisition17/search14/subscriptions8/secrets4/协议迁移2/离线审计2/filemanager8/catalog1/library25/clouddrive2 16/httpapi28。library的既有显式fixture exporter只在该测试容器的空/owned-fixtures tmpfs运行，未排除用例；13份原始日志独立核验。初始两次隔离环境在migrate连接检查失败、未开始测试包，已清理且失败记录保留；第三次使用TCP和认证SQL就绪检测后通过，未改变应用镜像/二进制或放宽测试，不将旧Unix就绪报告当成真实网络连接证明。

新生产备份在fresh PG恢复，22→23/idempotent23及九类密文dry-run通过，迁移及dry-run全部原字节保持；旧镜像的九类密文只读工具也能读取23，未自动降库/恢复生产库。回退镜像`media-tool:before-files-management-20261011`，旧源码`/root/docker/mediatool/private/source-before-files-management-20261011`，备份`/root/docker/mediatool/private/database-before-files-management-20261011.sql.gz`（SHA256 `5bca7248a7d2d634a4b6e9b7aa3d59488ccc17143a691791491cce2f816fec71`）。新目录/文件操作记录有下迁移门槛，不在仍有本地连接或未核对操作时破坏性回退。

仅切server与三个Worker；管理账号密码、主密钥/配置、115授权身份、资产/绑定/播放Key/计划与原5输出字节保持。24个其他容器的ID从本轮最初清单到切换后一致。Emby/CD2/PG/Redis/Emby-loopback保持，migrate一次性容器本轮前后均不存在，不能凭旧发布记录假定存在。12项媒体信息/PlaybackInfo为STRM原链接，四组代表UA原URL302→cdnfhnfile.115cdn.net→206/1KiB通过，坏Token401、HLS/转码409且无新FFmpeg日志。

公网新页面与对应服务bundle200且含实际功能入口；匿名文件/媒体库/回显API401，注册保持关闭、密码6–20位。服务静态探测初版错误依赖会被压缩的JS变量名，改为稳定功能文案/路径后通过，未修改运行代码；同样未绕过真实管理员登录或声称真实影视批量整理。目录权限、客户端解码/全部字幕音轨、完整PT/qB/通知业务与HTTPS/NAS容量验收仍是整体门槛，不凭207个fixture宣布全项目完成。
