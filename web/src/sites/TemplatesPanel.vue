<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
import { publicAccount } from './accountModel';
const catalog = reactive(useResource<any>(() => request('/pt/templates'))), accounts = reactive(useResource<any>(() => request<any[]>('/integrations').then(rows => rows.filter(r => r.kind === 'ptsite').map(publicAccount)), [])), selected = ref<string[]>([]), query = ref(''), op = reactive(useOperation('templates')), detail = ref<any>(null), detailOpen = ref(false);
const iconErrors=ref(new Set<string>());
const orphaned = computed(() => catalog.data && !catalog.error && !catalog.loading ? accounts.data.filter((a: any) => !catalog.data.templates.some((t: any) => t.id === a.definitionId)).map((a: any) => ({ id: a.definitionId, name: a.name, domain: a.settings.endpoint, unavailable: true, loaded: null })) : []);
const available = computed(() => catalog.data?.templates.filter((t: any) => !t.loaded) || []), shown = computed(() => [...(catalog.data?.templates || []), ...orphaned.value].filter((t: any) => (t.name + ' ' + t.domain).toLowerCase().includes(query.value.toLowerCase())));
async function load() {
  await Promise.all([catalog.load(), accounts.load()]);
  if (op.pending && !catalog.error) {
    const ids = JSON.parse(op.pending.key);
    if (ids.every((id: string) => catalog.data.templates.some((t: any) => t.id === id && t.loaded)))
      op.clear();
  }
}
function selectAll() {
  selected.value = selected.value.length === available.value.length ? [] : available.value.map((t: any) => t.id);
}
async function loadSelected() {
  if (!selected.value.length || selected.value.some(id => !available.value.some((site: any) => site.id === id)) || op.pending || catalog.loading || catalog.error)
    return;
  await op.run(JSON.stringify([...selected.value]), () => post('/pt/templates/load', { ids: [...selected.value] }));
  await load();
  if (!op.pending && !catalog.error)
    selected.value = [];
}
const blocked = computed(() => !!op.pending || op.busy || catalog.loading || !!catalog.error);
onMounted(load);
defineExpose({ load, loadSelected, selected, blocked });
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>站点模板</h2>
      <el-button :disabled="!available.length||!!op.pending" @click="selectAll">{{ selected.length===available.length&&selected.length?'清空选择':'选择全部未加载模板' }}</el-button>
    </header>
    <p>只加载你需要的内置模板；已被账号使用的模板会保留。</p>
    <label class="u5-field">
      查找模板
      <input v-model="query" aria-label="查找站点模板"/>
    </label>
    <ResourceState overlay :stale="catalog.loaded" :loading="catalog.loading" :error="catalog.error" :empty="!shown.length" @reload="load"/>
    <ResourceState :error="accounts.error" @reload="accounts.load"/>
    <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'模板加载已排队，尚未核验完成。':'正在核对原批次，不重复加载。'" @refresh="load"/>
    <div class="f5-template-list">
      <article v-for="site in shown" :key="site.id" class="u5-template">
        <label class="u5-check">
          <input v-model="selected" type="checkbox" :value="site.id" :aria-label="'选择模板 '+site.name" :disabled="site.loaded||site.unavailable||!!op.pending||!!catalog.error"/>
          <img v-if="site.iconUrl?.startsWith('/api/v1/pt/templates/')&&!iconErrors.has(site.id)" :src="site.iconUrl" alt="" width="32" height="32" loading="lazy" referrerpolicy="no-referrer" @error="iconErrors.add(site.id)"/>
          <span v-else class="f5-template-icon" aria-hidden="true">{{Array.from(site.name)[0]}}</span>
          <strong>{{ site.name }}</strong>
        </label>
        <a v-if="/^https?:\/\//.test(site.domain)" :href="site.domain" target="_blank" rel="noopener noreferrer">{{ site.domain }}</a>
        <p v-else>{{ site.domain||'—' }}</p>
        <p>{{ accounts.data.some((a:any)=>a.definitionId===site.id)?site.unavailable?'使用中 · 当前内置列表未提供':'使用中':site.loaded?'已加载':'未加载' }}</p>
        <el-button @click="detail=site;detailOpen=true">模板详情</el-button>
      </article>
    </div>
    <p>当前不支持移除已加载模板；账号使用中的模板不能移除。</p>
  </section>
  <DetailDrawer v-model:visible="detailOpen" title="内置模板详情" :technical-data="detail" class="media-detail">
    <h3>{{ detail?.name||'—' }}</h3>
    <p>{{ detail?.domain||'—' }}</p>
    <p>{{ detail?.unavailable?'不在当前内置列表，账号和已有配置仍保留。':detail?.loaded?'已加载':'尚未加载' }}</p>
    <RouterLink to="/sites?tab=accounts">管理站点账号</RouterLink>
  </DetailDrawer>
</template>

<style scoped>
.f5-template-icon{display:grid;place-items:center;width:32px;height:32px;flex:none;background:var(--mt-bg-canvas);border-radius:6px}
.f5-template-list{min-height:460px;display:grid;gap:0;border-block:1px solid var(--mt-border-color);margin-block:20px}.f5-template-list>.u5-template{display:grid;grid-template-columns:minmax(160px,1fr) minmax(150px,1.5fr) minmax(90px,auto) auto;align-items:center;gap:20px;border:0;border-bottom:1px solid var(--mt-border-color);border-radius:0;padding:14px 0}.f5-template-list>.u5-template:last-child{border-bottom:0}.f5-template-list a{overflow-wrap:anywhere}.f5-template-list p{margin:0;font-size:13px;color:var(--mt-text-muted)}
@media(max-width:700px){.f5-template-list>.u5-template{grid-template-columns:minmax(0,1fr) auto;gap:12px}.f5-template-list a{grid-column:1/-1;grid-row:2}.f5-template-list .u5-template>p{grid-column:1}.f5-template-list .el-button{grid-column:2;grid-row:1}}
</style>
