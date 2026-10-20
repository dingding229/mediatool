import type { RouteLocationNormalized, RouteLocationRaw, Router } from 'vue-router'
import { primaryPages, settingsSections } from '../navigation/pages'
export const legacyDestinations: Record<string, { path: string; tab: string; query?: Record<string, string> }> = {
  '/emby': { path: '/library', tab: 'browse', query: { connectionKind: 'emby' } },
  '/mediaLibrary': { path: '/library', tab: 'browse' },
  '/media-server': { path: '/playback', tab: 'settings' },
  '/media-users': { path: '/playback', tab: 'users' },
  '/player-access': { path: '/playback', tab: 'players' },
  '/cloudStorage': { path: '/organize', tab: 'library' },
  '/organize-history': { path: '/organize', tab: 'history' },
  '/classification': { path: '/settings', tab: 'classification' },
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
  return { path: target.path, query: { ...target.query, ...to.query, tab: target.tab }, hash: to.hash }
}
export function normalizePageLocation(to: Pick<RouteLocationNormalized, 'path' | 'query' | 'hash'>): RouteLocationRaw | undefined {
  const page = primaryPages.find(page => to.path === `/${page.id}`)
  if (!page?.tabs[0]?.id) return
  const query = { ...to.query }, tab = typeof query.tab === 'string' ? query.tab : ''
  if (page.id === 'settings' && settingsSections.some(s => s.id === tab)) { query.section = tab; query.tab = 'general' }
  else if (!page.tabs.some(t => t.id === tab)) query.tab = page.tabs[0].id
  if (page.id === 'settings' && query.tab === 'general' && query.connectionKind === 'tmdb' && !query.section) query.section = 'tmdb'
  if (JSON.stringify(query) !== JSON.stringify(to.query)) return { path: to.path, query, hash: to.hash, replace: true }
}
// Same-path settings query migrations must also run on query-only navigation, not only beforeEnter.
export function installPageRedirects(router: Router) { router.beforeEach(to => normalizePageLocation(to)) }
