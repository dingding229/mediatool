# 站点模板已迁移

唯一模板仓库：`https://github.com/dingding229/mediatool-site`。

HDSKY、CHDBITS（ptchdbits）、SSD、OURBITS、UBITS、TTG模板、索引与源许可均已迁入该仓库。主程序只接受templateVersion=1的原生独立模板，不再兼容原站点库、公共配置继承或无版本模板。

后台在PT站点页面读取内置仓库目录、选择站点并确认许可后同步固定快照；无需填写仓库地址。部署时可使用显式site-template-sync CLI。详情见docs/NATIVE_SITE_TEMPLATES.md。

internal/modules/sites/testdata/repository仅保留固定公共快照用于离线回归，不是运行时来源，也不对外提供模板导入入口。
