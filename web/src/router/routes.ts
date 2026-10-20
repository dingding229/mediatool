import type { RouteRecordRaw } from 'vue-router'
import { connectionOwner, connectionTab } from '../integrations/owners'
import { primaryPages } from '../navigation/pages'
import { legacyDestinations, legacyLocation } from './redirects'
import Layout from '../layouts/AppLayout.vue'
import WorkflowPage from '../pages/WorkflowPage.vue'
export const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
  { path: '/reset-password', name: 'resetPassword', component: () => import('../views/ForcePasswordView.vue') },
  { path: '/', component: Layout, children: [
    { path: '', redirect: '/dashboard' },
    ...primaryPages.map(page => ({ path: page.id, name: page.id, component: WorkflowPage, props: { pageId: page.id } })),
    { path: 'search/results/:id', name: 'searchResults', component: () => import('../views/ResourceResultsView.vue') },
    { path: 'plugins/:id/pages/:key', name: 'pluginPage', component: () => import('../views/PluginPageView.vue') },
    ...Object.keys(legacyDestinations).map(path => ({ path: path.slice(1), redirect: (to: Parameters<typeof legacyLocation>[1]) => legacyLocation(path, to) })),
    { path: 'integrations/:kind', name: 'integrationService', redirect: to => ({ path: connectionOwner(String(to.params.kind)), query: { ...to.query, connectionKind: String(to.params.kind), ...(connectionOwner(String(to.params.kind)) === '/connections' ? { tab: connectionTab(String(to.params.kind)) } : {}) }, hash: to.hash }) },
  ] },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' },
]
