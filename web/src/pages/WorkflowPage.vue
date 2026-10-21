<script setup lang="ts">
import PageRefreshScope from '../ui/PageRefreshScope.vue';
import { computed } from 'vue';
import { pageById } from '../navigation/pages';
import { workspaceViews } from './views';
const props = defineProps<{
  pageId: string;
}>(), page = computed(() => pageById(props.pageId)), view = computed(() => workspaceViews[props.pageId]);
</script>
<template>
  <section class="workflow-page" :aria-label="`${page.name}工作区`">
    <PageRefreshScope :key="page.id" :default-tab="page.tabs[0]?.id"><component :is="view"/></PageRefreshScope>
  </section>
</template>
<style scoped>
.workflow-page{min-width:0;display:flow-root}
</style>
