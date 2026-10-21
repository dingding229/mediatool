<script setup lang="ts">
import { computed } from 'vue';
import { pageById } from '../navigation/pages';
import { workspaceViews } from './views';
const props = defineProps<{
  pageId: string;
}>(), page = computed(() => pageById(props.pageId)), view = computed(() => workspaceViews[props.pageId]);
</script>
<template>
  <section class="workflow-page" :aria-label="`${page.name}工作区`">
    <component :is="view" :key="page.id"/>
  </section>
</template>
<style scoped>
.workflow-page{min-width:0;display:flow-root}
</style>
