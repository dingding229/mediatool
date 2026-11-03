<script setup lang="ts">
import inventory from '../../../internal/modules/plugins/v2/library.json';
import {extensionTitle,extensionDanger,runTitle} from './extension-presentation';
import PagePagination from '../ui/PagePagination.vue';

import {usePageNumber,usePageSize} from '../ui/page-query';

import { safePluginError } from './model';
import { computed, onMounted, reactive, ref } from 'vue';
import { extensions, runHistory, runExtension } from '../api/plugins';
import { useResource } from '../workflows/useResource';
import { savePrivate } from '../organize/model';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const props = defineProps<{
  id: string;
  pluginId?: string;
  running: boolean;
}>(), page = usePageNumber(),pageSize=usePageSize(25), read = reactive(useResource(() => Promise.all([extensions(props.id), runHistory(props.id, page.value,pageSize.value)]))), op = reactive(useOperation('plugin-run')), selected = ref<any>(null), confirm = ref(false);
const pendingInstance = computed(() => {
  try {
    return op.pending ? JSON.parse(op.pending.key).instanceId : '';
  }
  catch {
    return '';
  }
});
const runnable = computed(() => read.data?.[0].metadata.descriptors.filter(d => ['service', 'command', 'action'].includes(d.kind)) || []), states: Record<string, string> = { pending: '等待执行', running: '执行中', succeeded: '已完成', failed: '失败', unknown: '结果未知，请先核对', canceled: '已取消' };
async function load() {
  await read.load();
  if (op.pending && !read.error && read.data) {
    const p = JSON.parse(op.pending.key);
    if (p.runId && read.data[1].some(r => r.id === p.runId && !['pending', 'running', 'unknown'].includes(r.state)))
      op.clear();
  }
}
async function run() {
  const d = selected.value, out = await op.run(JSON.stringify({ descriptor: d.id, kind: d.kind, instanceId: props.id }), () => runExtension(props.id, { kind: d.kind, id: d.id, operationKey: op.pending.token, confirm: true }));
  confirm.value = false;
  if (out && op.pending) {
    op.pending.key = JSON.stringify({ descriptor: d.id, kind: d.kind, instanceId: props.id, ...out });
    savePrivate('media-tool:u5:plugin-run', op.pending);
  }
  await load();
}
async function reconcile() {
  await load();
  if (!op.pending || read.error || op.busy)
    return;
  const p = JSON.parse(op.pending.key);
  if (p.instanceId !== props.id) {
    op.error = Error('另一插件的执行结果仍待核对，请先返回该实例。');
    return;
  }
  if (p.runId || p.jobId)
    return;
  const descriptor = read.data?.[0].metadata.descriptors.find(d => d.id === p.descriptor && d.kind === p.kind);
  if (!descriptor || !props.running)
    return;
  op.busy = true;
  try {
    const out = await runExtension(props.id, { kind: p.kind, id: p.descriptor, operationKey: op.pending.token, confirm: true });
    op.pending.key = JSON.stringify({ ...p, ...out });
    op.phase = 'queued';
    savePrivate('media-tool:u5:plugin-run', op.pending);
    op.error = null;
    await load();
  }
  catch (e) {
    op.error = Error('原请求仍未确认，请先核对运行记录，不重复创建。');
  }
  finally {
    op.busy = false;
  }
}
const history=computed(()=>read.data?.[1].filter(r=>r.kind!=='ui')||[]);
const titles=computed(()=>{const manifest=inventory.manifests.find(m=>m.id===props.pluginId);return (manifest?.extensions as {titles?:Record<string,string>}|undefined)?.titles||{}});
const nameOf=(d:Parameters<typeof extensionTitle>[0])=>extensionTitle(d,history.value,titles.value);
onMounted(load);
</script>
<template>
  <section aria-label="插件服务与命令">
    <h3>常用操作</h3>
    <p v-if="!running">插件尚未运行；请启用后再执行。</p>
    <ResourceState :stale="read.loaded" :loading="read.loading" :error="safePluginError(read.error)" @reload="load"/>
    <el-alert v-if="op.error" :title="op.pending?'执行请求结果尚未确认，请先核对。':'请求未完成，请重新核对插件状态与权限。'" type="error" :closable="false"/>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" message="已提交或结果尚未确认，不是执行成功；请先核对运行记录。" @refresh="load"/>
    <RouterLink v-if="pendingInstance&&pendingInstance!==id" :to="'/plugins/'+encodeURIComponent(pendingInstance)">返回待核对插件</RouterLink>
    <el-button v-if="op.pending" :disabled="op.busy||!!read.error" @click="reconcile">核对原执行请求</el-button>
    <article v-for="d in runnable" :key="d.kind+':'+d.id" class="organize-record plugin-extension">
      <header class="plugin-extension-heading"><h4>{{ nameOf(d) }}</h4><el-tag v-if="extensionDanger(d)" size="small" type="danger">危险操作</el-tag></header>
      <p v-if="d.description?.trim()">{{ d.description }}</p>
      <el-button :disabled="!running||!!op.pending||!!read.error" @click="selected=d;confirm=true">执行</el-button>
    </article>
    <h3>执行历史</h3>
    <p v-if="read.loaded&&!read.error&&!history.length" class="f5-empty">暂无执行记录</p>
    <article v-for="r in history" :key="r.id" class="plugin-run-row">
      <el-tag size="small" :type="r.state==='succeeded'?'success':r.state==='failed'?'danger':'info'">{{states[r.state]||'状态待核对'}}</el-tag><time>{{date(r.createdAt)}}</time><span class="plugin-run-name">{{runTitle(r,read.data?.[0].metadata.descriptors||[],titles,history)}}</span>
      <span v-if="r.reason" class="plugin-run-reason">请查看日志核对原因。</span>
      <RouterLink :to="{path:'/ops',query:{tab:'logs',module:'plugins',instance:id}}">日志</RouterLink>
    </article>
    <PagePagination v-model:page-size="pageSize" v-model:current-page="page" :item-count="history.length||0" :has-next="(history.length||0)>=pageSize" @current-change="load"/>
    <ConfirmAction v-model:visible="confirm" title="执行插件扩展" :description="'将执行 '+(selected?nameOf(selected):'此扩展')" impact-scope="可能产生插件权限范围内的业务变更；结果未知时先核对，不会自动重试。" confirm-text="确认执行" class="media-dialog" :loading="op.busy" @confirm="run"/>
  </section>
