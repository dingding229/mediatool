import { createRouter,createWebHistory } from 'vue-router'
import { useAuthStore } from './stores/auth'
import {connectionOwner} from './integrations/owners'
import Layout from './layouts/AppLayout.vue'
const moduleRoutes:never[]=[]
const router=createRouter({history:createWebHistory(),routes:[
 {path:'/login',name:'login',component:()=>import('./views/LoginView.vue'),meta:{public:true}},
 {path:'/reset-password',name:'resetPassword',component:()=>import('./views/ForcePasswordView.vue')},
 {path:'/',component:Layout,children:[
  {path:'',redirect:'/dashboard'},
  {path:'dashboard',name:'dashboard',component:()=>import('./views/DashboardView.vue')},
  ...moduleRoutes,
  {path:'emby',name:'emby',component:()=>import('./views/EmbyView.vue')},
  {path:'subscriptions',name:'subscriptions',component:()=>import('./views/SubscriptionsView.vue')},
  {path:'sites',name:'sites',component:()=>import('./views/SitesView.vue')},
{path:'site-templates',name:'siteTemplates',component:()=>import('./views/SiteTemplatesView.vue')},
  {path:'tmdb-cache',name:'tmdbCache',component:()=>import('./views/CacheView.vue'),props:{kind:'tmdb'}},
 {path:'search-cache',name:'searchCache',component:()=>import('./views/CacheView.vue'),props:{kind:'search'}},
 {path:'search/results/:id',name:'searchResults',component:()=>import('./views/SearchResultsView.vue')},
  {path:'search',name:'search',component:()=>import('./views/SearchView.vue')},
  {path:'downloads',name:'downloads',component:()=>import('./views/DownloadsView.vue')},
  {path:'files',name:'files',component:()=>import('./views/FilesView.vue')},
  {path:'mediaLibrary',name:'mediaLibrary',component:()=>import('./views/MediaLibraryView.vue')},
  {path:'cloudStorage',name:'cloudStorage',component:()=>import('./views/LibraryView.vue')},
  {path:'notifications',name:'notifications',component:()=>import('./views/NotificationsView.vue')},
  {path:'strm',name:'strm',component:()=>import('./views/StrmView.vue')},
  {path:'jobs',name:'jobs',component:()=>import('./views/JobsView.vue')},
  {path:'integrations',name:'integrations',component:()=>import('./views/IntegrationsView.vue')},
  {path:'integrations/:kind',name:'integrationService',redirect:to=>({path:connectionOwner(String(to.params.kind)),query:{connectionKind:String(to.params.kind)}})},
  {path:'logs',name:'logs',component:()=>import('./views/LogsView.vue')},
 {path:'data',name:'data',component:()=>import('./views/DataView.vue')},
 {path:'plugins/:id/:page',name:'pluginPage',component:()=>import('./views/PluginPageView.vue')},
 {path:'plugins',name:'plugins',component:()=>import('./views/PluginsView.vue')},
 {path:'organize-history',name:'organizeHistory',component:()=>import('./views/OrganizeHistoryView.vue')},
 {path:'settings',name:'settings',component:()=>import('./views/SettingsView.vue')},
 ]},
 {path:'/:pathMatch(.*)*',redirect:'/dashboard'},
]})
router.beforeEach(async(to)=>{
 const auth=useAuthStore()
 try{await auth.ensure()}catch{return to.name==='login'?true:{name:'login'}}
 if(auth.authenticated&&auth.account?.mustChangePassword&&to.name!=='resetPassword')return{name:'resetPassword'}
 if(to.name==='resetPassword'&&auth.authenticated&&!auth.account?.mustChangePassword)return{name:'dashboard'}
 if(!to.meta.public&&!auth.authenticated)return{name:'login'}
 if(to.name==='login'&&auth.authenticated)return{name:'dashboard'}
 return true
})
export default router
