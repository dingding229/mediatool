<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Search, Menu as MenuIcon, SwitchButton, Bell, ArrowRight } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import { useSystemStore } from '../stores/system'
import { navigationLocation } from '../navigation/items'
import NavigationMenu from '../components/NavigationMenu.vue'
import QuickNavigator from '../components/QuickNavigator.vue'
const route = useRoute(), router = useRouter(), auth = useAuthStore(), system = useSystemStore()
const menuOpen = ref(false), navigatorOpen = ref(false), main = ref<HTMLElement>()
const expandedGroups = ref<string[]>([])
const current = computed(() => navigationLocation(route.path))
let mobile: MediaQueryList | undefined
function onResize() { if (!mobile?.matches) menuOpen.value = false }
function shortcut(event: KeyboardEvent) {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k' && !menuOpen.value) {
    if (Array.from(document.querySelectorAll('.el-overlay')).some(node => node.getClientRects().length > 0) && !navigatorOpen.value) return
    event.preventDefault(); navigatorOpen.value = !navigatorOpen.value
  }
}
watch(() => route.path, () => { menuOpen.value = false; main.value?.focus({ preventScroll: true }) })
watch(() => current.value?.name, name => { document.title = `${name ?? '媒体管理'} · Media Tool` }, { immediate: true })
onMounted(() => { if (route.name !== 'dashboard' && !system.status) system.refresh(); mobile = window.matchMedia('(max-width: 900px)'); mobile.addEventListener('change', onResize); window.addEventListener('keydown', shortcut) })
onUnmounted(() => { mobile?.removeEventListener('change', onResize); window.removeEventListener('keydown', shortcut) })
async function logout() { await auth.logout(); await router.push('/login') }
</script>
<template>
  <div class="app-shell">
    <a href="#main-content" class="skip-link">跳到主要内容</a>
    <aside class="sidebar" aria-label="主导航">
      <RouterLink to="/dashboard" class="brand"><span class="brand-mark" aria-hidden="true"><span></span><span></span><span></span></span><span>Media Tool<small>你的媒体自动化工作台</small></span></RouterLink>
      <NavigationMenu v-model:expanded="expandedGroups" />
      <div class="sidebar-note"><span class="sidebar-system-dot" aria-hidden="true"></span><span>媒体自动化<span class="build-label">{{ system.status?.version ?? '读取版本中' }}</span></span></div>
    </aside>
    <el-drawer v-model="menuOpen" direction="ltr" size="280px" title="主导航" class="navigation-drawer" :close-on-click-modal="true">
      <NavigationMenu v-model:expanded="expandedGroups" @navigate="menuOpen=false" />
    </el-drawer>
    <div class="main-shell">
      <header class="topbar">
        <div class="topbar-heading"><button class="mobile-menu icon-button" aria-label="打开导航" :aria-expanded="menuOpen" @click="menuOpen=true"><MenuIcon aria-hidden="true" /></button><span>{{ current?.group ?? '工作台' }}</span><ArrowRight class="topbar-divider" aria-hidden="true" /><strong>{{ current?.name ?? '媒体管理' }}</strong></div>
        <div class="topbar-actions">
          <button class="quick-trigger" aria-label="快速前往" @click="navigatorOpen=true"><Search aria-hidden="true" /><span>快速前往</span><kbd>⌘ / Ctrl K</kbd></button>
          <RouterLink to="/notifications" class="icon-button notification-link" aria-label="查看通知"><Bell aria-hidden="true" /></RouterLink>
          <span class="environment">{{ system.status?.environment === 'production' ? '生产环境' : system.status ? '开发环境' : '读取状态中' }}</span>
          <button class="text-button logout-button" @click="logout"><SwitchButton aria-hidden="true" /><span>退出</span></button>
        </div>
      </header>
      <main id="main-content" ref="main" tabindex="-1" class="page-content"><RouterView /></main>
      <footer class="app-footer"><span>Media Tool · 资源到播放，一站整理</span><span>仅展示真实服务数据</span></footer>
    </div>
    <QuickNavigator v-model="navigatorOpen" />
  </div>
</template>
