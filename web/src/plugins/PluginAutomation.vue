<script setup lang="ts">
import{ref,onMounted}from'vue'
import{ElMessageBox}from'element-plus'
import{workflows,workflowDefinition,workflowRuns,workflowOperation}from'../api/plugins'
import type{WorkflowSummary,WorkflowDefinition,WorkflowRunView}from'../api/plugins'
import{requestUUID}from'../utils/request-uuid'
const list=ref<WorkflowSummary[]>([]),page=ref(1),selected=ref<WorkflowDefinition|null>(null),runs=ref<WorkflowRunView[]>([]),runPage=ref(1),busy=ref(false),error=ref(''),notice=ref('')
const dag=ref('{"id":"example","steps":[]}'),pending=new Map<string,string>()
async function load(){list.value=await workflows(page.value)}
onMounted(async()=>{try{await load()}catch(e){error.value=String(e)}})
async function open(id:string){error.value='';try{selected.value=await workflowDefinition(id);dag.value=JSON.stringify(selected.value.dag,null,2);runPage.value=1;await refreshRuns()}catch(e){error.value=String(e)}}
async function refreshRuns(){if(selected.value)runs.value=await workflowRuns(selected.value.dag.id,runPage.value)}
async function move(delta:number){page.value=Math.max(1,page.value+delta);try{await load()}catch(e){error.value=String(e)}}
async function historyMove(delta:number){runPage.value=Math.max(1,runPage.value+delta);try{await refreshRuns()}catch(e){error.value=String(e)}}
async function perform(operation:string,runId=''){if(busy.value)return;error.value='';notice.value='';let graph:unknown
 if(operation==='save')try{graph=JSON.parse(dag.value);if(new TextEncoder().encode(dag.value).length>16384)throw new Error('DAG超过16KiB')}catch(e){error.value=String(e);return}
 if(operation!=='save'&&!selected.value)return
 try{await ElMessageBox.confirm('执行工作流 '+operation+'？修改DAG将清除批准；运行前会重新检查实例与scope，unknown不自动重放。','确认工作流操作',{type:'warning',customClass:'plugin-confirm-dialog',confirmButtonText:'确认操作',cancelButtonText:'取消'})}catch{return}
 busy.value=true;try{const input={operation,workflowId:selected.value?.dag.id,runId,fingerprint:selected.value?.fingerprint,revision:selected.value?.revision??0,dag:graph,confirm:true};const identity=JSON.stringify(input);let key=pending.get(identity);if(!key){key=requestUUID();pending.set(identity,key)};await workflowOperation({...input,operationKey:key});pending.delete(identity);notice.value=operation==='run'?'已受理，尚非完成；请刷新运行状态':'操作已完成';await load();if(selected.value)await open(selected.value.dag.id)}catch(e){error.value=e instanceof Error?e.message:'操作结果未知，请核对'}finally{busy.value=false}}
</script>
<template>
 <p>最小工作流宿主：最多32步、深度16，禁止成环。不提供可视化编排器、LLM或自动Agent。所有变更需二次确认；不回退业务副作用。</p>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><el-alert v-if="notice" :title="notice" type="info" :closable="false" role="status"/>
 <el-table :data="list" empty-text="尚无工作流"><el-table-column prop="id" label="工作流" min-width="130"/><el-table-column label="DAG批准状态" min-width="180"><template #default="{row}"><el-tag :type="row.approved===row.fingerprint?'success':'warning'">{{row.approved===row.fingerprint?'已批准':'待重新批准'}}</el-tag></template></el-table-column><el-table-column prop="steps" label="步骤" width="70"/><el-table-column label="查看" width="90"><template #default="{row}"><el-button @click="open(row.id)">详情</el-button></template></el-table-column></el-table>
 <nav aria-label="工作流分页"><el-button :disabled="page===1||busy" @click="move(-1)">上一页</el-button><span>第{{page}}页</span><el-button :disabled="list.length<20||busy" @click="move(1)">下一页</el-button></nav>
 <p v-if="selected">修订 {{selected.revision}} · DAG指纹 <code>{{selected.fingerprint}}</code><br>批准指纹 <code>{{selected.approved||'未批准'}}</code></p>
 <el-button :disabled="busy" @click="selected=null;dag='{&quot;id&quot;:&quot;example&quot;,&quot;steps&quot;:[]}'">新建工作流</el-button>
 <label for="workflow-schema">DAG JSON 表单</label><el-input id="workflow-schema" v-model="dag" type="textarea" :rows="8" :maxlength="16384"/>
 <details><summary>DAG schema 提示</summary><p>id：1–64字母/数字/_/-；steps：1–32项，每步含id、instanceId、action、dependsOn数组和input.data对象。保存后清除原批准；运行前仍校验动作scope与当前实例代际。</p></details>
 <nav><el-button :loading="busy" @click="perform('save')">保存DAG</el-button><el-button :disabled="!selected||busy" type="warning" @click="perform('approve')">批准当前指纹</el-button><el-button :disabled="!selected||selected.approved!==selected.fingerprint||busy" type="danger" plain @click="perform('run')">手动运行工作流</el-button><el-button :disabled="!selected||busy" @click="refreshRuns().catch(e=>error=String(e))">刷新运行状态</el-button></nav>
 <article v-for="r in runs" :key="r.id"><h3>{{r.state}} · {{r.id}}</h3><p v-if="['unknown','failed','canceled'].includes(r.state)" role="status">结果未知或失败不会自动重放，请核对运行记录。</p><ul><li v-for="(s,id) in r.steps" :key="id">{{id}}：{{s}} <span v-if="r.reasons[id]">（{{r.reasons[id]}}）</span></li></ul><el-button type="danger" plain :disabled="busy||!['pending','running'].includes(r.state)" @click="perform('cancel',r.id)">取消工作流</el-button></article>
 <nav aria-label="工作流历史分页"><el-button :disabled="runPage===1||busy" @click="historyMove(-1)">上一页运行</el-button><span>第{{runPage}}页</span><el-button :disabled="runs.length<20||busy" @click="historyMove(1)">下一页运行</el-button></nav>
</template>
<style scoped>p,code,li,h3{overflow-wrap:anywhere;line-height:1.6}nav{display:flex;flex-wrap:wrap;gap:8px;align-items:center;margin-block:14px}.el-button,summary{min-height:44px;margin-left:0}label{display:block;margin:12px 0}article{border-top:1px solid var(--el-border-color);margin-top:16px;padding-top:12px}.el-table{width:100%}summary{cursor:pointer}</style>
