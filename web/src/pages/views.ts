import { defineAsyncComponent, markRaw, type Component } from 'vue'
const view = (loader: () => Promise<any>) => markRaw(defineAsyncComponent(loader))
export const legacyViews: Record<string, Component> = {
  dashboard: view(() => import('../views/DashboardWorkspaceView.vue')),
  search: view(() => import('../views/SearchWorkspaceView.vue')),
  subscriptions: view(() => import('../views/SubscriptionWorkspaceView.vue')),
  downloads: view(() => import('../views/DownloadsWorkspaceView.vue')),
  library: view(() => import('../views/MediaWorkspaceView.vue')),
  organize: view(() => import('../views/OrganizeWorkspaceView.vue')),
  files: view(() => import('../views/FilesWorkspaceView.vue')),
  playback: view(() => import('../views/PlaybackWorkspaceView.vue')),
  connections: view(() => import('../views/ConnectionsWorkspaceView.vue')),
  sites: view(() => import('../views/SitesWorkspaceView.vue')),
  plugins: view(() => import('../views/PluginsWorkspaceView.vue')),
  ops: view(() => import('../views/OpsWorkspaceView.vue')),
  settings: view(() => import('../views/SettingsView.vue')),
  classification: view(() => import('../views/ClassificationView.vue')),
  api: view(() => import('../views/ExternalAPIView.vue')),
}
export const tabViews: Record<string, string[]> = {
  dashboard: ['dashboard'], search: ['search'], subscriptions: ['subscriptions'], downloads: ['downloads'], library: ['library'],
  organize: ['organize'], files: ['files'], playback: ['playback'], connections: ['connections'], sites: ['sites'], plugins: ['plugins'], ops: ['ops'], settings: ['settings','classification','api'],
}
