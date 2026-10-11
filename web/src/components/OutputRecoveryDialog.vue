<script setup lang="ts">
import {ref} from 'vue'
import {ElMessage} from 'element-plus'
import {request,post} from '../api/client'
interface Preview{transferId:string;assetId:string;controlRevision:number;assetRevision:number;state:string;existingPaths:string[];expectedPaths:string[];needsMigration:boolean;needsArtifactRecovery:boolean;ready:boolean;planDigest:string;message:string;noUploadRecreation:boolean;sourceFilesPreserved:boolean}
const visible=ref(false),busy=ref(false),applying=ref(false),error=ref(''),preview=ref<Preview|null>(null),transferId=ref('')
const emit=defineEmits<{completed:[]}>()
async function refresh(){if(!transferId.value||applying.value)return;busy.value=true;try{preview.value=await request<Preview>(`/transfers/${transferId.value}/output-preview`);error.value=''}catch(e){error.value=e instanceof Error?e.message:'输出核对暂不可用'}finally{busy.value=false}}
async function open(id:string){transferId.value=id;preview.value=null;visible.value=true;await refresh()}
async function resume(){if(!preview.value?.ready||preview.value.state!=='needs_output_reconciliation'||applying.value)return;applying.value=true;try{await post(`/transfers/${transferId.value}/resume-import`,{controlRevision:preview.value.controlRevision,planDigest:preview.value.planDigest});ElMessage.success('入库恢复已明确批准；不重新上传、不改播放链接，请核对后续状态');visible.value=false;emit('completed')}catch(e){error.value=e instanceof Error?e.message:'结果未确认；先核对当前状态，不自动重复提交'}finally{applying.value=false}}
defineExpose({open})
</script>
<template>
<el-dialog v-model="visible" append-to-body top="6vh" title="核对并恢复已有入库" width="min(740px,95vw)" :close-on-click-modal="false">
 <el-alert title="只恢复已有已校验文件的入库任务；不创建上传会话、不覆盖已确认身份、不重写STRM固定链接。旧布局必须先在云盘索引明确迁移。" type="warning" :closable="false"/>
 <el-alert v-if="error" :title="error" type="error" :closable="false"/>
 <div v-if="preview">
  <p class="recovery-message" role="status">{{preview.message}}</p>
  <p class="field-hint recovery-id">资产ID：{{preview.assetId}} · 传输修订：{{preview.controlRevision}}</p>
  <div class="recovery-paths" aria-label="已有入库输出核对"><section><h3>已有输出</h3><code v-for="path in preview.existingPaths" :key="path">{{path}}</code></section><section><h3>应有布局</h3><code v-for="path in preview.expectedPaths" :key="path">{{path}}</code></section></div>
  <RouterLink v-if="preview.needsMigration||preview.needsArtifactRecovery" :to="{path:'/library',query:{assetId:preview.assetId}}" class="recovery-link" @click="visible=false">前往云盘索引核对该资产、恢复已有输出并明确预览迁移</RouterLink>
  <p class="field-hint">核对只读，不会迁移文件。预览后文件、授权、绑定或修订变化会阻止恢复；请求结果不确定时先“重新核对”，不自动重建任务。</p>
 </div>
 <template #footer><el-button :loading="busy" :disabled="applying" @click="refresh">重新核对</el-button><el-button :disabled="applying" @click="visible=false">关闭</el-button><el-button type="primary" :loading="applying" :disabled="!preview?.ready||preview?.state!=='needs_output_reconciliation'" @click="resume">明确恢复入库（不重新上传）</el-button></template>
</el-dialog>
</template>
<style scoped>.recovery-message{color:#263b45;line-height:1.7;margin-bottom:12px}.recovery-id{overflow-wrap:anywhere}.recovery-paths{max-height:32vh;overflow:auto;display:grid;gap:14px;margin:16px 0}.recovery-paths h3{font-size:14px;margin-bottom:6px}.recovery-paths code{display:block;overflow-wrap:anywhere;line-height:1.7;padding:4px 0}.recovery-link{display:block;color:#176d78;text-decoration:underline;margin:12px 0;line-height:1.7}@media(max-width:500px){:deep(.el-dialog__footer){display:flex;flex-wrap:wrap;justify-content:flex-end;gap:8px}}</style>
