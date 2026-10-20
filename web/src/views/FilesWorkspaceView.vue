<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import BrowserPanel from '../files/BrowserPanel.vue';
import ManagedSTRMPanel from '../files/ManagedSTRMPanel.vue';
import LinkDiagnostics from '../files/LinkDiagnostics.vue';
import STRMRepairPanel from '../files/STRMRepairPanel.vue';
import '../media/media.css';
import '../files/files.css';
import { useSystemStore } from '../stores/system';
const system = useSystemStore();
const route = useRoute(), tabs = [{ id: 'browser', label: '文件浏览' }, { id: 'strm', label: 'STRM 目录' }, { id: 'tools', label: 'STRM 工具' }], tab = computed(() => String(route.query.tab || 'browser')), browser = ref<InstanceType<typeof BrowserPanel>>(), managed = ref<InstanceType<typeof ManagedSTRMPanel>>(), repair = ref<InstanceType<typeof STRMRepairPanel>>();
const visited = ref(new Set([tab.value]));
import { watch } from 'vue';
watch(tab, v => {
  visited.value = new Set([...visited.value, v]);
});
function refresh() {
  if (tab.value === 'browser')
    void browser.value?.refresh();
  else if (tab.value === 'strm')
    void managed.value?.refresh();
  else
    void repair.value?.readback();
}
</script>
<template>
  <div class="media-workspace files-workspace">
    <PageHeader title="文件与 STRM" description="浏览目录、核对文件变更，再检查受管输出与播放链路。">
      <template #actions>
        <el-button type="primary" @click="refresh">刷新当前记录</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="browser"/>
    <div v-if="visited.has('browser')" v-show="tab==='browser'">
      <BrowserPanel ref="browser"/>
    </div>
    <div v-if="visited.has('strm')" v-show="tab==='strm'">
      <ManagedSTRMPanel ref="managed"/>
    </div>
    <div v-if="visited.has('tools')" v-show="tab==='tools'" class="media-stack">
      <STRMRepairPanel ref="repair"/>
      <LinkDiagnostics :allowed-roots="system.status?.allowedRoots||[]"/>
    </div>
  </div>
</template>
