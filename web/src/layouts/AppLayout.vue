<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Menu, Search, Bell, SwitchButton, ArrowRight } from '@element-plus/icons-vue'
import { useAuthStore } from '../stores/auth'
import {useUpdateStore} from '../stores/updates'
import {useRefresh} from '../composables/useRefresh'
import { useSystemStore } from '../stores/system'
import { navigationLocation } from '../navigation/items'
import { pageById } from '../navigation/pages'
import NavigationMenu from '../components/NavigationMenu.vue'
import QuickNavigator from '../components/QuickNavigator.vue'
import ThemeToggle from '../components/ThemeToggle.vue'
const route = useRoute(), router = useRouter(), auth = useAuthStore(), system = useSystemStore(), updates=useUpdateStore()
// Only the server's cached result is read here; release network checks run in the backend.
useRefresh(updates.refresh,{interval:5*60_000})

const menuOpen = ref(false), navigatorOpen = ref(false), main = ref<HTMLElement>()
const environmentLabel = computed(() => system.status?.environment === 'development' ? '开发环境' : system.status?.environment === 'production' ? '生产环境' : system.status ? '环境未知' : '读取状态中')
const versionLabel = computed(() => {const version=updates.build?.version||system.status?.version;return version?`v${version.replace(/^v/,'')}`:'读取版本中'})
const current = computed(() => navigationLocation(route.path))
const tabLabel = computed(() => current.value&&pageById(current.value.id).tabs.length>1 ? pageById(current.value.id).tabs.find(t => t.id === route.query.tab)?.label : '')
function shortcut(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return
  if (!navigatorOpen.value && [...document.querySelectorAll('.el-overlay')].some(node => node.getClientRects().length)) return
  event.preventDefault(); menuOpen.value = false; navigatorOpen.value = !navigatorOpen.value
}
let mobile: MediaQueryList | undefined
function resize() { if (!mobile?.matches) menuOpen.value = false }
watch(() => route.fullPath, () => { menuOpen.value = false; void nextTick(() => main.value?.focus({ preventScroll: true })) })
watch(() => [current.value?.name, tabLabel.value], () => { document.title = `${current.value?.name ?? '媒体管理'}${tabLabel.value && tabLabel.value !== current.value?.name ? ` · ${tabLabel.value}` : ''} · Media Tool` }, { immediate: true })
onMounted(() => { if (!system.status) void system.refresh(); window.addEventListener('keydown', shortcut); mobile = window.matchMedia('(max-width: 900px)'); mobile.addEventListener('change', resize) })
onUnmounted(() => { window.removeEventListener('keydown', shortcut); mobile?.removeEventListener('change', resize) })
async function logout() { await auth.logout(); await router.push('/login') }
</script>
<template>
  <div class="workbench-app">
    <a href="#main-content" class="skip-link">跳到主要内容</a>
    <aside class="workbench-sidebar" aria-label="主导航"><div class="workbench-sidebar-inner"><RouterLink to="/dashboard" class="workbench-brand"><span class="workbench-monogram" aria-hidden="true">M</span><span>Media Tool<small>媒体自动化工作台</small></span></RouterLink><NavigationMenu /><RouterLink class="workbench-version" to="/settings?tab=about"><strong>{{versionLabel}}</strong></RouterLink></div></aside>
    <el-drawer v-model="menuOpen" direction="ltr" size="min(300px, 88vw)" title="主导航" class="workbench-mobile-menu" destroy-on-close><NavigationMenu @navigate="menuOpen = false" /></el-drawer>
    <div class="workbench-main">
      <header class="workbench-topbar"><div class="workbench-location"><button class="workbench-menu-button" type="button" aria-label="打开导航" :aria-expanded="menuOpen" @click="menuOpen = true"><Menu aria-hidden="true" /></button><span v-if="current?.group">{{ current.group }}</span><ArrowRight v-if="current?.group" class="location-separator" aria-hidden="true" /><strong>{{ current?.name || '媒体管理' }}</strong><small v-if="tabLabel && tabLabel !== current?.name">/ {{ tabLabel }}</small></div><div class="workbench-actions"><button class="workbench-search" type="button" aria-label="快速前往" @click="navigatorOpen = true"><Search aria-hidden="true" /><span>快速前往</span><kbd>⌘ / Ctrl K</kbd></button><ThemeToggle /><RouterLink :to="updates.notice?'/settings?tab=about':'/connections?tab=notifications'" class="workbench-icon-link f8-bell" :aria-label="updates.notice?'发现新版本 '+updates.notice.tag:'查看通知'" :title="updates.notice?'发现新版本 '+updates.notice.tag+'，点击查看':'查看通知'"><Bell aria-hidden="true"/><span v-if="updates.notice" class="f8-dot" aria-hidden="true"/></RouterLink><span class="workbench-environment">{{ environmentLabel }}</span><button type="button" class="workbench-logout" aria-label="退出" @click="logout"><SwitchButton aria-hidden="true" /><span>退出</span></button></div></header>
      <main id="main-content" ref="main" class="workbench-content" tabindex="-1"><RouterView /></main>
      <footer class="workbench-footer">Media Tool · 资源到播放，一站整理</footer>
    </div>
    <QuickNavigator v-model="navigatorOpen" />
  </div>
