# CHDBITS / SSD / OURBITS / UBITS / TTG 精简站点模板

日期：2026-10-13。承接HDSKY模板，只保留主程序消费的登录、账号统计、等级、搜索/订阅资源发现、真实取种链接及做种统计。删除签到/验证码/站内信/字幕/聊天/上游搜索表单等未使用功能，模板不是插件，无账号秘密。

## 固定参考与差异

pre-dessert-sites固定提交37bf5c5064ae437f54e26b57406ca84d480daab5；CHDBITS对应源ID ptchdbits（彩虹岛），不编造新的上游ID。TTG搜索字段由MoviePilot固定提交6ef8408f432dde643aee2ba7078f59d2776f84ce的test_indexer_spider_search_url.py交叉核对，使用search_field/c=M，不误用Nexus search。

- CHDBITS：标题b、circle-text H&R、subtitle/tag、第三列做种大小。
- SSD：魔力title，span促销/标签；做种接口两个b为总数/总体积，不能强套列表。
- OURBITS：rowfollow第五列资源大小，hitandrun标签，第三列做种大小。
- UBITS：描述/标签、hitandrun标签；做种接口“条记录/总大小”的汇总，不能强套列表。
- TTG：browse.php、torrent_table、name_left、dl_a、不同列号、userdetails做种列表。

## 工作与验收顺序

1. [x] 五个独立JSON模板与索引/说明，字段白名单、原授权文本/署名、无继承/秘密。
2. [x] 新增显式summary做种统计分支：同源/路径/用户绑定、唯一汇总容器、严格整数/大小、0与未知/部分结果分开。
3. [x] 搜索容器证据与实际next链接，拒绝挑战/错误容器；保持同源/同查询/5页500项边界。
4. [x] 五站点合成页面、未知促销/H&R、两位数H&R、分页/取种/统计/挑战/零结果/非法响应回归；不声称真实账号实测。
5. [x] Go race/vet、Vue32/构建、Python18、E2E52串行测试；提交全部新增文件。
6. [ ] GitHub→Actions AMD64→不可变Docker Hub→服务器仅app更新、保护检查和原STRM302回归。

## 边界

不登录未提供账号的站点，不处理验证码或绕过反爬。RSS使用账号配置URL及既有通用模块。无促销证明不推断免费/付费；无H&R证据不推断无HR，不用固定“2/3天”代替站点证据。新模板是本地文件，后台仍为上游固定快照导入；本轮不增加任意JSON上传，也不自动覆盖正式定义。模板不进入运行镜像；服务程序包含所需解析能力。此计划的发布证据以最终回执为准。

补充：移除TTG上传/下载XPath字段从公共配置残留的selection=html，防止把上传值同时读成下载值；保留站点框架标识与实测门槛。汇总数字使用完整文本匹配，负号不能被regex截掉变成正数。

TTG只在第六个站点徽标的tooltip具有H&R/保种/考核语义时读天数，不将2160p或0day等无关数字解释为免HR。汇总响应含form/password输入直接拒绝，即使页面回显合法数字；额外查询范围也不当作全量统计。

本地验收：五站点及HDSKY回归、完整Go race/vet、Vue32/构建、Python18、E2E52均通过。新增认证假计数/额外汇总范围及三站点真实next链接两页统计另行补测通过。发布仍以最终Actions及服务器回执为准。
