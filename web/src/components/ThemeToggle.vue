<script setup lang="ts">
import { computed } from 'vue'
import { Sunny, Moon, Monitor } from '@element-plus/icons-vue'
import { themeMode, applyTheme, type ThemeMode } from '../ui/theme'

const modes: Array<{ mode: ThemeMode; label: string; icon: any }> = [
  { mode: 'light', label: '浅色', icon: Sunny },
  { mode: 'dark', label: '深色', icon: Moon },
  { mode: 'auto', label: '跟随系统', icon: Monitor }
]

function cycle() {
  const currentIdx = modes.findIndex(m => m.mode === themeMode.value)
  const next = modes[(currentIdx + 1) % modes.length].mode
  applyTheme(next)
}

const currentLabel = computed(() => {
  return modes.find(m => m.mode === themeMode.value)?.label || '跟随系统'
})
const CurrentIcon = computed(() => {
  return modes.find(m => m.mode === themeMode.value)?.icon || Monitor
})
</script>

<template>
  <button
    type="button"
    class="theme-toggle-btn icon-button"
    :title="`主题模式：${currentLabel}（点击切换）`"
    :aria-label="`主题模式：${currentLabel}`"
    @click="cycle"
  >
    <component :is="CurrentIcon" aria-hidden="true" />
  </button>
</template>

<style scoped>
.theme-toggle-btn {
  border: 0;
  cursor: pointer;
  color: var(--muted);
  background: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
}
.theme-toggle-btn:hover {
  color: var(--text);
  background: var(--mt-color-primary-light);
}
</style>
