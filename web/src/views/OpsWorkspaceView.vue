<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed, nextTick, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import DashboardWorkspaceView from './DashboardWorkspaceView.vue';
import QueuePanel from '../operations/QueuePanel.vue';
import RuntimeChecks from '../operations/RuntimeChecks.vue';
import MaintenancePanel from '../operations/MaintenancePanel.vue';
import LogsPanel from '../operations/LogsPanel.vue';
import CachePanel from '../operations/CachePanel.vue';
import '../media/media.css';
import '../workflows/u5.css';
import '../operations/u6.css';
import '../operations/presentation.css';
const route = useRoute(), tab = computed(() => String(route.query.tab || 'jobs')), tabs = [{ id: 'jobs', label: '任务列表' }, {id:'attention',label:'检测与待处理'}, { id: 'maintenance', label: '备份与告警' }, { id: 'logs', label: '日志' }, { id: 'cache', label: '缓存' }], visited = ref(new Set([tab.value])), checks = ref<InstanceType<typeof RuntimeChecks>>(), jobs = ref<InstanceType<typeof QueuePanel>>(), maintenance = ref<InstanceType<typeof MaintenancePanel>>(), logs = ref<InstanceType<typeof LogsPanel>>(), cache = ref<InstanceType<typeof CachePanel>>();
function load() {
  return ({ attention: checks.value, jobs: jobs.value, maintenance: maintenance.value, logs: logs.value, cache: cache.value } as any)[tab.value]?.load();
}
watch(tab, async (value) => {
  const existing = visited.value.has(value);
  visited.value = new Set([...visited.value, value]);
  await nextTick();
  if (existing)
    void load();
});
usePageRefresh(load,'*');
</script>
<template>
  <div class="media-workspace u5-workspace u6-workspace t8-ops">
    <PageHeader title="运维" description="查看后台任务与运行记录，按需备份、核对和维护。">
      <template #actions>

      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="jobs"/>
    <div v-if="visited.has('jobs')" v-show="tab==='jobs'">
      <QueuePanel ref="jobs"/>
    </div>
    <div class="ops-attention" v-if="visited.has('attention')" v-show="tab==='attention'"><RuntimeChecks ref="checks"/><DashboardWorkspaceView attention/></div>
    <div v-if="visited.has('maintenance')" v-show="tab==='maintenance'">
      <MaintenancePanel ref="maintenance"/>
    </div>
    <div v-if="visited.has('logs')" v-show="tab==='logs'">
      <LogsPanel ref="logs"/>
    </div>
    <div v-if="visited.has('cache')" v-show="tab==='cache'">
      <CachePanel ref="cache"/>
    </div>
  </div>
</template>
