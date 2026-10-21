<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

export interface TabItem {
  id: string
  label: string
  count?: number
}

const props = defineProps<{
  tabs: TabItem[]
  queryKey?: string
  defaultTab?: string
}>()

const emit = defineEmits<{
  (e: 'change', tabId: string): void
}>()

const route = useRoute()
const router = useRouter()
const key = computed(() => props.queryKey || 'tab')

const currentTab = computed(() => {
  const q = route.query[key.value]
  if (typeof q === 'string' && props.tabs.some(t => t.id === q)) {
    return q
  }
  return props.defaultTab || props.tabs[0]?.id || ''
})

function selectTab(id: string) {
  if (currentTab.value === id) return
  emit('change', id)
  const nextQuery = { ...route.query, [key.value]: id }
  router.replace({ query: nextQuery })
}
</script>

<template>
  <nav class="page-tabs" :aria-label="key">
    <button
      v-for="tab in tabs"
      :key="tab.id"
      type="button"
      class="tab-button"
      :class="{ 'is-active': currentTab === tab.id }"
      :aria-pressed="currentTab === tab.id"
      @click="selectTab(tab.id)"
    >
      <span>{{ tab.label }}</span>
      <span v-if="typeof tab.count === 'number'" class="tab-badge">{{ tab.count }}</span>
    </button>
  </nav>
</template>

<style scoped>
.page-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 20px;
  border-bottom: 1px solid var(--mt-border-color);
  padding-bottom: 8px;
}
.tab-button {
  background: transparent;
  border: 1px solid transparent;
  padding: 8px 16px;
  min-height: 44px;
  border-radius: 8px;
  color: var(--mt-text-muted);
  font-size: 14px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  transition: all 0.16s ease;
}
.tab-button:hover {
  color: var(--mt-text-primary);
  background: var(--mt-color-primary-light);
}
.tab-button.is-active {
  color: var(--mt-color-primary);
  background: var(--mt-color-primary-light);
  font-weight: 600;
  border-color: var(--mt-color-primary);
}
.tab-badge {
  font-size: 11px;
  background: var(--mt-border-color);
  color: var(--mt-text-primary);
  padding: 1px 6px;
  border-radius: 10px;
}
.tab-button.is-active .tab-badge {
  background: var(--mt-color-primary);
  color: var(--el-color-white);
}
</style>
