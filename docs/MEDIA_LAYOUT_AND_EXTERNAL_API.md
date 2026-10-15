# 分类命名、STRM目录与外部API

2026-10-15 · 0.26.0-dev · migration49

## 分类与模板

目录分类采用有序分类组/子规则，首次匹配。不同字段同时满足，同字段多个值任一匹配；未知地区/原语言不猜测。配置修改只影响新预览及新任务。新上传准备时冻结模板、分类、确认身份和源文件名；云端文件名与STRM使用同一布局，STRM扩展名为.strm。不再添加资产短ID，重名不会自动覆盖或生成随机后缀。

旧输出和云端文件保持原值。需要按新模板更新时，在云盘索引批量整理/输出迁移中预览、批准；原STRM正文与播放Key不改变，手工输出不覆盖。季0和明确多集覆盖保留；非连续多集仍投影单集STRM，播放的是同一个完整源文件，不切片。

## 进度与自动确认

处理进度表示校验/传输/落盘/入库阶段；实际传输字节另列。秒传/复用不伪装为完整网络上传。新输出及修复完成后排队Emby定向刷新，现有cron继续检查，只有逐路径确认后显示可见。刷新请求成功不表示完整解码通过。

全局STRM检查不要求选择，不受列表筛选影响；预览整库缺失/完整/冲突项，再明确批准。只恢复同摘要的可信原文，跳过手工更改/归属冲突，不更换文件Key。

## 外部API

管理员在“外部API”创建独立凭据，默认只读，全部永久有效（包括已有凭据）、可主动撤销、每分钟120次限额。通过HTTPS发送 Authorization: Bearer，不使用URL参数、管理员密码或后台Cookie替代。后台可回显完整凭据，数据加密存储；外部接口不暴露管理员凭据、原始数据库及PT鉴权参数。

- status.read：GET /api/external/v1/status
- library.read：GET /api/external/v1/strm?kind=strm&mediaType=movie；GET /api/external/v1/strm/{assetId}/content?path=URL编码的STRM相对路径
- layout.preview：GET /api/external/v1/classification；POST /api/external/v1/layout/preview（item媒体身份/分类字段、source源文件名、season、episodes）
- tasks.read：GET /api/external/v1/downloads；GET /api/external/v1/transfers
- search.run：POST /api/external/v1/search（keyword，或media:{id,mediaType}）；GET /api/external/v1/search/{id}
- downloads.run：POST /api/external/v1/downloads（searchRunId、resourceId、mediaType、tmdbId；更正需明确correctMediaIdentity）

STRM正文包含该媒体的固定播放授权链接，授权library.read者可读取这些链接；下载权限将产生真实写入，应谨慎授予。调用记录不收集Authorization值。错误响应区分401无效/过期/撤销、403缺少权限、429限额。

TMDB关联搜索的缓存含媒体ID/类型；创建任务时原子写入已核验身份。按资源发行标题、电影年份、媒体类型核对，不用副标题同名别名冒充身份。不确定结果被排除并计数，下载入口再次检查。

## 上传后清理

默认关闭。启用只冻结到新下载任务，旧任务须逐项批准。全部媒体上传、云端实时大小/SHA1/提供方ID复核、入库完成、本地SHA1、qB真实hash/完成状态/目录/标签/文件清单、无其他任务/种子共享文件才清理。H&R未知或有要求时必须明确确认已满足做种要求。

删除由qB仅删除已核对的种子及其文件，不对下载根目录递归删除；云端媒体/STRM/播放Key保留。先持久化删除意图，结果未知进入uncertain，不盲目重放。只读核对种子及本地文件都不存在后才完成；仍存在时保留记录并要求人工检查。

验收删除只使用隔离、短小的自有测试文件；生产不会自动启用或批量清理历史任务。
