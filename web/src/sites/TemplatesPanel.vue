<script setup lang="ts">
import {usePageNumber,usePageSize} from '../ui/page-query';
import {useDebouncedFilter} from '../composables/useDebouncedFilter';
import TemplateRow from './TemplateRow.vue';
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText } from '../workflows/readiness';
import { publicAccount } from './accountModel';
const catalog = reactive(useResource<any>(() => request('/pt/templates'))), accounts = reactive(useResource<any>(() => request<any[]>('/integrations').then(rows => rows.filter(r => r.kind === 'ptsite').map(publicAccount)), [])), selected = ref<string[]>([]), query = ref(''), op = reactive(useOperation('templates')), detail = ref<any>(null), detailOpen = ref(false);
const initialRead=computed(()=>(!catalog.loaded&&!catalog.error)||(!accounts.loaded&&!accounts.error));
const page=usePageNumber('templatesPage'),pageSize=usePageSize(25,'templatesPage'),appliedQuery=ref('');useDebouncedFilter(query,value=>{page.value=1;appliedQuery.value=value.trim()});
const orphaned = computed(() => catalog.data && !catalog.error && !catalog.loading ? accounts.data.filter((a: any) => !catalog.data.templates.some((t: any) => t.id === a.definitionId)).map((a: any) => ({ id: a.definitionId, name: a.name, domain: a.settings.endpoint, unavailable: true, loaded: null })) : []);
const available = computed(() => catalog.data?.templates.filter((t:any)=>!t.loaded&&!t.unavailable)||[]),loaded = computed(()=>[...(catalog.data?.templates||[]).filter((t:any)=>t.loaded),...orphaned.value]),shown = computed(()=>available.value.filter((t:any)=>(t.name+' '+t.domain).toLowerCase().includes(appliedQuery.value.toLowerCase()))),pageItems=computed(()=>shown.value.slice((page.value-1)*pageSize.value,page.value*pageSize.value));
watch(()=>shown.value.length,()=>{if(page.value>Math.max(1,Math.ceil(shown.value.length/pageSize.value)))page.value=1});
async function load() {
  await Promise.all([catalog.load(), accounts.load()]);
  if (op.pending && !catalog.error) {
    const ids = JSON.parse(op.pending.key);
    if (ids.every((id: string) => catalog.data.templates.some((t: any) => t.id === id && t.loaded)))
      op.clear();
  }
}
function selectAll() {
 const ids=pageItems.value.map((t:any)=>t.id);
 selected.value=ids.every((id:string)=>selected.value.includes(id))?selected.value.filter(id=>!ids.includes(id)):[...new Set([...selected.value,...ids])].slice(0,30);
}
function select(id:string,checked:boolean){if(checked&&!selected.value.includes(id)&&selected.value.length<30)selected.value=[...selected.value,id];else if(!checked)selected.value=selected.value.filter(value=>value!==id)}

async function loadSelected() {
  if (!selected.value.length || selected.value.some(id => !available.value.some((site: any) => site.id === id)) || op.pending || catalog.loading || catalog.error)
    return;
  await op.run(JSON.stringify([...selected.value]), () => post('/pt/templates/load', { ids: [...selected.value] }));
  await load();
  if (!op.pending && !catalog.error)
    selected.value = [];
}
const blocked = computed(() => !!op.pending || op.busy || catalog.loading || !!catalog.error || selected.value.length>30 || selected.value.some(id=>!available.value.some((site:any)=>site.id===id)));
onMounted(load);
defineExpose({ load, loadSelected, selected, blocked });
</script>
<template>
 <section v-if="initialRead" class="media-surface templates-initial-read" aria-label="正在读取站点模板" aria-busy="true"><ResourceState loading title="正在读取站点模板与账号…"/></section>
 <template v-else>
 <section class="media-surface templates-loaded" aria-label="已加载模板">
  <header class="media-section-heading"><h2>已加载（{{loaded.length}}）</h2></header>
  <p>只加载你需要的内置模板；账号使用中的模板会保留，当前不支持移除。</p>
  <ResourceState :stale="catalog.loaded" :loading="catalog.loading" :error="catalog.error" :empty="!loaded.length" empty-title="尚未加载站点模板" @reload="load"/>
  <ResourceState :error="accounts.error" @reload="load"/>
  <div class="f5-template-list" role="list" aria-label="已加载模板列表"><TemplateRow v-for="site in loaded" :key="site.id" :site="site" :used="accounts.data.some((a:any)=>a.definitionId===site.id)" @details="detail=site;detailOpen=true"/></div>
 </section>
 <section class="media-surface templates-available" aria-label="可加载的模板">
  <header class="media-section-heading"><h2>可加载的模板</h2><el-input v-model="query" aria-label="查找站点模板" placeholder="搜索模板名称或域名" clearable/></header>
  <div class="media-actions"><el-button :disabled="!pageItems.length||blocked||selected.length>=30&&!pageItems.every((site:any)=>selected.includes(site.id))" @click="selectAll">选择本页未加载模板</el-button><el-button v-if="selected.length" :disabled="!!op.pending||op.busy" @click="selected=[]">清空选择</el-button><span>已选 {{selected.length}} 项，每批最多 30 项</span></div>
  <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
  <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'模板加载已排队，尚未核验完成。':'正在核对原批次，不重复加载。'" @refresh="load"/>
  <ResourceState :stale="catalog.loaded" :loading="catalog.loading" :error="catalog.error" :empty="!shown.length" empty-title="暂无可加载的匹配模板" @reload="load"/>
  <div class="f5-template-list" role="list" aria-label="可加载模板列表"><TemplateRow v-for="site in pageItems" :key="site.id" :site="site" selectable :selected="selected.includes(site.id)" :disabled="!!op.pending||!!catalog.error||!selected.includes(site.id)&&selected.length>=30" @select="select(site.id,$event)" @details="detail=site;detailOpen=true"/></div>
  <PagePagination v-model:current-page="page" v-model:page-size="pageSize" :total="shown.length" label="可加载模板分页"/>
 </section>
 </template>
 <DetailDrawer v-model:visible="detailOpen" title="内置模板详情" class="media-detail"><h3>{{detail?.name||'—'}}</h3><p>{{detail?.domain||'—'}}</p><p>{{detail?.unavailable?'不在当前内置列表，账号和已有配置仍保留。':detail?.loaded?'已加载':'尚未加载'}}</p><RouterLink to="/sites?tab=accounts">管理站点账号</RouterLink></DetailDrawer>
</template>
<style scoped>.templates-initial-read{min-height:100dvh}.f5-template-list{min-height:0;margin-block:16px;border-block:1px solid var(--mt-border-color)}.f5-template-list:empty{display:none}.templates-available .el-input{width:260px;max-width:100%}.templates-available>.media-actions{font-size:12px;color:var(--mt-text-muted)}.templates-loaded>p{font-size:13px;color:var(--mt-text-muted)}</style>
