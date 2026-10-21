import type { Component } from 'vue'
import { Monitor, Search, Star, Download, Film, FolderChecked, FolderOpened, VideoPlay, Connection, Tickets, MagicStick, Operation, Setting } from '@element-plus/icons-vue'
import { primaryPages } from './pages'
export interface NavigationItem { id: string; name: string; icon: Component; keywords: string; path: string; group: string }
export interface NavigationGroup { id: string; name: string; items: NavigationItem[] }
const icons = [Monitor, Search, Star, Download, Film, FolderChecked, FolderOpened, VideoPlay, Connection, Tickets, MagicStick, Operation, Setting]
export const navigationItems: NavigationItem[] = primaryPages.map((page, index) => ({ id: page.id, name: page.name, group: page.group, icon: icons[index]!, path: `/${page.id}`, keywords: page.description }))
export const navigationGroups: NavigationGroup[] = [{ id: 'home', name: '', items: navigationItems.filter(p => !p.group) }, ...['获取', '媒体', '系统'].map((name, index) => ({ id: ['acquire','media','system'][index]!, name, items: navigationItems.filter(p => p.group === name) }))]
export interface QuickSearchMatch extends NavigationItem { pageId: string; to: { path: string; query?: Record<string, string> } }
const primaryDestinations: QuickSearchMatch[] = navigationItems.map(item => ({ ...item, pageId: item.id, keywords: item.name, to: { path: item.path } }))
const tabDestinations: QuickSearchMatch[] = primaryPages.flatMap(page => page.tabs.filter(tab => tab.id).map(tab => ({ ...navigationItems.find(item => item.id === page.id)!, id: `${page.id}:${tab.id}`, pageId: page.id, name: `${page.name} · ${tab.label}`, keywords: tab.keywords ?? '', to: { path: `/${page.id}`, query: { tab: tab.id } } })))
export function searchNavigation(query: string): QuickSearchMatch[] {
  const terms = query.trim().toLocaleLowerCase().split(/\s+/).filter(Boolean)
  if (!terms.length) return primaryDestinations
  return [...primaryDestinations, ...tabDestinations].filter(item => terms.every(term => `${item.name} ${item.group} ${item.keywords}`.toLocaleLowerCase().includes(term)))
}
export function navigationLocation(path: string) { return navigationItems.find(item => path === item.path || path.startsWith(item.path + '/')) }
