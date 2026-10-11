<script setup lang="ts">
import { computed,onMounted,ref } from 'vue'
import { useRoute,useRouter } from 'vue-router'
import { Monitor,Connection,Search,Collection,Download,FolderOpened,Link,VideoPlay,Bell,Operation,Setting,Close,Menu as MenuIcon,SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useSystemStore } from '../stores/system'
const route=useRoute(),router=useRouter(),auth=useAuthStore(),system=useSystemStore(),menuOpen=ref(false)
const nav=[{id:'dashboard',name:'总览',icon:Monitor},{id:'sites',name:'PT 站点',icon:Connection},{id:'search',name:'资源搜索',icon:Search},{id:'subscriptions',name:'资源订阅',icon:Collection},{id:'downloads',name:'下载与传输',icon:Download},{id:'files',name:'文件管理',icon:FolderOpened},{id:'mediaLibrary',name:'媒体库',icon:Collection},{id:'cloudStorage',name:'云盘索引',icon:FolderOpened},{id:'strm',name:'STRM 管理',icon:Link},{id:'emby',name:'Emby 网关',icon:VideoPlay},{id:'notifications',name:'通知中心',icon:Bell},{id:'jobs',name:'任务队列',icon:Operation},{id:'integrations',name:'服务连接',icon:Connection},{id:'settings',name:'系统设置',icon:Setting}]
const currentTitle=computed(()=>nav.find(item=>item.id===route.name)?.name??(route.path.startsWith('/integrations')?'服务连接':'媒体管理'))
onMounted(()=>system.refresh())
async function logout(){await auth.logout();await router.push('/login')}
</script>
<template>
 <div class="app-shell">
  <div v-if="menuOpen" class="menu-scrim" @click="menuOpen=false"></div>
  <aside class="sidebar" :class="{'is-open':menuOpen}" aria-label="主导航">
   <RouterLink to="/dashboard" class="brand" @click="menuOpen=false"><span class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></span><span>Media Tool<small>媒体自动化工作台</small></span></RouterLink>
   <button class="mobile-close icon-button" aria-label="关闭导航" @click="menuOpen=false"><Close /></button>
   <nav><RouterLink v-for="item in nav" :key="item.id" :to="`/${item.id}`" class="nav-item" :class="{'router-link-active':item.id==='integrations'&&route.path.startsWith('/integrations')}" @click="menuOpen=false"><component :is="item.icon" aria-hidden="true"/><span>{{item.name}}</span></RouterLink></nav>
   <div class="sidebar-note"><span class="note-line"></span><strong>先建立可靠链路</strong><p>订阅 → 下载 → 云盘 → 媒体库</p><span class="build-label">{{system.status?.version??'工程框架'}}</span></div>
  </aside>
  <div class="main-shell">
   <header class="topbar"><div class="topbar-heading"><button class="mobile-menu icon-button" aria-label="打开导航" @click="menuOpen=true"><MenuIcon/></button><span>工作台</span><span class="topbar-divider">/</span><strong>{{currentTitle}}</strong></div><div class="topbar-actions"><span class="environment">{{system.status?.environment==='production'?'生产环境':'开发环境'}}</span><button class="text-button logout-button" @click="logout"><SwitchButton aria-hidden="true"/><span>退出</span></button></div></header>
   <main id="main-content" class="page-content"><RouterView /></main>
   <footer class="app-footer"><span>数据来自实际服务状态，不展示模拟媒体记录。</span><span>Go · Vue · PostgreSQL · Redis</span></footer>
  </div>
 </div>
</template>
<style scoped>
.sidebar nav{min-height:0;flex:1;overflow-y:auto;overscroll-behavior:contain;scrollbar-width:thin}
.sidebar .brand,.sidebar-note{flex-shrink:0}
.sidebar-note{margin-top:18px;padding-top:10px}
</style>
