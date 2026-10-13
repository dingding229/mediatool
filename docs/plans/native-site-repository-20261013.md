# 原生站点模板仓库切换（2026-10-13）

## 目标

所有6个模板迁入dingding229/mediatool-site，主程序内置该唯一仓库。取消pre-dessert-sites导入、common/reuse_schema/disabled_extends继承及无版本模板运行支持；保留来源署名/许可证，不等于格式兼容。

原生templateVersion=1，独立完整requests；仓库catalogVersion=1索引列出ID/名称/相对路径/SHA256。解析main一次固定40位提交，再下载同提交index和模板，限制路径、来源、大小、数量、摘要与契约，全部验证后原子写入。无可替换仓库URL，不下载原公共配置；失败或未知提交不自动重试。

## 工作步骤

1. [x] 发布mediatool-site的sites/*.json、index.json、许可/README；保留原ID（CHDBITS=ptchdbits）。
2. [x] 后端原生契约/唯一仓库目录与固定快照导入；旧格式、继承、摘要不符、跨源路径等拒绝。
3. [x] schema33记录source_repository；旧定义保留为不支持而非偷偷执行。账号密文/关联不删除。新定义更新改变fingerprint，旧证据不重用。
4. [x] Vue内置仓库目录选择/原生格式说明/恢复门禁，不显示原仓库或M-Team兼容承诺；新增明确CLI同步（仅官方仓库/确认许可）。
5. [x] 更新离线审计与testdata；原子性/旧包拒绝/缓存门禁、Go race/vet、Vue、Python/E2E串行回归。
6. [ ] 主仓库GitHub→ActionsAMD64→DockerHub不可变镜像→服务器保护发布；从新模板仓库固定提交同步6个定义，不改PT账号/订阅规则，不开始真实下载。
7. [ ] 保护核对、原HDSKY只读搜索/统计、原STRM302/CDN与克隆恢复验证；原始凭据不外传。真实5站点账号仍未实测，不伪称通过。

## 保护

仅app/postgres/redis官方Compose，外部Emby/CD2及其他容器不变。保留旧定义与更新前数据库/源码回退备份。切换站点定义是本轮明确预期变更；账号、媒体/STRM/密码/115授权保留。全新模板仓库初始README必须检查并保留用户有效内容。上传只有模板公开数据、无生产URL/Cookie/Token。

模板仓库已推送固定提交f7bbf0418338c4be8c66310e29ecef92783fbecc，主仓库只保留冻结testdata，不再拥有运行模板副本。

本地验收：原生契约/旧格式拒绝/固定目录与SHA256/原子导入/旧来源不执行、Go全量race/vet、Vue32/构建、Python18、E2E53（桌面与390px目录/旧来源恢复门禁）通过；模板仓库6文件线上摘要已按固定提交逐个核对。主程序发布与正式定义切换仍以本轮Actions/服务器回执为准。
