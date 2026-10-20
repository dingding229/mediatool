import { defineAsyncComponent, markRaw, type Component } from 'vue'
const view = (loader: () => Promise<any>) => markRaw(defineAsyncComponent(loader))
export const legacyViews: Record<string, Component> = {
  dashboard: view(() => import('../views/DashboardWorkspaceView.vue')),
  search: view(() => import('../views/SearchWorkspaceView.vue')),
  subscriptions: view(() => import('../views/SubscriptionWorkspaceView.vue')),
  downloads: view(() => import('../views/DownloadsWorkspaceView.vue')),
  library: view(() => import('../views/MediaLibraryView.vue')),
  organize: view(() => import('../views/LibraryView.vue')),
  organizeHistory: view(() => import('../views/OrganizeHistoryView.vue')),
  files: view(() => import('../views/FilesView.vue')),
  strm: view(() => import('../views/StrmView.vue')),
  strmTools: view(() => import('../views/StrmToolsView.vue')),
  playback: view(() => import('../views/MediaServerView.vue')),
  users: view(() => import('../views/MediaUsersView.vue')),
  players: view(() => import('../views/PlayerAccessView.vue')),
  connections: view(() => import('../views/ConnectionsWorkspaceView.vue')),
  sites: view(() => import('../views/SitesView.vue')),
  templates: view(() => import('../views/SiteTemplatesView.vue')),
  plugins: view(() => import('../views/PluginsView.vue')),
  jobs: view(() => import('../views/JobsView.vue')),
  maintenance: view(() => import('../views/MaintenanceView.vue')),
  logs: view(() => import('../views/LogsView.vue')),
  data: view(() => import('../views/DataView.vue')),
  cache: view(() => import('./OpsCacheBridge.vue')),
  settings: view(() => import('../views/SettingsView.vue')),
  classification: view(() => import('../views/ClassificationView.vue')),
  api: view(() => import('../views/ExternalAPIView.vue')),
}
export const tabViews: Record<string, string[]> = {
  dashboard: ['dashboard'], search: ['search'], subscriptions: ['subscriptions'], downloads: ['downloads'], library: ['library'],
  organize: ['organize','organizeHistory'], files: ['files','strm','strmTools'], playback: ['playback','users','players'], connections: ['connections'], sites: ['sites','templates'], plugins: ['plugins'], ops: ['jobs','maintenance','logs','data','cache'], settings: ['settings','classification','api'],
}
