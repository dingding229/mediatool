<script setup lang="ts">
import {ref,onMounted,watch} from 'vue'
import {useRoute} from 'vue-router'
import {request,post} from '../api/client'
import StatusBadge from '../components/StatusBadge.vue'
import {kindLabels,stepState} from '../files/workbench'
import type {FilePlan} from '../files/types'
const route=useRoute()
interface Entry{id:string;kind:string;state:string;source:string;connectionId:string;batchId:string;message:string;createdAt:string;updatedAt:string;details:{plan?:FilePlan;completedSteps?:number;items?:{ordinal:number;source:string;state:string;message:string;operationId:string}[];phase?:string;bytesTransferred?:number;size?:number;taskId?:string;paths?:{path:string;visible:boolean;reason:string;itemId:string}[];[key:string]:unknown}}
const items=ref<Entry[]>([]),page=ref(1),total=ref(0),state=ref(''),kind=ref(String(route.query.kind??'')),query=ref(''),batchId=ref(String(route.query.batchId??'')),busy=ref(false),error=ref(''),detail=ref<Entry|null>(null),dialog=ref(false),reconciling=ref(false),dates=ref<[string,string]|null>(null)
let epoch=0
async function load(){const serial=++epoch;busy.value=true;error.value='';try{const q=new URLSearchParams({page:String(page.value),pageSize:'25',state:state.value,kind:kind.value,q:query.value,batchId:batchId.value});if(dates.value){q.set('from',new Date(dates.value[0]).toISOString());q.set('to',new Date(dates.value[1]).toISOString())}const out=await request<{items:Entry[];total:number}>(`/library/activity?${q}`);if(serial===epoch){items.value=out.items;total.value=out.total}}catch(e){if(serial===epoch)error.value=e instanceof Error?e.message:'历史读取失败'}finally{if(serial===epoch)busy.value=false}}
function inspect(item:Entry){detail.value=item;dialog.value=true}
async function reconcile(){if(!detail.value||!['batch','file'].includes(detail.value.kind))return;reconciling.value=true;error.value='';try{await post(detail.value.kind==='batch'?`/files/batches/${detail.value.id}/reconcile`:`/files/operations/${detail.value.id}/reconcile`,{});dialog.value=false;await load()}catch(e){error.value=e instanceof Error?e.message:'核对失败'}finally{reconciling.value=false}}
function apply(){page.value=1;void load()}
watch(()=>[route.query.kind,route.query.batchId],()=>{kind.value=String(route.query.kind??'');batchId.value=String(route.query.batchId??'');apply()})
onMounted(load)
</script>
<template>
  <section class="page-heading"><div><span class="page-kicker">ORGANIZE / 历史与恢复</span><h1>文件与入库历史</h1><p>按批次追踪扫描、整理、上传、输出及 Emby 确认。进度、冲突和未知结果独立展示。</p></div><el-button :loading="busy" @click="load">刷新历史</el-button></section>
  <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  <section class="panel">
    <el-form label-position="top" class="history-filters" @submit.prevent="apply">
      <el-form-item label="阶段"><el-select v-model="kind" aria-label="历史阶段" @change="apply"><el-option label="全部阶段" value=""/><el-option v-for="(label,value) in kindLabels" :key="value" :label="label" :value="value"/></el-select></el-form-item>
      <el-form-item label="状态"><el-select v-model="state" aria-label="历史状态" @change="apply"><el-option label="全部状态" value=""/><el-option v-for="value in ['pending','running','succeeded','partial','needs_reconcile','failed','paused','cancelled','verified','imported','visible','conflict','applied']" :key="value" :value="value" :label="value"/></el-select></el-form-item>
      <el-form-item label="媒体 / 来源关键词"><el-input v-model="query" aria-label="历史关键词" clearable @keyup.enter="apply"/></el-form-item>
      <el-form-item label="批次ID"><el-input v-model="batchId" aria-label="历史批次" clearable @keyup.enter="apply"/></el-form-item>
      <el-form-item label="起止时间"><el-date-picker v-model="dates" type="datetimerange" start-placeholder="开始" end-placeholder="结束" @change="apply"/></el-form-item>
      <el-button :loading="busy" @click="apply">筛选历史</el-button>
    </el-form>
    <div v-loading="busy" class="history-list">
      <article v-for="item in items" :key="`${item.kind}:${item.id}`" class="history-row">
        <div class="history-head"><strong>{{kindLabels[item.kind]}}</strong><StatusBadge :state="item.state"/><time>{{new Date(item.updatedAt).toLocaleString()}}</time></div>
        <code v-if="item.source">{{item.source}}</code><p v-if="item.batchId" class="field-hint">批次 {{item.batchId}}</p>
        <div v-for="(entry,index) in item.details.plan?.items??[]" :key="index" class="history-path"><code>{{entry.source.path}}</code><span>→</span><code>{{entry.target}}</code></div>
        <p class="field-hint">{{item.message||'查看步骤证据与恢复入口'}}</p><el-button @click="inspect(item)">查看步骤详情</el-button>
      </article>
      <el-empty v-if="!busy&&!items.length" description="此范围暂无实际记录"/>
    </div>
    <el-pagination v-model:current-page="page" :page-size="25" :total="total" layout="prev,pager,next,total" @current-change="load"/>
  </section>
  <el-dialog v-model="dialog" title="步骤证据与恢复" width="min(850px,95vw)" top="5vh" class="history-detail">
    <template v-if="detail"><div class="history-head"><strong>{{kindLabels[detail.kind]}}</strong><StatusBadge :state="detail.state"/></div><p class="field-hint">{{detail.message}}</p>
      <ol v-if="detail.details.plan" class="step-list"><li v-for="(step,index) in detail.details.plan.steps" :key="index"><StatusBadge :state="stepState(detail.state,detail.details.completedSteps??0,index)"/><strong>{{step.action==='mkdir'?'创建目录':'移动 / 重命名'}}</strong><code>{{step.source.path||'—'}} → {{step.target}}</code></li></ol>
      <ol v-if="detail.details.items" class="step-list"><li v-for="row in detail.details.items" :key="row.ordinal"><StatusBadge :state="row.state"/><code>{{row.source}}</code><span>{{row.message}}</span></li></ol>
      <div v-if="detail.kind==='transfer'" class="phase-note"><strong>当前阶段：{{detail.details.phase}}</strong><p>{{detail.details.bytesTransferred}} / {{detail.details.size}} 字节已传输；源SHA1 {{detail.details.sourceSha1||'未知'}}。上传100%不代表云端校验、输出完成或Emby可见。</p><RouterLink to="/downloads">进入传输任务处理暂停 / 恢复 / 入库批准</RouterLink></div>
      <div v-for="entry in detail.details.paths??[]" :key="entry.path" class="history-path"><code>{{entry.path}}</code><span>{{entry.visible?'可见':'待确认'}}</span><span>{{entry.reason}}</span></div>
      <pre class="evidence-json">{{JSON.stringify(detail.details,null,2)}}</pre>
      <RouterLink v-if="detail.kind==='output'" to="/cloudStorage">进入输出修复与归档批准</RouterLink><RouterLink v-if="detail.kind==='scan'" to="/cloudStorage">进入扫描控制</RouterLink>
    </template>
    <template #footer><el-button @click="dialog=false">关闭</el-button><el-button v-if="detail&&['batch','file'].includes(detail.kind)&&['running','needs_reconcile'].includes(detail.state)" :loading="reconciling" @click="reconcile">核对结果（不重放）</el-button></template>
  </el-dialog>
