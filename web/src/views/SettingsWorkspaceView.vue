<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import IPGeoSettings from '../settings/IPGeoSettings.vue';
import ConfigPanel from '../settings/ConfigPanel.vue';
import ClassificationPanel from '../settings/ClassificationPanel.vue';
import ExternalAccessPanel from '../settings/ExternalAccessPanel.vue';
import AccountPanel from '../settings/AccountPanel.vue';
import MobilePanel from '../settings/MobilePanel.vue';
import { pageById } from '../navigation/pages';
import '../settings/settings.css';
const visited = ref(new Set<string>());
const route = useRoute(), tab = computed(() => String(route.query.tab || 'general')), tabs = pageById('settings').tabs;
watch(tab, value => {
  visited.value = new Set([...visited.value, value]);
}, { immediate: true });
</script>
<template>
  <div class="settings-workspace">
    <PageHeader title="设置" description="按分区修改，每个分区单独保存。"/>
    <PageTabs :tabs="tabs" default-tab="general"/>
    <div class="settings-stack" v-for="section in (['general','naming','automation'] as const)" :key="section" v-show="tab===section">
      <template v-if="visited.has(section)">
        <ConfigPanel :section="section"/>
        <ClassificationPanel v-if="section==='naming'"/>
        <IPGeoSettings v-if="section==='general'"/>
      </template>
    </div>
    <ExternalAccessPanel v-if="tab==='api'"/>
    <MobilePanel v-if="tab==='mobile'"/>
    <AccountPanel v-if="tab==='security'"/>
  </div>
</template>
