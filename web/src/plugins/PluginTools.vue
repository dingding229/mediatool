<script setup lang="ts">
import {ref,onMounted} from 'vue'
import ReadFeedback from './ReadFeedback.vue'
import {useReadState} from './read-state'
import {ElMessageBox} from 'element-plus'
import {tools,callTool,dangerousScope} from '../api/plugins'
import type{ToolDescriptor}from'../api/plugins'
import{requestUUID}from'../utils/request-uuid'
const props=defineProps<{instanceId:string}>()
const list=ref<ToolDescriptor[]>([]),selected=ref(''),input=ref('{}'),output=ref(''),error=ref(''),busy=ref(false)
const pending=new Map<string,string>()
const listRead=useReadState()
async function load(){await listRead.run(()=>tools(props.instanceId),value=>{list.value=value;selected.value=value[0]?.id??''})}
onMounted(load)
async function run(){const d=list.value.find(x=>x.id===selected.value);if(!d||busy.value)return;error.value='';output.value=''
 let value:unknown;try{if(new TextEncoder().encode(input.value).length>16384)throw new Error('输入超过16KiB');value=JSON.parse(input.value)}catch(e){error.value=String(e);return}
 if(d.effectKind!=='read')try{await ElMessageBox.confirm('将人工调用 '+d.name+'（'+d.effectKind+'）。仅按已确认scope执行，unknown不自动重放。','确认工具调用',{type:'warning',customClass:'plugin-confirm-dialog',confirmButtonText:'确认调用',cancelButtonText:'取消'})}catch{return}
 busy.value=true;try{const identity=d.id+':'+JSON.stringify(value);let key=pending.get(identity);if(!key){key=requestUUID();pending.set(identity,key)};const result=await callTool(props.instanceId,{name:d.id,input:value,operationKey:key,confirm:d.effectKind!=='read'});pending.delete(identity);output.value=JSON.stringify(result,null,2)}catch(e){error.value=e instanceof Error?e.message:'结果未知，请核对运行历史'}finally{busy.value=false}}
</script>
<template>
 <p>仅管理员可手动调用工具。调用前会检查权限，可能修改数据的操作需要再次确认。</p>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><ReadFeedback :state="listRead" caption="工具列表" @retry="load"/><p v-if="listRead.loaded&&!list.length">没有已授权且运行中的工具</p>
 <label for="plugin-tool-select">工具</label><el-select id="plugin-tool-select" v-model="selected" aria-label="工具"><el-option v-for="t in list" :key="t.id" :value="t.id" :label="t.name"/></el-select>
 <template v-for="d in list.filter(x=>x.id===selected)" :key="d.id"><p>{{d.description}} <el-tag :type="d.effectKind==='read'?'info':'danger'">{{d.effectKind==='read'?'只读':'危险确认'}} · {{({read:'只读',write:'会修改数据',unknown:'影响未确认'} as Record<string,string>)[d.effectKind]||'影响未确认'}}</el-tag></p><p><el-tag v-for="s in d.requiredScopes" :key="s" :type="dangerousScope(s)?'danger':'info'">{{s}}</el-tag></p><details><summary>技术详情：输入格式</summary><pre>{{JSON.stringify(d.inputSchema,null,2)}}</pre></details></template>
 <label for="plugin-tool-input">输入 JSON</label><el-input id="plugin-tool-input" v-model="input" type="textarea" :rows="6" :maxlength="16384"/><el-button :disabled="!selected||busy||listRead.loading||!listRead.loaded" :loading="busy" @click="run">人工调用工具</el-button>
 <h3>输出</h3><pre aria-live="polite">{{output||'尚无结果（不代表执行成功）'}}</pre>
</template>
<style scoped>p{line-height:1.7;overflow-wrap:anywhere}label{display:block;margin:12px 0 6px}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:320px;overflow:auto}.el-select{width:100%}.el-button,summary{min-height:44px;margin-top:12px}.el-tag{margin:4px;max-width:100%;height:auto;white-space:normal}summary{cursor:pointer} </style>
