<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed, ref, watch, nextTick } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import ServicePanel from '../playback/ServicePanel.vue';
import UsersPanel from '../playback/UsersPanel.vue';
import AccessPanel from '../playback/AccessPanel.vue';
import SessionsPanel from '../playback/SessionsPanel.vue';
import ProcessingPanel from '../playback/ProcessingPanel.vue';
import '../media/media.css';
import '../workflows/u5.css';
const route = useRoute(), tabs = [{ id: 'settings', label: '服务设置' }, { id: 'users', label: '媒体用户' }, { id: 'players', label: '播放器准入' }, { id: 'sessions', label: '在线会话' }, { id: 'processing', label: '预处理' }], tab = computed(() => String(route.query.tab || 'settings')), visited = ref(new Set([tab.value])), service = ref<InstanceType<typeof ServicePanel>>(), users = ref<InstanceType<typeof UsersPanel>>(), access = ref<InstanceType<typeof AccessPanel>>(), sessions = ref<InstanceType<typeof SessionsPanel>>(), processing = ref<InstanceType<typeof ProcessingPanel>>();
watch(tab, async (v) => {
  const existed = visited.value.has(v);
  visited.value = new Set([...visited.value, v]);
  await nextTick();
  if (existed) {
    if (v === 'users')
      void users.value?.load();
    else
      act();
  }
});
function act() {
  if (tab.value === 'users')
    users.value?.create();
  else if (tab.value === 'players')
    return access.value?.load();
  else if (tab.value === 'sessions')
    return sessions.value?.load();
  else if (tab.value === 'processing')
    return processing.value?.load();
  else
    return service.value?.load();
}
usePageRefresh(()=>tab.value==='users'?users.value?.load():act(),'*');
</script>
<template>
  <div class="media-workspace u5-workspace">
    <PageHeader title="播放服务" description="管理播放服务、家人账号和设备访问。">
      <template #actions>
        <el-button v-if="tab==='users'" type="primary" @click="act">新建媒体用户</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="settings"/>
    <div v-if="visited.has('settings')" v-show="tab==='settings'">
      <ServicePanel ref="service"/>
    </div>
    <div v-if="visited.has('users')" v-show="tab==='users'">
      <UsersPanel ref="users"/>
    </div>
    <div v-if="visited.has('players')" v-show="tab==='players'">
      <AccessPanel ref="access"/>
    </div>
    <div v-if="visited.has('sessions')" v-show="tab==='sessions'">
      <SessionsPanel ref="sessions"/>
    </div>
    <div v-if="visited.has('processing')" v-show="tab==='processing'">
      <ProcessingPanel ref="processing" :initial-kind="String(route.query.kind||'')" :job-id="String(route.query.jobId||'')"/>
    </div>
  </div>
</template>
