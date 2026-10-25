<script setup lang="ts">
import PathText from '../ui/PathText.vue';
import StatusBadge from '../components/StatusBadge.vue';
import {label} from './model';
import {date} from '../workflows/readiness';
defineProps<{path?:string;state?:string;time?:string;percent?:number}>();
</script>
<template>
 <article class="organize-record scan-record">
  <header class="scan-record-heading"><h3><PathText :path="path"/></h3><StatusBadge :state="state||'unknown'" :label="label(state)"/></header>
  <div class="scan-record-meta"><slot/><time v-if="time">{{date(time)}}</time></div>
  <div v-if="percent!=null" class="scan-record-progress"><el-progress :show-text="false" :stroke-width="4" :percentage="percent"/><span>{{Number(percent.toFixed(1))}}%</span></div>
  <div v-if="$slots.details" class="scan-record-details"><slot name="details"/></div>
  <div v-if="$slots.actions" class="media-actions scan-record-actions"><slot name="actions"/></div>
 </article>
</template>
<style scoped>
.scan-record{display:grid;grid-template-columns:minmax(0,1fr);gap:6px;padding:12px!important;border:1px solid var(--mt-border-color);border-radius:8px;margin-block:12px;background:var(--mt-bg-surface);min-width:0}.scan-record-heading{display:flex;align-items:center;gap:12px;min-width:0}.scan-record-heading h3{flex:1;min-width:0;margin:0;font-size:14px}.scan-record-meta{display:flex;flex-wrap:wrap;align-items:center;gap:6px 16px;font-size:12px;color:var(--mt-text-muted)}.scan-record-progress{display:grid;grid-template-columns:minmax(0,1fr) 52px;gap:12px;align-items:center;font-size:12px;font-variant-numeric:tabular-nums}.scan-record-progress .el-progress{max-width:none;width:100%;margin:0}.scan-record-progress>span{text-align:right}.scan-record-actions{margin:0;justify-content:flex-end;gap:8px}.scan-record-details{min-width:0}.scan-record-details :deep(p){overflow-wrap:anywhere}.scan-record-details :deep(summary){min-height:36px;cursor:pointer;font-size:12px}@media(max-width:600px){.scan-record-actions{justify-content:flex-start}.scan-record-actions :deep(.el-button){white-space:normal;height:auto}}
</style>