</template>
<style scoped>
.history-filters{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:12px;align-items:end}.history-filters>.el-form-item:nth-child(5){grid-column:span 2}.history-filters :deep(.el-date-editor){width:100%;max-width:100%}.history-filters>.el-button{min-height:44px;align-self:center}.history-row{padding:24px 0;border-bottom:1px solid var(--border)}.history-head{display:flex;flex-wrap:wrap;gap:16px;align-items:center;margin-bottom:16px}.history-head time{color:var(--muted);font-size:12px;margin-left:auto}.history-path{display:grid;grid-template-columns:1fr 24px 1fr;gap:12px;margin:10px 0}code{line-height:1.7;overflow-wrap:anywhere;white-space:normal}.el-pagination{margin-top:24px;overflow:auto}.step-list{padding:0;list-style:none}.step-list li{display:flex;flex-wrap:wrap;gap:12px;border-bottom:1px solid var(--border);padding:16px 0}.step-list code{flex-basis:100%}.evidence-json{font-size:13px;line-height:1.7;white-space:pre-wrap;overflow-wrap:anywhere;background:var(--surface-soft);border:1px solid var(--border);padding:16px;max-height:45vh;overflow:auto}.phase-note{border-left:3px solid var(--el-color-primary);padding:8px 16px}:global(.history-detail){max-height:90dvh;display:flex;flex-direction:column}:global(.history-detail .el-dialog__body){overflow:auto;min-height:0}@media(max-width:680px){.history-filters,.history-path{grid-template-columns:1fr}.history-filters>.el-form-item:nth-child(5){grid-column:span 1}.history-head time{margin-left:0}.history-path>span:nth-child(2){width:auto}}
</style>