</template>

<style scoped>
.plugin-extension{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:8px 16px;align-items:center}.plugin-extension-heading{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.plugin-extension h4{margin:0;font-size:14px}.plugin-extension>p{grid-column:1;margin:0;font-size:12px;color:var(--mt-text-muted)}.plugin-extension>.el-button{grid-column:2;grid-row:1/3;margin:0}.plugin-run-row{display:flex;align-items:center;gap:12px;flex-wrap:wrap;padding-block:3px;border-bottom:1px solid var(--mt-border-color);font-size:13px;min-width:0}.plugin-run-row time{white-space:nowrap;color:var(--mt-text-muted)}.plugin-run-name{min-width:0;overflow-wrap:anywhere;flex:1}.plugin-run-row>a{font-size:12px;white-space:nowrap}.plugin-run-reason{font-size:12px;color:var(--mt-text-muted)}
@media(max-width:600px){.plugin-run-row{display:grid;grid-template-columns:max-content minmax(0,1fr) auto;gap:4px}.plugin-run-row>.el-tag{grid-column:1;grid-row:1}.plugin-run-row>time{grid-column:2;grid-row:1;font-size:12px}.plugin-run-row>a{grid-column:3;grid-row:1}.plugin-run-name{grid-column:1/-1;grid-row:2;line-height:20px}.plugin-run-reason{grid-column:1/-1;grid-row:3}.plugin-extension{gap:8px}}
</style>
