import { describe, expect, it } from 'vitest'
import { navigationGroups, navigationItems, navigationLocation, searchNavigation } from './items'
describe('workflow navigation', () => {
  it('has exactly one entry for every feature', () => {
    expect(navigationGroups.map(group => group.name)).toEqual(['媒体浏览','资源获取','文件与入库','系统运维'])
    expect(navigationItems).toHaveLength(25)
    expect(new Set(navigationItems.map(item => item.id)).size).toBe(25)
  })
  it('searches names, domains and keywords case-insensitively', () => {
    expect(searchNavigation('redis').map(item => item.id)).toEqual(['data'])
    expect(searchNavigation('115 授权').map(item => item.id)).toEqual(['cloud-connections'])
    expect(searchNavigation('TMDB')).toEqual(expect.arrayContaining([expect.objectContaining({ id: 'search' }),expect.objectContaining({ id: 'settings' })]))
    expect(searchNavigation('   ')).toHaveLength(25)
    expect(searchNavigation('no-such-feature')).toHaveLength(0)
  })
  it('retains legacy/deep-link locations without false prefix matches', () => {
    expect(navigationLocation('/integrations/emby')?.id).toBe('integrations')
    expect(navigationLocation('/emby')?.id).toBe('mediaLibrary')
    expect(navigationLocation('/mediaLibrary')?.name).toBe('媒体库')
    expect(navigationLocation('/search-unrelated')).toBeUndefined()
  })
})
