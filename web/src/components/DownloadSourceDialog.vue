<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { request, post } from '../api/client'
interface Preview{taskId:string;resourceId:string;searchRunId:string;currentSearchRunId:string;controlRevision:number;planDigest:string;mode:string;originalHash:string;candidateHash:string;candidateTitle:string;downloaderObservation:string;eligible:boolean;reasons:string[]}
const visible=ref(false),taskId=ref(''),searchRunId=ref(''),preview=ref<Preview|null>(null),busy=ref(false),applying=ref(false),error=ref(''),confirmed=ref(false),initialConfirmed=ref(false)
const emit=defineEmits<{completed:[]}>()
async function refresh(){if(applying.value||!taskId.value)return;busy.value=true;preview.value=null;confirmed.value=false;initialConfirmed.value=false;try{const suffix=searchRunId.value.trim()?`?searchRunId=${encodeURIComponent(searchRunId.value.trim())}`:'';preview.value=await request<Preview>(`/downloads/${taskId.value}/source-preview${suffix}`);error.value=''}catch(e){error.value=e instanceof Error?e.message:'来源预览暂不可用'}finally{busy.value=false}}
async function open(id:string){taskId.value=id;searchRunId.value='';preview.value=null;error.value='';visible.value=true;await refresh()}
function changed(){preview.value=null;confirmed.value=false;initialConfirmed.value=false}
async function approve(){const p=preview.value;if(!p?.eligible||!confirmed.value||applying.value||(p.mode==='initial_identity'&&!initialConfirmed.value))return;applying.value=true;try{const result=await post<{alreadyApplied:boolean;recoveryStillRequired:boolean}>(`/downloads/${taskId.value}/reconfirm-source`,{searchRunId:p.searchRunId,controlRevision:p.controlRevision,planDigest:p.planDigest,confirmSourceBinding:true,confirmInitialIdentity:initialConfirmed.value});if(result.recoveryStillRequired){error.value='此确认已应用，但任务目前又需要来源核对。请重新预览，不重复提交旧确认。';emit('completed');return}ElMessage.success(result.alreadyApplied?'此前确认已应用，未重复排队':'原任务来源已确认；请查看下载器核对和后续状态');visible.value=false;emit('completed')}catch(e){error.value=e instanceof Error?e.message:'提交结果未确认。先重新核对，不自动重复提交。'}finally{applying.value=false}}
defineExpose({open})
</script>
<template>
 <el-dialog v-model="visible" append-to-body top="5vh" title="重新确认下载来源" width="min(760px,95vw)" :close-on-click-modal="false" :close-on-press-escape="!applying" :show-close="!applying">
  <el-alert title="只恢复原下载任务。媒体身份、下载器和保存路径不变；不删除种子、不移动做种文件，也不创建另一份下载任务。" type="warning" :closable="false"/>
  <p class="field-hint">先在资源搜索中取得当前来源，再核对 InfoHash。预览会读取 PT 种子元数据和下载器状态，不向下载器提交种子。</p>
  <label class="source-label" for="source-search-run">搜索记录 ID <span>可选，留空使用最近可用快照</span></label>
  <el-input id="source-search-run" v-model="searchRunId" :disabled="applying" placeholder="可从资源搜索记录选择当前快照" @input="changed"/>
  <el-alert v-if="error" class="source-gap" :title="error" type="error" :closable="false" role="alert"/>
  <div v-if="preview" class="source-proof" aria-label="下载来源身份核对">
   <p class="source-title">{{preview.candidateTitle||'当前任务尚不能确认来源'}}</p>
   <dl><div><dt>原 InfoHash</dt><dd><code>{{preview.originalHash||'尚无已确认的种子身份'}}</code></dd></div><div><dt>新来源 InfoHash</dt><dd><code>{{preview.candidateHash||'尚未取得'}}</code></dd></div><div><dt>下载器观察</dt><dd>{{preview.downloaderObservation==='present'?'已有同 Hash 任务，将先核对而不是重复添加':preview.downloaderObservation==='absent'?'当前未发现该 Hash 任务':'尚未确认'}}</dd></div></dl>
   <el-alert v-if="preview.reasons.length" :title="preview.reasons.join('；')" type="warning" :closable="false" role="alert"/>
   <div v-if="preview.eligible" class="source-confirmations"><label class="source-consent"><input v-model="confirmed" type="checkbox" :disabled="applying"><span>我确认原媒体与任务不变，并批准使用此新来源</span></label><label v-if="preview.mode==='initial_identity'" class="source-consent"><input v-model="initialConfirmed" type="checkbox" :disabled="applying"><span>原任务尚无 InfoHash，我明确确认其首次种子身份</span></label></div>
  </div>
  <template #footer><el-button :loading="busy" :disabled="applying" @click="refresh">重新预览来源</el-button><el-button :disabled="applying" @click="visible=false">关闭</el-button><el-button type="primary" :loading="applying" :disabled="!preview?.eligible||!confirmed||(preview.mode==='initial_identity'&&!initialConfirmed)||busy" @click="approve">明确确认并恢复原任务</el-button></template>
 </el-dialog>
</template>
<style scoped>
.source-label{display:block;font-weight:600;margin:16px 0 7px}.source-label span{font-size:12px;font-weight:400;color:#667d85}.source-gap{margin-top:14px}.source-proof{margin-top:18px;max-height:37vh;overflow:auto}.source-title{font-weight:600;line-height:1.6}.source-proof dl{display:grid;gap:12px;margin:12px 0}.source-proof dt{color:#667d85;font-size:12px;margin-bottom:4px}.source-proof dd{margin:0;line-height:1.6}.source-proof code{overflow-wrap:anywhere}.source-confirmations{display:grid;gap:10px;margin-top:16px}.source-consent{display:flex;align-items:flex-start;gap:10px;line-height:1.6;cursor:pointer}.source-consent input{width:20px;height:20px;flex-shrink:0;margin:2px 0 0;accent-color:#176d78}.source-consent input:focus-visible{outline:2px solid #176d78;outline-offset:3px}@media(max-width:500px){:deep(.el-dialog__footer){display:flex;flex-wrap:wrap;justify-content:flex-end;gap:8px}:deep(.el-dialog__footer .el-button){margin:0}}
</style>
