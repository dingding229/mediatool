import type { RouteLocationNormalized, RouteLocationRaw, Router } from 'vue-router'
import { primaryPages } from '../navigation/pages'
export const legacyDestinations: Record<string, { path: string; tab: string; query?: Record<string, string> }> = {
  '/emby': { path: '/connections', tab: 'emby' },
  '/mediaLibrary': { path: '/library', tab: 'browse' },
  '/media-server': { path: '/playback', tab: 'settings' },
  '/media-users': { path: '/playback', tab: 'users' },
  '/player-access': { path: '/playback', tab: 'players' },
  '/cloudStorage': { path: '/organize', tab: 'candidates' },
  '/organize-history': { path: '/organize', tab: 'history' },
  '/classification': { path: '/settings', tab: 'naming' },
  '/cloud-connections': { path: '/connections', tab: 'storage' },
  '/strm': { path: '/files', tab: 'strm' },
  '/strm-tools': { path: '/files', tab: 'tools' },
  '/maintenance': { path: '/ops', tab: 'maintenance' },
  '/jobs': { path: '/ops', tab: 'jobs' },
  '/logs': { path: '/ops', tab: 'logs' },
  '/data': { path: '/ops', tab: 'data' },
  '/notifications': { path: '/connections', tab: 'notifications' },
  '/integrations': { path: '/connections', tab: 'overview' },
  '/external-api': { path: '/settings', tab: 'api' },
  '/tmdb-cache': { path: '/ops', tab: 'cache' },
  '/search-cache': { path: '/search', tab: 'history' },
  '/site-templates': { path: '/sites', tab: 'templates' },
}
export function legacyLocation(path: string, to: Pick<RouteLocationNormalized, 'query' | 'hash'>): RouteLocationRaw {
  const target = legacyDestinations[path]!
  return { path: target.path, query: { ...target.query, ...to.query, tab: path === '/cloudStorage' && to.query.assetId ? 'assets' : target.tab }, hash: to.hash }
}
export function normalizePageLocation(to: Pick<RouteLocationNormalized, 'path' | 'query' | 'hash'>): RouteLocationRaw | undefined {
  if(to.path==='/library'&&to.query.source==='plugins')return {path:'/search',query:{...to.query,tab:'discover',kind:to.query.kind==='recommend.source'?'recommend.source':'discover.source',source:undefined},hash:to.hash,replace:true};
  if ((to.path === '/settings' && (['tmdb','proxy'].includes(String(to.query.section)) || ['tmdb','proxy'].includes(String(to.query.tab)))) || (['/library','/settings'].includes(to.path) && to.query.connectionKind === 'emby')) return { path: '/connections', query: { ...to.query, tab: to.query.connectionKind === 'emby' ? 'emby' : 'aux' }, hash: to.hash }
  if (to.path === '/organize' && (to.query.tab === 'library' || !to.query.tab && to.query.assetId)) return {path:'/organize',query:{...to.query,tab:to.query.assetId?'assets':'candidates'},hash:to.hash,replace:true}
  const page = primaryPages.find(page => to.path === `/${page.id}`)
  if (!page?.tabs[0]?.id) return
  const query = { ...to.query }, tab = typeof query.tab === 'string' ? query.tab : ''
  if (page.id === 'settings') {
    const old: Record<string,string> = { access:'general', library:'naming', classification:'naming', automation:'automation', account:'security', security:'security', mobile:'mobile', api:'api', general:'general', naming:'naming' };
    const section = typeof query.section === 'string' ? query.section : '';
    query.tab = old[(!tab || tab === 'general') && old[section] ? section : tab] || 'general';
    if (old[section]) delete query.section;
    if (tab === 'cache' || ((!tab || tab === 'general') && section === 'cache')) return { path:'/ops',query:{...query,tab:'cache'},hash:to.hash,replace:true };
  } else if (!page.tabs.some(t => t.id === tab)) query.tab = page.tabs[0].id
  if (page.id === 'settings' && query.connectionKind === 'tmdb') return { path: '/connections', query: { ...query, tab: 'aux' }, hash: to.hash }
  if (JSON.stringify(query) !== JSON.stringify(to.query)) return { path: to.path, query, hash: to.hash, replace: true }
}
// Same-path settings query migrations must also run on query-only navigation, not only beforeEnter.
export function installPageRedirects(router: Router) { router.beforeEach(to => normalizePageLocation(to)) }
