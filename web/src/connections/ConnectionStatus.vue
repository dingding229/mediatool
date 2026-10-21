<script setup lang="ts">
import StatusBadge from '../components/StatusBadge.vue';
import StateView from '../ui/StateView.vue';
import { readinessLabel, readinessReason, stateFor, errorText, type Readiness } from '../workflows/readiness';
defineProps<{
  value: Readiness | null;
  loading: boolean;
  error: unknown;
  services: {
    key: string;
    name: string;
  }[];
}>();
defineEmits<{
  retry: [
  ];
}>();
</script>
<template>
  <section class="connection-status" aria-label="本页服务状态">
    <StateView v-if="loading" state="loading" title="正在读取服务状态…"/>
    <StateView v-else-if="error" :state="stateFor(error)" title="服务状态暂不可用" :description="errorText(error)" @retry="$emit('retry')"/>
    <dl v-else>
      <div v-for="service in services" :key="service.key">
        <dt>{{service.name}}</dt>
        <dd>
          <StatusBadge :state="value?.components?.[service.key]?.state||'unavailable'" :label="readinessLabel(value?.components?.[service.key])"/>
          <span class="connection-status-reason">{{readinessReason(value?.components?.[service.key])}}</span>
        </dd>
      </div>
    </dl>
  </section>
</template>
<style scoped>
.connection-status{margin-bottom:0;padding:16px 24px;border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);background:var(--mt-bg-surface)}dl{display:grid;gap:16px;margin:0}dl>div{display:flex;gap:16px;align-items:center;flex-wrap:wrap}dt{min-width:120px;font-weight:600;font-size:14px}dd{display:flex;align-items:center;gap:12px;flex-wrap:wrap;flex:1;margin:0;min-width:0}.connection-status-reason{font-size:13px;color:var(--mt-text-muted);overflow-wrap:anywhere}
@media(max-width:600px){.connection-status{padding:16px}dl>div{gap:8px}dt{min-width:100px}.connection-status-reason{flex-basis:100%}}
</style>
