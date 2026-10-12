# HDSKY精简站点模板

`hdsky.json`是供本程序sites模块读取的独立模板。无需NexusPHP公共配置；用空对象`{}`作为common即可。不是插件，不包含账号Cookie、Passkey、用户ID、API密钥或下载凭据。

## 仅保留实际需要的4个请求

| 请求 | 用途 | 字段 |
|---|---|---|
| user_basic_info | 添加账号时验证登录、同步账号统计 | is_login、id、name、uploaded、downloaded、ratio、bonus |
| user_details | 用户等级 | level |
| search | 站点直搜、TMDB关联资源搜索、订阅资源发现、取种入口 | id、title、details、download、size、seeders、leechers、downloadvolumefactor、description、labels |
| seeding_statistics | 做种数量、体积、完整分页统计 | size（HDSKY第三列） |

- 取种使用搜索结果中`form[action*='/download.php?id=']`的真实action，不凭空构造下载地址/Passkey。不新增无用的download请求。
- RSS地址仍在PT账号中配置，由程序通用RSS模块处理。无需复制站点生成RSS、RSS页面解析或登录/签到接口。
- 分辨率、来源、编码、季/集、标题/年份由主程序0day解析和筛选功能处理，不复制上游搜索面板/category/封面/下载完成数/发布时间/IMDb/豆瓣等未消费字段。
- 删除签到/验证码、站内信、公告、字幕、详情抓取、聊天、favicon、HR列表等非本程序业务功能及所有继承。
- 未证实的促销/H&R保持未知，不把缺失标成免费或无HR；不会为了订阅通过而关闭安全检查。无HR证明的资源需按既有订阅规则人工明确允许。
- 跟随页面给出的下一页地址，必须同源、同路径且保留搜索/用户身份，只允许数值分页变化；最多5页/500条。空白/认证挑战/重复页面/解析失败为partial，不能推断“没有资源”。
- 做种接口只有经过登录校验、身份/路径匹配的明确`没有记录`响应才确认0；其他空白或异常不显示假0。

## 使用与验收边界

当前后台“导入pre-dessert-sites”仍是上游固定快照导入，不是任意JSON上传器；不能把本地路径填到站点ID或来源提交栏。本文件用于独立配置/二次开发，加载示例：

```go
raw, err := os.ReadFile("examples/sites/hdsky.json")
if err != nil { return err }
config, err := sites.Decode(raw, []byte(`{}`))
```

正式保存可调用现有sites.Repository.Save，common使用`{}`，来源用文件内容摘要标记本地模板（不要冒充上游提交）。保存会改变定义指纹，从而让旧搜索/账号统计证据失效；必须走已有显式定义更新和来源重新确认流程，不能直接SQL替换正式站点。本轮不会自动覆盖已添加的HDSKY账号/定义。

离线验证：`go test ./internal/modules/sites -run TestHDSKYMinimalTemplate`。测试使用自有合成HTML和受控HTTP transport，不联系PT站点、不下载真实种子。

模板选择器精简自mantou568/pre-dessert-sites提交`37bf5c5064ae437f54e26b57406ca84d480daab5`的`site_config/sites/hdsky.json`及`site_config/commons/NexusPHP.json`。保留上游署名与原许可文本于LICENSE.pre-dessert-sites。此模板文件不打包进运行镜像；发布/使用需遵守上游条款。原HDSKY统计/搜索已有前轮只读实测；本精简版本本轮仅有合成页面验证，不冒充新的账号实测。
