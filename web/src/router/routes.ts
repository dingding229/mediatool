import type { RouteRecordRaw } from 'vue-router'
import { connectionOwner } from '../integrations/owners'
import Layout from '../layouts/AppLayout.vue'

export const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
  { path: '/reset-password', name: 'resetPassword', component: () => import('../views/ForcePasswordView.vue') },
  {
    path: '/',
    component: Layout,
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', name: 'dashboard', component: () => import('../views/DashboardView.vue') },

      // 13 个主路由
      { path: 'search', name: 'search', component: () => import('../views/SearchView.vue') },
      { path: 'subscriptions', name: 'subscriptions', component: () => import('../views/SubscriptionsView.vue') },
      { path: 'downloads', name: 'downloads', component: () => import('../views/DownloadsView.vue') },
      { path: 'library', name: 'library', component: () => import('../views/MediaLibraryView.vue') },
      { path: 'organize', name: 'organize', component: () => import('../views/LibraryView.vue') },
      { path: 'files', name: 'files', component: () => import('../views/FilesView.vue') },
      { path: 'playback', name: 'playback', component: () => import('../views/MediaServerView.vue') },
      { path: 'connections', name: 'connections', component: () => import('../views/CloudConnectionsView.vue') },
      { path: 'sites', name: 'sites', component: () => import('../views/SitesView.vue') },
      { path: 'plugins', name: 'plugins', component: () => import('../views/PluginsView.vue') },
      { path: 'ops', name: 'ops', component: () => import('../views/JobsView.vue') },
      { path: 'settings', name: 'settings', component: () => import('../views/SettingsView.vue') },

      // 特殊子页面
      { path: 'search/results/:id', name: 'searchResults', component: () => import('../views/SearchResultsView.vue') },
      { path: 'plugins/:id/pages/:key', name: 'pluginPage', component: () => import('../views/PluginPageView.vue') },

      // 原有全部 31 个导航与旧页面保持直接组件挂载，确保原有页面结构、tab 及测试选择器 100% 可达
      { path: 'emby', name: 'emby', component: () => import('../views/EmbyView.vue') },
      { path: 'mediaLibrary', name: 'mediaLibrary', component: () => import('../views/MediaLibraryView.vue') },
      { path: 'media-server', name: 'mediaServer', component: () => import('../views/MediaServerView.vue') },
      { path: 'media-users', name: 'mediaUsers', component: () => import('../views/MediaUsersView.vue') },
      { path: 'player-access', name: 'playerAccess', component: () => import('../views/PlayerAccessView.vue') },
      { path: 'cloudStorage', name: 'cloudStorage', component: () => import('../views/LibraryView.vue') },
      { path: 'organize-history', name: 'organizeHistory', component: () => import('../views/OrganizeHistoryView.vue') },
      { path: 'classification', name: 'classification', component: () => import('../views/ClassificationView.vue') },
      { path: 'cloud-connections', name: 'cloudConnections', component: () => import('../views/CloudConnectionsView.vue') },
      { path: 'strm', name: 'strm', component: () => import('../views/StrmView.vue') },
      { path: 'strm-tools', name: 'strmTools', component: () => import('../views/StrmToolsView.vue') },
      { path: 'maintenance', name: 'maintenance', component: () => import('../views/MaintenanceView.vue') },
      { path: 'jobs', name: 'jobs', component: () => import('../views/JobsView.vue') },
      { path: 'logs', name: 'logs', component: () => import('../views/LogsView.vue') },
      { path: 'data', name: 'data', component: () => import('../views/DataView.vue') },
      { path: 'notifications', name: 'notifications', component: () => import('../views/NotificationsView.vue') },
      { path: 'integrations', name: 'integrations', component: () => import('../views/IntegrationsView.vue') },
      {
        path: 'integrations/:kind',
        name: 'integrationService',
        redirect: to => ({
          path: connectionOwner(String(to.params.kind)),
          query: { ...to.query, connectionKind: String(to.params.kind) }
        })
      },
      { path: 'external-api', name: 'external-api', component: () => import('../views/ExternalAPIView.vue') },
      { path: 'tmdb-cache', name: 'tmdbCache', component: () => import('../views/CacheView.vue'), props: { kind: 'tmdb' } },
      { path: 'search-cache', name: 'searchCache', component: () => import('../views/CacheView.vue'), props: { kind: 'search' } },
      { path: 'site-templates', name: 'siteTemplates', component: () => import('../views/SiteTemplatesView.vue') }
    ]
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' }
]
