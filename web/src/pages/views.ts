import { defineAsyncComponent, markRaw, type Component } from 'vue';
const view = (loader: () => Promise<any>) => markRaw(defineAsyncComponent(loader));
export const workspaceViews: Record<string, Component> = {
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
  settings: view(() => import('../views/SettingsWorkspaceView.vue')),
};
