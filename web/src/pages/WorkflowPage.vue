<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { pageById } from '../navigation/pages'
import PageTabs from '../ui/PageTabs.vue'
import LegacyTabHost from './LegacyTabHost.vue'
import { legacyViews, tabViews } from './views'
const props = defineProps<{ pageId: string }>()
const route = useRoute(), page = computed(() => pageById(props.pageId))
const index = computed(() => Math.max(0, page.value.tabs.findIndex(tab => tab.id === route.query.tab)))
const active = computed(() => page.value.tabs[index.value]!)
const view = computed(() => legacyViews[tabViews[props.pageId]![['connections','search'].includes(props.pageId) ? 0 : index.value]!]!)
</script>
<template>
  <section class="workflow-page" :aria-label="`${page.name}工作区`">
    <div v-if="page.tabs.length > 1 && !['connections','search'].includes(page.id)" class="workflow-page-navigation"><p>{{ page.description }}</p><PageTabs :tabs="page.tabs" :default-tab="page.tabs[0]?.id" /></div>
    <section class="workflow-content" :aria-label="active.label">
      <LegacyTabHost :key="['connections','search'].includes(page.id) ? page.id : `${page.id}:${active.id}`" :settings="page.id === 'settings' && active.id === 'general'"><component :is="view" /></LegacyTabHost>
    </section>
  </section>
</template>
<style scoped>
.workflow-page, .workflow-content { min-width: 0; }
.workflow-page-navigation { margin-bottom: 24px; }
.workflow-page-navigation > p { margin: 0 0 12px; color: var(--mt-text-muted); font-size: 13px; }
.workflow-page-navigation :deep(.page-tabs) { margin-bottom: 0; }
.workflow-page-navigation :deep(.tab-button) { min-height: 44px; }
.workflow-content { display: flow-root; }
</style>
