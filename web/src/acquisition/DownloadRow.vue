<script setup lang="ts">
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
import StatusBadge from '../components/StatusBadge.vue';
defineProps<{title?:string;path?:string;state:string;stateLabel:string;percent:number|null;interactive?:boolean}>();
defineEmits<{inspect:[]}>();
</script>
<template>
 <article class="acquire-task t8-download-row">
  <div class="download-main">
   <header class="download-heading"><h3><el-tooltip v-if="interactive" :content="title" :trigger="['hover','focus']" :trigger-keys="[]" popper-class="middle-ellipsis-tooltip"><button type="button" class="t8-download-title" :title="title" @click="$emit('inspect')"><MiddleEllipsis :text="title" :copy="false" embedded/></button></el-tooltip><MiddleEllipsis v-else :text="title" :path="path" :copy="false"/></h3><StatusBadge :state="state" :label="stateLabel"/></header>
   <div class="download-progress"><el-progress v-if="percent!==null" :show-text="false" :stroke-width="4" :percentage="Math.max(0,Math.min(100,Math.round(percent)))"/><span v-else class="download-progress-unknown" aria-label="等待同步进度"/><span>{{percent===null?'—':Math.max(0,Math.min(100,Math.round(percent)))+'%'}}</span></div>
   <div class="download-row-meta"><slot name="meta"/></div>
  </div>
  <div class="download-row-actions acquire-actions"><slot name="actions"/></div>
 </article>
</template>
