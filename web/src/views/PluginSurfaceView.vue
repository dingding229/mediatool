<script setup lang="ts">
import { computed, reactive, watch } from 'vue';
import { useRoute } from 'vue-router';
import { uiSurfaces } from '../api/plugins';
import PluginSurface from '../plugins/PluginSurface.vue';
import PageHeader from '../ui/PageHeader.vue';
import ResourceState from '../workflows/ResourceState.vue';
import StateView from '../ui/StateView.vue';
import { useResource } from '../workflows/useResource';
import '../media/media.css';
const route = useRoute(), data = reactive(useResource(uiSurfaces, [])), plugin = computed(() => data.data.find(i => i.instanceId === route.params.id)?.pluginId);
watch(() => [route.params.id, route.params.key], () => data.load(), { immediate: true });
</script>
<template>
  <div class="media-workspace u6-workspace">
    <PageHeader title="插件应用" description="此页面由已启用的内置插件提供。"/>
    <RouterLink :to="'/plugins/'+encodeURIComponent(String(route.params.id))">返回插件详情</RouterLink>
    <ResourceState :loading="data.loading" :error="data.error" @reload="data.load"/>
    <StateView v-if="!data.loading&&!data.error&&!plugin" state="unsupported" title="插件未启用或暂不可用" description="配置和数据仍保留，请在插件详情核对状态。"/>
    <section v-if="plugin&&!data.error" class="media-surface u6-runtime">
      <PluginSurface :key="String(route.params.id)+':'+String(route.params.key)" :instance-id="String(route.params.id)" :plugin-id="plugin" :page-key="String(route.params.key)"/>
    </section>
  </div>
</template>
