# 原生站点模板仓库

更新：2026-10-13。主程序唯一内置仓库为dingding229/mediatool-site；旧pre-dessert-sites模板不再导入或执行。当前仓库发布HDSKY、CHDBITS（ptchdbits）、SSD、OURBITS、UBITS、TTG六个原生模板。

## 原生契约

根templateVersion必须为数字1，独立完整requests；不支持reuse_schema、disabled_extends或非空common配置。schema只是本程序请求解析框架标识，不表示接收旧模板。来源许可/署名仍在模板仓库保留，不等于运行兼容。

目录index.json使用catalogVersion=1、repository=dingding229/mediatool-site；每项具有id/name/file/templateVersion/sha256，文件仅能为sites/<id>.json。修改模板需同步其原始UTF-8字节的摘要；索引不接受外部URL、路径穿越、重复ID、未知版本或其他仓库。

主程序只查询内置仓库main一次，再用返回的完整40位提交获取该版本索引和文件。可显式填写同仓库提交用于恢复，不自动取另一个版本。校验摘要、ID、名称和版本后才原子写入；失败不留下半批，提交结果未知必须读回，不能盲目重复。主程序不下载原公共配置，也不回退旧仓库。

## 后台与部署

PT站点页提供内置目录读取/选择、固定快照同步、已导入定义与来源检查。旧来源保留历史记录，但显示不支持，不能按旧仓库SHA执行“同快照恢复”；需要从原生仓库重新导入同ID定义。账号Cookie/密钥、ID关联及订阅不被删除或自动改规则。

数据库schema33添加source_repository。导入保存新来源并改变定义指纹，旧搜索/指标缓存及来源证明不能用于继续下载；已有任务仍按既有来源重新确认规则处理，不能把模板更新当做下载重试授权。

部署CLI（仅内置仓库，需明确许可确认）：

```sh
docker compose exec -T app /app/site-template-sync -license-accepted -source-commit <mediatool-site完整SHA>
```

CLI只更新站点定义，不读取PT凭据、不开始下载/订阅、不修改媒体。常规更新不会自动同步模板；本次用户明确要求仓库切换，发布后从本轮固定提交同步6个定义，备份原定义保留回退能力。

## 测试和限制

原生格式旧包拒绝、目录路径/来源/摘要、固定提交及原子写入等均离线测试；testdata只保留公共固定快照用于合成HTML回归，不是运行入口。HDSKY沿用现有账号只读验收，其他5个新站点仍未真实账号验证。

离线审计使用原生仓库根目录（sites/*.json），不再读取commons：

```sh
go run ./cmd/site-compat-audit -directory <原生仓库目录> -source-commit <完整SHA>
```

审计只证明格式/解析支持，不证明账号、促销或站点页面可用。TTG仍保留专用框架实测提示。原STRM→115 CDN302、不转码/中转策略不变。

本轮模板仓库固定提交：`f28a0b70a3541ad740adb4aa3dae4b722a6df90b`。
