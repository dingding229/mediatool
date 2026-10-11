import { createRouter,createWebHistory } from 'vue-router'
import { useAuthStore } from './stores/auth'
import Layout from './layouts/AppLayout.vue'
const moduleRoutes:never[]=[]
const router=createRouter({history:createWebHistory(),routes:[
 {path:'/login',name:'login',component:()=>import('./views/LoginView.vue'),meta:{public:true}},
 {path:'/',component:Layout,children:[
  {path:'',redirect:'/dashboard'},
  {path:'dashboard',name:'dashboard',component:()=>import('./views/DashboardView.vue')},
  ...moduleRoutes,
  {path:'emby',name:'emby',component:()=>import('./views/EmbyView.vue')},
  {path:'subscriptions',name:'subscriptions',component:()=>import('./views/SubscriptionsView.vue')},
  {path:'sites',name:'sites',component:()=>import('./views/SitesView.vue')},
  {path:'search',name:'search',component:()=>import('./views/SearchView.vue')},
  {path:'downloads',name:'downloads',component:()=>import('./views/DownloadsView.vue')},
  {path:'files',name:'files',component:()=>import('./views/FilesView.vue')},
  {path:'mediaLibrary',name:'mediaLibrary',component:()=>import('./views/MediaLibraryView.vue')},
  {path:'cloudStorage',name:'cloudStorage',component:()=>import('./views/LibraryView.vue')},
  {path:'notifications',name:'notifications',component:()=>import('./views/NotificationsView.vue')},
  {path:'strm',name:'strm',component:()=>import('./views/StrmView.vue')},
  {path:'jobs',name:'jobs',component:()=>import('./views/JobsView.vue')},
  {path:'integrations',name:'integrations',component:()=>import('./views/IntegrationsView.vue')},
  {path:'integrations/:kind',name:'integrationService',component:()=>import('./views/IntegrationsView.vue')},
  {path:'settings',name:'settings',component:()=>import('./views/SettingsView.vue')},
 ]},
 {path:'/:pathMatch(.*)*',redirect:'/dashboard'},
]})
router.beforeEach(async(to)=>{
 const auth=useAuthStore()
 try{await auth.ensure()}catch{return to.name==='login'?true:{name:'login'}}
 if(!to.meta.public&&!auth.authenticated)return{name:'login'}
 if(to.name==='login'&&auth.authenticated)return{name:'dashboard'}
 return true
})
export default router
