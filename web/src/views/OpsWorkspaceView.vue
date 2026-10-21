<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import QueuePanel from '../operations/QueuePanel.vue';
import MaintenancePanel from '../operations/MaintenancePanel.vue';
import LogsPanel from '../operations/LogsPanel.vue';
import DataPanel from '../operations/DataPanel.vue';
import CachePanel from '../operations/CachePanel.vue';
import '../media/media.css';
import '../workflows/u5.css';
import '../operations/u6.css';
const route = useRoute(), tab = computed(() => String(route.query.tab || 'jobs')), tabs = [{ id: 'jobs', label: '任务队列' }, { id: 'maintenance', label: '备份与告警' }, { id: 'logs', label: '日志' }, { id: 'data', label: '数据浏览' }, { id: 'cache', label: '缓存' }], visited = ref(new Set([tab.value])), jobs = ref<InstanceType<typeof QueuePanel>>(), maintenance = ref<InstanceType<typeof MaintenancePanel>>(), logs = ref<InstanceType<typeof LogsPanel>>(), data = ref<InstanceType<typeof DataPanel>>(), cache = ref<InstanceType<typeof CachePanel>>();
function load() {
  return ({ jobs: jobs.value, maintenance: maintenance.value, logs: logs.value, data: data.value, cache: cache.value } as any)[tab.value]?.load();
}
watch(tab, async (value) => {
  const existing = visited.value.has(value);
  visited.value = new Set([...visited.value, value]);
  await nextTick();
  if (existing)
    void load();
});
</script>
<template>
  <div class="media-workspace u5-workspace u6-workspace">
    <PageHeader title="运维" description="查看后台任务与运行记录，按需备份、核对和维护。">
      <template #actions>
        <el-button type="primary" @click="load">刷新当前记录</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="jobs"/>
    <div v-if="visited.has('jobs')" v-show="tab==='jobs'">
      <QueuePanel ref="jobs"/>
    </div>
    <div v-if="visited.has('maintenance')" v-show="tab==='maintenance'">
      <MaintenancePanel ref="maintenance"/>
    </div>
    <div v-if="visited.has('logs')" v-show="tab==='logs'">
      <LogsPanel ref="logs"/>
    </div>
    <div v-if="visited.has('data')" v-show="tab==='data'">
      <DataPanel ref="data"/>
    </div>
    <div v-if="visited.has('cache')" v-show="tab==='cache'">
      <CachePanel ref="cache"/>
    </div>
  </div>
</template>
