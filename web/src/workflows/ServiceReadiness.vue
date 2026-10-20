<script setup lang="ts">
import StateView from '../ui/StateView.vue';
import StatusBadge from '../components/StatusBadge.vue';
import { readinessLabel, readinessReason, services, stateFor, errorText, type Readiness } from './readiness';
defineProps<{
  value: Readiness | null;
  loading: boolean;
  error: unknown;
}>();
defineEmits<{
  retry: [
  ];
}>();
</script>
<template>
<section class="service-readiness" aria-label="服务就绪状态">
  <StateView v-if="loading" state="loading" title="正在检查服务状态…" />
  <StateView v-else-if="error" :state="stateFor(error)" title="服务状态暂不可用" :description="errorText(error)" @retry="$emit('retry')" />
  <div v-else class="readiness-cards">
    <article v-for="service in services" :key="service.key">
      <h3>{{ service.name }}</h3>
      <StatusBadge :state="value?.components?.[service.key]?.state || 'unavailable'" :label="readinessLabel(value?.components?.[service.key])" />
      <p>{{ readinessReason(value?.components?.[service.key]) }}</p>
      <RouterLink :to="{ path: '/connections', query: { tab: service.tab } }">
        去处理
        <span class="sr-only">{{ service.name }}</span>
        →
      </RouterLink>
    </article>
  </div>
</section>
</template>
<style scoped>
.readiness-cards  {
   display:grid;
  grid-template-columns:repeat(4,minmax(0,1fr));
  gap:16px
}
.readiness-cards article  {
   padding:20px;
  border:1px solid var(--mt-border-color);
  border-radius:12px;
  background:var(--mt-bg-surface);
  min-width:0
}
.readiness-cards h3  {
   margin-bottom:12px;
  font-size:15px
}
.readiness-cards p  {
   font-size:12px;
  color:var(--mt-text-muted);
  line-height:1.7;
  min-height:42px;
  margin:12px 0;
  overflow-wrap:anywhere
}
.readiness-cards a  {
   display:inline-flex;
  align-items:center;
  min-height:44px;
  color:var(--mt-color-primary);
  font-size:13px
}
.sr-only  {
   position:absolute;
  width:1px;
  height:1px;
  overflow:hidden;
  clip-path:inset(50%)
}
@media(max-width:1100px) {
  .readiness-cards {
    grid-template-columns:1fr 1fr
  }
}
@media(max-width:600px) {
  .readiness-cards {
    grid-template-columns:1fr
  }
  .readiness-cards p {
    min-height:0
  }
}
</style>