</template>
<style scoped>
.workbench-app { display: flex; min-height: 100dvh; background: var(--mt-bg-canvas); color: var(--mt-text-primary); }
.workbench-sidebar { flex: 0 0 240px; align-self: stretch; background: var(--mt-bg-nav); border-right: 1px solid var(--mt-border-color); }
.workbench-sidebar-inner { position: sticky; top: 0; display: flex; flex-direction: column; height: 100dvh; max-height: 100dvh; overflow: hidden; }
.workbench-brand { display: flex; align-items: center; gap: 12px; padding: 24px 20px; font-weight: 650; border-bottom: 1px solid var(--mt-border-color); }.workbench-brand small { display: block; color: var(--mt-text-muted); font-size: 11px; font-weight: 400; margin-top: 3px; }.workbench-monogram { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 10px; color: var(--mt-color-primary); background: var(--mt-color-primary-light); font-size: 19px; }
.workbench-sidebar :deep(.navigation-content) { flex: 1 1 0; min-height: 0; overflow-y: auto; }.workbench-brand, .workbench-version { flex-shrink: 0; }.workbench-version { min-height:60px;box-sizing:border-box;flex-direction:column;text-decoration:none; padding: 16px 20px; border-top: 1px solid var(--mt-border-color); display: flex; justify-content: center; gap: 8px; color: var(--mt-text-muted); font-size: 12px; }.workbench-version strong { font-weight: 500; }
.workbench-main { display: flex; flex-direction: column; flex: 1; min-width: 0; }.workbench-topbar { position: sticky; top: 0; z-index: 20; display: flex; align-items: center; justify-content: space-between; gap: 16px; height: 68px; padding: 0 28px; border-bottom: 1px solid var(--mt-border-color); background: var(--mt-bg-surface); }
.workbench-location, .workbench-actions { display: flex; align-items: center; gap: 12px; min-width: 0; }.workbench-location { font-size: 13px; }.workbench-location > span, .workbench-location small { color: var(--mt-text-muted); }.location-separator { width: 14px; height: 14px; color: var(--mt-text-muted); }.workbench-location strong { white-space: nowrap; }
.workbench-topbar button, .workbench-icon-link { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-width: 44px; min-height: 44px; border: 0; background: none; color: var(--mt-text-muted); cursor: pointer; }.workbench-topbar svg { width: 19px; height: 19px; }.workbench-topbar .workbench-menu-button { display: none; }.workbench-topbar .workbench-search { padding: 0 12px; border: 1px solid var(--mt-border-color); border-radius: 8px; font-size: 12px; background: var(--mt-bg-canvas); }.workbench-search kbd { font-size: 11px; color: var(--mt-text-muted); }
.workbench-environment { padding: 3px 8px; border-radius: 12px; background: var(--mt-bg-canvas); font-size: 11px; color: var(--mt-text-muted); white-space: nowrap; }.workbench-content { flex: 1; width: 100%; max-width: 1440px; min-width: 0; padding: 28px; margin-inline: auto; outline: none; }.workbench-footer { padding: 16px 28px; color: var(--mt-text-muted); font-size: 12px; border-top: 1px solid var(--mt-border-color); }
@media (max-width: 1100px) { .workbench-search kbd, .workbench-location small { display: none; } }
@media (max-width: 900px) { .workbench-sidebar { display: none; }.workbench-topbar { padding: 0 12px; height: 60px; gap: 8px; }.workbench-actions, .workbench-location { gap: 4px; }.workbench-topbar .workbench-menu-button { display: inline-flex; }.workbench-location > span, .location-separator, .workbench-search span, .workbench-environment, .workbench-logout span { display: none; }.workbench-topbar .workbench-search { padding: 0; border: 0; background: none; }.workbench-content { padding: 16px; }.workbench-footer { padding: 16px; } }
:global(.workbench-mobile-menu .el-drawer__close-btn) { min-width: 44px; min-height: 44px; }
.f8-bell{position:relative}.f8-dot{position:absolute;right:5px;top:4px;width:7px;height:7px;border-radius:50%;background:var(--mt-color-danger,#dc2626)}
</style>
