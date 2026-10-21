<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { computed, ref, watch, nextTick } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import AccountsPanel from '../sites/AccountsPanel.vue';
import TemplatesPanel from '../sites/TemplatesPanel.vue';
import '../media/media.css';
import '../workflows/u5.css';
const route = useRoute(), tab = computed(() => route.query.tab === 'templates' ? 'templates' : 'accounts'), tabs = [{ id: 'accounts', label: '站点账号' }, { id: 'templates', label: '站点模板' }], visited = ref(new Set([tab.value])), accounts = ref<InstanceType<typeof AccountsPanel>>(), templates = ref<InstanceType<typeof TemplatesPanel>>();
watch(tab, async (value) => {
  const existing = visited.value.has(value);
  visited.value = new Set([...visited.value, value]);
  await nextTick();
  if (existing) {
    if (value === 'accounts')
      void accounts.value?.load();
    else
      void templates.value?.load();
  }
});
function act() {
  if (tab.value === 'accounts')
    accounts.value?.create();
  else
    void templates.value?.loadSelected();
}
usePageRefresh(()=>tab.value==='accounts'?accounts.value?.load():templates.value?.load(),'*');
</script>
<template>
  <div class="media-workspace u5-workspace u5-sites">
    <PageHeader title="站点" description="添加已登录的站点账号，按需加载内置模板。">
      <template #actions>
        <el-button type="primary" :disabled="tab==='templates'&&(!templates?.selected.length||templates?.blocked)" @click="act">{{ tab==='accounts'?'添加站点账号':'加载所选模板' }}</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="accounts"/>
    <div v-if="visited.has('accounts')" v-show="tab==='accounts'">
      <AccountsPanel ref="accounts"/>
    </div>
    <div v-if="visited.has('templates')" v-show="tab==='templates'">
      <TemplatesPanel ref="templates"/>
    </div>
  </div>
</template>
