<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { uiSurfaces } from '../api/plugins'
import inventory from '../../../internal/modules/plugins/v2/library.json'
import { navigationGroups, navigationLocation } from '../navigation/items'
const emit = defineEmits<{ navigate: [] }>()
const route = useRoute(), current = computed(() => navigationLocation(route.path))
interface PluginLink { id: string; name: string; pages: { page: string; title: string }[] }
const plugins = ref<PluginLink[]>([])
let disposed = false, timer: ReturnType<typeof setInterval> | undefined
async function refreshPlugins() {
  try {
    const surfaces = await uiSurfaces()
    if (disposed) return
    plugins.value = surfaces.flatMap(surface => {
      const manifest = inventory.manifests.find(m => m.id === surface.pluginId)
      return manifest?.permissions.some(p => p.scope === 'ui.sidebar') && manifest.ui?.sidebar?.length ? [{ id: surface.instanceId, name: manifest.name, pages: manifest.ui.sidebar }] : []
    })
  } catch { if (!disposed) plugins.value = [] }
}
onMounted(() => { void refreshPlugins(); timer = setInterval(refreshPlugins, 60000); window.addEventListener('plugins:changed', refreshPlugins) })
onUnmounted(() => { disposed = true; if (timer) clearInterval(timer); window.removeEventListener('plugins:changed', refreshPlugins) })
</script>
<template>
  <div class="navigation-content">
    <nav class="app-navigation" aria-label="功能导航">
      <section v-for="group in navigationGroups" :key="group.id" class="app-navigation-group" :aria-label="group.name || '首页'">
        <h2 v-if="group.name">{{ group.name }}</h2>
        <RouterLink v-for="item in group.items" :key="item.id" :to="item.path" class="app-navigation-link" :class="{ selected: current?.id === item.id }" :aria-current="current?.id === item.id ? 'page' : undefined" @click="emit('navigate')"><component :is="item.icon" aria-hidden="true" /><span>{{ item.name }}</span></RouterLink>
      </section>
    </nav>
    <nav v-if="plugins.length" class="plugin-navigation" aria-label="插件应用"><h2>插件应用</h2><template v-for="plugin in plugins" :key="plugin.id"><RouterLink v-for="page in plugin.pages" :key="page.page" :to="`/plugins/${plugin.id}/pages/${page.page}`" class="app-navigation-link" @click="emit('navigate')">{{ plugin.name }} · {{ page.title }}</RouterLink></template></nav>
  </div>
</template>
<style scoped>
.navigation-content { min-height: 0; overflow-y: auto; padding: 16px 12px; }
.app-navigation-group + .app-navigation-group { margin-top: 20px; }
.app-navigation-group h2, .plugin-navigation h2 { margin: 0 12px 8px; color: var(--mt-text-muted); font-size: 12px; font-weight: 600; }
.app-navigation-link { display: flex; align-items: center; gap: 12px; min-height: 44px; padding: 10px 12px; border-radius: 8px; color: var(--mt-text-primary); font-size: 14px; overflow-wrap: anywhere; }
.app-navigation-link svg { width: 19px; height: 19px; flex-shrink: 0; color: var(--mt-text-muted); }
.app-navigation-link:hover { background: var(--mt-bg-canvas); }
.app-navigation-link.selected { color: var(--mt-color-primary); background: var(--mt-color-primary-light); font-weight: 600; }
.app-navigation-link.selected svg { color: var(--mt-color-primary); }
.plugin-navigation { border-top: 1px solid var(--mt-border-color); margin-top: 20px; padding-top: 20px; }
</style>
