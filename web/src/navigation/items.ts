import { Monitor, Film, Search, Connection, Collection, Download, FolderOpened, Cloudy, Link, Clock, Bell, Operation, Files, Coin, Setting, SetUp } from '@element-plus/icons-vue'
import type { Component } from 'vue'

export interface NavigationItem { id: string; name: string; icon: Component; keywords: string }
export interface NavigationGroup { id: string; name: string; items: NavigationItem[] }
export const navigationGroups: NavigationGroup[] = [
  { id: 'browse', name: '媒体浏览', items: [
    { id: 'dashboard', name: '总览', icon: Monitor, keywords: '首页 状态 工作台' },
    { id: 'media-server', name: '自研媒体库', icon: Film, keywords: '内置 Go TypeScript Provider 播放接口' },
    { id: 'player-access', name: '播放器访问', icon: Monitor, keywords: 'UA User-Agent 白名单 限制 播放器 访问记录' },
    { id: 'media-users', name: '媒体用户', icon: Collection, keywords: '账号 权限 登录 独立媒体用户' },
    { id: 'mediaLibrary', name: '媒体库', icon: Film, keywords: 'Emby 电影 剧集 播放' },
  ] },
  { id: 'acquire', name: '资源获取', items: [
    { id: 'search', name: '资源搜索', icon: Search, keywords: 'TMDB 站点直搜 搜索历史' },
    { id: 'subscriptions', name: '资源订阅', icon: Collection, keywords: 'RSS 订阅规则' },
    { id: 'sites', name: 'PT 站点', icon: Connection, keywords: 'HDSKY 账号' },
{ id: 'site-templates', name: '站点模板', icon: SetUp, keywords: 'HDSKY OURBITS CHDBITS SSD Go 模板' },
    { id: 'tmdb-cache', name: 'TMDB缓存', icon: Coin, keywords: 'TMDB 缓存 TTL 查看 清理' },
{ id: 'search-cache', name: '搜索缓存', icon: Clock, keywords: '站点 搜索 缓存 TTL 查看 清理' },
    { id: 'downloads', name: '下载与传输', icon: Download, keywords: '下载器 做种 上传' },
  ] },
  { id: 'organize', name: '文件与入库', items: [
    { id: 'files', name: '文件管理', icon: FolderOpened, keywords: '本地 云盘 重命名 移动 回收' },
    { id: 'cloud-connections', name: '云盘连接', icon: Connection, keywords: '115 授权 CloudDrive2 空间 会员' },
    { id: 'cloudStorage', name: '云盘索引', icon: Cloudy, keywords: '扫描 识别 入库' },
    { id: 'strm', name: 'STRM 管理', icon: Link, keywords: '固定链接 302 播放' },
    { id: 'organize-history', name: '整理历史', icon: Clock, keywords: '命名 对账 记录' },
  ] },
  { id: 'operate', name: '系统运维', items: [
    { id: 'jobs', name: '任务队列', icon: Operation, keywords: 'Worker 重试 后台任务' },
    { id: 'notifications', name: '通知中心', icon: Bell, keywords: 'Telegram 通知配置' },
    { id: 'integrations', name: '服务连接', icon: Connection, keywords: '只读 连接状态' },
    { id: 'plugins', name: '插件扩展', icon: SetUp, keywords: 'Webhook 扩展' },
    { id: 'logs', name: '程序日志', icon: Files, keywords: 'debug info warn error 清理' },
    { id: 'data', name: '数据库浏览', icon: Coin, keywords: 'PostgreSQL Redis 只读' },
    { id: 'settings', name: '系统设置', icon: Setting, keywords: 'TMDB 缓存 命名模板 密码' },
  ] },
]
export const navigationItems = navigationGroups.flatMap(group => group.items.map(item => ({ ...item, group: group.name })))
export function searchNavigation(query: string) {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  return navigationItems.filter(item => terms.every(term => `${item.name} ${item.keywords} ${item.group}`.toLocaleLowerCase().includes(term)))
}
export function navigationLocation(path: string) {
  return navigationItems.find(item => path === `/${item.id}` || path.startsWith(`/${item.id}/`))
    ?? (path === '/emby' ? navigationItems.find(item => item.id === 'mediaLibrary') : undefined)
}
