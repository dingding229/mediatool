<script setup lang="ts">
import ConnectionHealth from '../connections/ConnectionHealth.vue';
import StateView from '../ui/StateView.vue';
import StatusBadge from '../components/StatusBadge.vue';
import { readinessLabel, readinessReason, services, stateFor, errorText, type Readiness } from './readiness';
defineProps<{
  value: Readiness | null;
  loading: boolean;
  error: unknown;
  tests?:Partial<Record<string,{available:boolean;busy:boolean;locked:boolean}>>;
  overview?:boolean;
}>();
defineEmits<{
  test: [kind: string];
  retry: [
  ];
}>();
</script>
<template>
<section class="service-readiness" :class="{'is-overview':overview}" aria-label="服务就绪状态">
  <StateView v-if="loading" state="loading" title="正在检查服务状态…" />
  <StateView v-else-if="error" :state="stateFor(error)" title="服务状态暂不可用" :description="errorText(error)" @retry="$emit('retry')" />
  <div v-else class="readiness-cards">
    <article v-for="service in services.filter(s=>!overview||s.key!=='strm')" :key="service.key">
      <h3>{{ service.name }}</h3>
      <ConnectionHealth v-if="service.key!=='strm'&&value?.components?.[service.key]?.state!=='not_configured'" :state="value?.components?.[service.key]?.state||'unavailable'" :checked-at="value?.components?.[service.key]?.lastCheckedAt" :message="value?.components?.[service.key]?.message"/>
      <StatusBadge v-else :state="value?.components?.[service.key]?.state || 'unavailable'" :label="readinessLabel(value?.components?.[service.key])" />
      <p v-if="service.key==='strm'||value?.components?.[service.key]?.state==='not_configured'">{{ readinessReason(value?.components?.[service.key]) }}</p>
      <el-button v-if="!overview&&service.key!=='strm'&&value?.components?.[service.key]?.state!=='disabled'&&tests?.[service.key]?.available" class="readiness-test" :loading="tests?.[service.key]?.busy" :disabled="tests?.[service.key]?.locked||tests?.[service.key]?.busy" @click="$emit('test',service.key)">立即检测</el-button>
      <RouterLink :to="{ path: '/connections', query: { tab: service.tab } }">
        {{value?.components?.[service.key]?.state==='ready'?'查看':'去处理'}}
        <span class="sr-only">{{ service.name }}</span>
        →
      </RouterLink>
    </article>
  </div>
</section>
</template>
<style scoped>
.service-readiness.is-overview{min-height:0}
.is-overview .readiness-cards article{padding:16px}
@media(min-width:601px){
  .is-overview .readiness-cards article{display:flex;flex-direction:column}
  .is-overview .readiness-cards h3{margin:0 0 12px}
  .is-overview .readiness-cards a{align-self:flex-start;align-items:flex-end;margin-top:auto}
}

.readiness-cards  {
   display:grid;
  grid-template-columns:repeat(4,minmax(0,1fr));
  gap:16px
}
.readiness-cards:has(>article:nth-child(3):last-child){grid-template-columns:repeat(3,minmax(0,1fr))}
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
  .readiness-cards,.readiness-cards:has(>article:nth-child(3):last-child) {
    grid-template-columns:1fr 1fr
  }
}
@media(max-width:600px) {
  .is-overview .readiness-cards article{padding:12px 16px}
  .is-overview .readiness-cards h3{align-self:center}
  .readiness-cards,.readiness-cards:has(>article:nth-child(3):last-child) {
    grid-template-columns:1fr
  }
  .readiness-cards{gap:0;border:1px solid var(--mt-border-color);border-radius:12px;background:var(--mt-bg-surface);overflow:hidden}
  .readiness-cards article{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 12px;border:0;border-radius:0;padding:12px 16px;border-bottom:1px solid var(--mt-border-color)}
  .readiness-cards article:last-child{border-bottom:0}
  .readiness-cards h3{font-size:14px;margin:0}
  .readiness-cards :deep(.status-badge){justify-self:end}.readiness-cards :deep(.connection-health){grid-column:1/-1;grid-row:2}.readiness-cards article:has(.connection-health) a{grid-column:2;grid-row:1}
  .readiness-cards p{min-height:0;margin:0;grid-column:1;font-size:12px}
  .readiness-cards a{grid-column:2;grid-row:2;justify-self:end;align-self:center;min-height:44px}

}
.readiness-test{min-height:44px;margin:0 8px 0 0}
@media(max-width:600px){.readiness-test{grid-column:1;justify-self:start;grid-row:3}.readiness-cards article:has(.readiness-test) a{grid-row:3}}
</style>
