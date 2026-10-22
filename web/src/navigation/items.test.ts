import { describe, expect, it } from 'vitest'
import { navigationGroups, navigationItems, navigationLocation, searchNavigation } from './items'
describe('thirteen workflow destinations', () => {
  it('has standalone home and exactly 3/4/5 entries in the other groups', () => { expect(navigationGroups.map(g => [g.name,g.items.length])).toEqual([['',1],['获取',3],['媒体',4],['系统',5]]); expect(navigationItems.map(i => i.name)).toEqual(['首页','搜索','订阅','下载','媒体库','入库整理','文件','播放服务','连接','站点','插件','运维','设置']); expect(new Set(navigationItems.map(i => i.id)).size).toBe(13) })
  it('keeps a distinct icon per primary destination', () => expect(new Set(navigationItems.map(i => i.icon)).size).toBe(13))
  it('empty quick search offers only the thirteen pages', () => expect(searchNavigation(' ')).toHaveLength(13))
  it('logs and runtime health point to their actual operations tab, not the jobs default', () => { expect(searchNavigation('日志').map(i => i.to)).toEqual([{ path: '/ops', query: { tab: 'logs' } }]); expect(searchNavigation('Redis').map(i => i.to)).toEqual([{ path: '/ops', query: { tab: 'attention' } }]); expect(searchNavigation('运维 日志')[0]?.to).toEqual({ path: '/ops', query: { tab: 'logs' } }) })
  it('searches settings sections and authorization directly', () => { expect(searchNavigation('命名模板')[0]?.to).toEqual({ path: '/settings', query: { tab: 'naming' } }); expect(searchNavigation('115 授权')[0]?.to).toEqual({ path: '/connections', query: { tab: 'storage' } }); expect(searchNavigation('no-such-feature')).toEqual([]) })
  it('matches nested routes without prefix false positives', () => { expect(navigationLocation('/search/results/owned')?.id).toBe('search'); expect(navigationLocation('/plugins/owned/pages/page')?.id).toBe('plugins'); expect(navigationLocation('/search-unrelated')).toBeUndefined() })
})
