<script setup lang="ts">
import ConnectionHealth from './ConnectionHealth.vue';
import StateView from '../ui/StateView.vue';
import StatusBadge from '../components/StatusBadge.vue';
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
  <section class="connection-status" :style="{'--f5-status-rows':services.length}" aria-label="本页服务状态">
    <StateView v-if="error" compact :state="stateFor(error)" :description="errorText(error)" @retry="$emit('retry')"/>
    <dl v-else>
      <div v-for="service in services" :key="service.key">
        <dt>{{service.name}}</dt>
        <dd v-if="loading"><span role="status">正在读取服务状态…</span></dd>
        <dd v-else>
          <ConnectionHealth v-if="service.key!=='strm'&&value?.components?.[service.key]?.state!=='not_configured'" :state="value?.components?.[service.key]?.state||'unavailable'" :checked-at="value?.components?.[service.key]?.lastCheckedAt" :message="value?.components?.[service.key]?.message"/>
          <StatusBadge v-else :state="value?.components?.[service.key]?.state||'unavailable'" :label="readinessLabel(value?.components?.[service.key])"/>
          <span v-if="service.key==='strm'||value?.components?.[service.key]?.state==='not_configured'" class="connection-status-reason">{{readinessReason(value?.components?.[service.key])}}</span>
        </dd>
      </div>
    </dl>
  </section>
</template>
<style scoped>
.connection-status{min-height:calc(16px + var(--f5-status-rows)*60px);box-sizing:border-box;margin-bottom:0;padding:16px 24px;border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);background:var(--mt-bg-surface)}dl{display:grid;gap:16px;margin:0}dl>div{min-height:44px;display:flex;gap:16px;align-items:center;flex-wrap:wrap}dt{min-width:120px;font-weight:600;font-size:14px}dd{display:flex;align-items:center;gap:12px;flex-wrap:wrap;flex:1;margin:0;min-width:0}.connection-status-reason{font-size:13px;color:var(--mt-text-muted);overflow-wrap:anywhere}
@media(max-width:600px){.connection-status{padding:16px;min-height:calc(16px + var(--f5-status-rows)*104px)}dl>div{gap:8px;min-height:88px}dt{min-width:100px}.connection-status-reason{flex-basis:100%}}
</style>
