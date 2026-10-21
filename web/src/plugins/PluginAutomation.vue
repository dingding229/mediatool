<script setup lang="ts">
import{ref,onMounted}from'vue'
import ReadFeedback from './ReadFeedback.vue'
import {useReadState} from './read-state'
import{ElMessageBox}from'element-plus'
import{workflows,workflowDefinition,workflowRuns,workflowOperation}from'../api/plugins'
import type{WorkflowSummary,WorkflowDefinition,WorkflowRunView}from'../api/plugins'
import{requestUUID}from'../utils/request-uuid'
const list=ref<WorkflowSummary[]>([]),page=ref(1),selected=ref<WorkflowDefinition|null>(null),runs=ref<WorkflowRunView[]>([]),runPage=ref(1),busy=ref(false),error=ref(''),notice=ref('')
const dag=ref('{"id":"example","steps":[]}'),pending=new Map<string,string>()
const listRead=useReadState(),detailRead=useReadState(false),runsRead=useReadState(false)
async function load(){await listRead.run(()=>workflows(page.value),value=>list.value=value)}
onMounted(load)
const detailID=ref('')
async function retryDetail(){if(detailID.value)await open(detailID.value)}
async function open(id:string){detailID.value=id;selected.value=null;runs.value=[];runsRead.reset();error.value=''
 const ok=await detailRead.run(()=>workflowDefinition(id),value=>{selected.value=value;dag.value=JSON.stringify(value.dag,null,2);runPage.value=1})
 if(ok)await refreshRuns()
}
async function refreshRuns(){if(!selected.value)return;const id=selected.value.dag.id;await runsRead.run(()=>workflowRuns(id,runPage.value),value=>runs.value=value)}
async function move(delta:number){page.value=Math.max(1,page.value+delta);await load()}
async function historyMove(delta:number){runPage.value=Math.max(1,runPage.value+delta);await refreshRuns()}
function newWorkflow(){detailRead.reset();runsRead.reset();selected.value=null;runs.value=[];dag.value='{"id":"example","steps":[]}'}
async function perform(operation:string,runId=''){if(busy.value)return;error.value='';notice.value='';let graph:unknown
 if(operation==='save')try{graph=JSON.parse(dag.value);if(new TextEncoder().encode(dag.value).length>16384)throw new Error('DAG超过16KiB')}catch(e){error.value=String(e);return}
 if(operation!=='save'&&!selected.value)return
 try{await ElMessageBox.confirm('执行工作流 '+operation+'？修改DAG将清除批准；运行前会重新检查实例与scope，unknown不自动重放。','确认工作流操作',{type:'warning',customClass:'plugin-confirm-dialog',confirmButtonText:'确认操作',cancelButtonText:'取消'})}catch{return}
 busy.value=true;try{const input={operation,workflowId:selected.value?.dag.id,runId,fingerprint:selected.value?.fingerprint,revision:selected.value?.revision??0,dag:graph,confirm:true};const identity=JSON.stringify(input);let key=pending.get(identity);if(!key){key=requestUUID();pending.set(identity,key)};await workflowOperation({...input,operationKey:key});pending.delete(identity);notice.value=operation==='run'?'已受理，尚非完成；请刷新运行状态':'操作已完成';await load();if(selected.value)await open(selected.value.dag.id)}catch(e){error.value=e instanceof Error?e.message:'操作结果未知，请核对'}finally{busy.value=false}}
</script>
<template>
 <p>安排插件按顺序协作，每次最多32步。保存、批准和运行分别确认，失败时不会自动重做。</p>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><el-alert v-if="notice" :title="notice" type="info" :closable="false" role="status"/>
 <ReadFeedback :state="listRead" caption="工作流列表" @retry="load"/><el-table v-if="listRead.loaded" :aria-busy="listRead.loading" :data="list" empty-text="尚无工作流"><el-table-column label="工作流" min-width="130"><template #default="{$index}">工作流 {{(page-1)*20+$index+1}}</template></el-table-column><el-table-column label="审批状态" min-width="180"><template #default="{row}"><el-tag :type="row.approved===row.fingerprint?'success':'warning'">{{row.approved===row.fingerprint?'已批准':'待重新批准'}}</el-tag></template></el-table-column><el-table-column prop="steps" label="步骤" width="70"/><el-table-column label="查看" width="90"><template #default="{row}"><el-button @click="open(row.id)">详情</el-button></template></el-table-column></el-table>
 <PagePagination v-if="listRead.loaded" :current-page="page" :has-next="list.length>=20" :disabled="busy||listRead.loading" @current-change="(n:number)=>move(n-page)"/>
 <ReadFeedback v-if="detailRead.loading||detailRead.error" :state="detailRead" caption="工作流详情" @retry="retryDetail"/><details v-if="selected"><summary>技术详情</summary><pre class="technical-json">{{selected}}</pre></details>
 <el-button :disabled="busy" @click="newWorkflow">新建工作流</el-button>
 <label for="workflow-schema">工作流定义（JSON）</label><el-input id="workflow-schema" v-model="dag" type="textarea" :rows="8" :maxlength="16384"/>
 <details><summary>技术详情：定义格式</summary><p>id：1–64字母/数字/_/-；steps：1–32项，每步含id、instanceId、action、dependsOn数组和input.data对象。保存后清除原批准；运行前仍校验动作scope与当前实例代际。</p></details>
 <nav><el-button :loading="busy" @click="perform('save')" type="primary" class="mt-save-button">保存工作流</el-button><el-button :disabled="!selected||busy" type="warning" @click="perform('approve')">批准当前版本</el-button><el-button :disabled="!selected||selected.approved!==selected.fingerprint||busy" type="danger" plain @click="perform('run')">手动运行工作流</el-button><el-button :disabled="!selected||busy" @click="refreshRuns">核对运行状态</el-button></nav>
 <ReadFeedback v-if="selected" :state="runsRead" caption="工作流运行记录" @retry="refreshRuns"/><p v-if="selected&&runsRead.loaded&&!runs.length">暂无工作流运行记录</p><article v-for="r in runs" :key="r.id"><h3>{{({pending:'等待运行',running:'运行中',succeeded:'已完成',failed:'失败',canceled:'已取消',unknown:'待核对'} as Record<string,string>)[r.state]||'待核对'}}</h3><details><summary>技术详情</summary><pre class="technical-json">{{r}}</pre></details><p v-if="['unknown','failed','canceled'].includes(r.state)" role="status">结果未知或失败不会自动重放，请核对运行记录。</p><ul><li v-for="(s,id) in r.steps" :key="id">{{id}}：{{s}} <span v-if="r.reasons[id]">（{{r.reasons[id]}}）</span></li></ul><el-button type="danger" plain :disabled="busy||!['pending','running'].includes(r.state)" @click="perform('cancel',r.id)">取消工作流</el-button></article>
 <PagePagination :current-page="runPage" :has-next="runs.length>=20" :disabled="busy" @current-change="(n:number)=>historyMove(n-runPage)"/>
</template>
<style scoped>p,code,li,h3{overflow-wrap:anywhere;line-height:1.6}nav{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin-block:14px}.el-button,summary{min-height:44px;margin-left:0}label{display:block;margin:12px 0}article{border-top:1px solid var(--el-border-color);margin-top:16px;padding-top:12px}.el-table{width:100%}summary{cursor:pointer}</style>
