import { describe, it, expect } from 'vitest'
import { createRouter, createMemoryHistory } from 'vue-router'
import { routes } from './router/routes'
import { installPageRedirects } from './router/redirects'
import { primaryPages, settingsSections } from './navigation/pages'
function getRouter() { const router = createRouter({ history: createMemoryHistory(), routes }); installPageRedirects(router); return router }
const legacy = [
  ['/emby','/connections','emby'], ['/mediaLibrary','/library','browse'], ['/media-server','/playback','settings'],
  ['/media-users','/playback','users'], ['/player-access','/playback','players'], ['/cloudStorage','/organize','assets'],
  ['/organize-history','/organize','history'], ['/classification','/settings','classification'], ['/cloud-connections','/connections','storage'],
  ['/strm','/files','strm'], ['/strm-tools','/files','tools'], ['/maintenance','/ops','maintenance'], ['/jobs','/ops','jobs'],
  ['/logs','/ops','logs'], ['/data','/ops','data'], ['/notifications','/connections','notifications'], ['/integrations','/connections','overview'],
  ['/external-api','/settings','api'], ['/tmdb-cache','/ops','cache'], ['/search-cache','/search','history'], ['/site-templates','/sites','templates'],
] as const
describe('canonical workflow routing', () => {
  it.each(primaryPages)('keeps /$id reachable with the default tab', async page => { const r = getRouter(); await r.push(`/${page.id}`); expect(r.currentRoute.value.path).toBe(`/${page.id}`); expect(r.currentRoute.value.query.tab).toBe(page.tabs[0]?.id || undefined) })
  it.each(legacy)('%s redirects to %s?tab=%s and preserves every other query and hash', async (from, path, tab) => {
    const r = getRouter(); await r.push({ path: from, query: { tab: 'obsolete', q: '中文 & 原值', assetId: 'owned', tags: ['one','two'], flag: null }, hash: '#detail' })
    expect(r.currentRoute.value.path).toBe(path); expect(r.currentRoute.value.query).toMatchObject({ tab, q: '中文 & 原值', assetId: 'owned', tags: ['one','two'], flag: null }); expect(r.currentRoute.value.hash).toBe('#detail')
    expect(r.getRoutes().find(record => record.path === from)?.redirect).toBeTruthy(); expect(r.getRoutes().find(record => record.path === from)?.components).toBeUndefined()
  })
  it.each(settingsSections)('maps old settings tab $id to general + section, including same-path navigation', async section => { const r = getRouter(); await r.push('/settings?tab=general'); await r.push({ path: '/settings', query: { tab: section.id, keep: 'yes' } }); expect(r.currentRoute.value.query).toEqual({ tab: 'general', section: section.id, keep: 'yes' }) })
  it.each([
    ['emby','/connections','emby'], ['ptsite','/sites','accounts'], ['open115','/connections','storage'], ['clouddrive2','/connections','storage'],
    ['qbittorrent','/downloads','tasks'], ['tmdb','/connections','aux'], ['telegram','/connections','notifications'], ['local','/files','browser'], ['unknown','/connections','overview'],
  ])('keeps integrations/%s in its existing configuration owner', async (kind, path, tab) => { const r = getRouter(); await r.push(`/integrations/${kind}?keep=value`); expect(r.currentRoute.value.path).toBe(path); expect(r.currentRoute.value.query).toMatchObject({ tab, keep: 'value', connectionKind: kind }); if (kind === 'tmdb') expect(r.currentRoute.value.query.tab).toBe('aux') })
  it('does not rewrite standalone result and plugin detail URLs', async () => { const r = getRouter(); for (const path of ['/search/results/owned','/plugins/owned/pages/page']) { await r.push(path+'?keep=yes'); expect(r.currentRoute.value.path).toBe(path); expect(r.currentRoute.value.query).toEqual({ keep: 'yes' }) } })
})

it.each(["tmdb","proxy"])("moves old settings section %s to connections aux",async section=>{const r=getRouter();await r.push(`/settings?tab=general&section=${section}&keep=yes`);expect(r.currentRoute.value.path).toBe("/connections");expect(r.currentRoute.value.query).toMatchObject({tab:"aux",keep:"yes"});await r.push(`/settings?tab=${section}`);expect(r.currentRoute.value.path).toBe("/connections")})

it.each(['/cloudStorage','/organize?tab=library','/organize'])('organize legacy entry %s preserves asset links and canonical tabs',async from=>{const r=getRouter();await r.push(from+(from.includes('?')?'&':'?')+'assetId=owned&keep=yes');expect(r.currentRoute.value.query).toMatchObject({tab:'assets',assetId:'owned',keep:'yes'});await r.push('/cloudStorage');expect(r.currentRoute.value.query.tab).toBe('candidates')})
