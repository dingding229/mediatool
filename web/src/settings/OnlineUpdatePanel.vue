<script setup lang="ts">
import {computed,ref,onUnmounted} from 'vue';
import ReleaseNotes from './ReleaseNotes.vue';
import IdleUpdatePanel from './IdleUpdatePanel.vue';
import {useUpdateStore} from '../stores/updates';
import {getOnlineUpdate,executeOnlineUpdate,confirmOnlineUpdateRecovery,type OnlineUpdateStatus} from '../api/updates';
import {useRefresh} from '../composables/useRefresh';
import {usePageRefresh} from '../ui/page-refresh';
import {date,errorText} from '../workflows/readiness';
import {newerStable,updateDone,updatePhases,updateFailures} from './online-update';
const props=defineProps<{checkSummary?:string}>();
const updates=useUpdateStore(),status=ref<OnlineUpdateStatus>(),error=ref(''),open=ref(false),detailsOpen=ref(false),recoveryId=ref(''),password=ref(''),confirmed=ref(false),busy=ref(false);
const target=computed(()=>updates.state?.result.status==='verified'?updates.state.result.latest:null);
const running=computed(()=>Boolean(status.value?.job&&!updateDone(status.value.job.phase)));
const ready=computed(()=>!(status.value?.recoveryRequired?.length)&&status.value?.available&&status.value.policyVerified&&!running.value&&target.value&&newerStable(updates.build?.version||'',target.value.version));
const result=computed(()=>{const job=status.value?.job;return[props.checkSummary,job?'最近更新：'+(job.failure?updateFailures[job.failure]||'更新未完成，请查看详情':updatePhases[job.phase]||'等待核对状态'):status.value?'尚无在线更新记录':'读取更新状态中',status.value&&!status.value.available?status.value.reason:''].filter(Boolean).join(' · ')});
async function load(){try{status.value=await getOnlineUpdate();error.value=''}catch(e){error.value=errorText(e)}}
const refresh=useRefresh(load,{interval:()=>running.value?3000:0});usePageRefresh(refresh.refresh,'about');
function prepare(id=''){recoveryId.value=id;error.value='';password.value='';confirmed.value=false;open.value=true}
async function execute(){if(!confirmed.value||!password.value||busy.value||(!recoveryId.value&&(!ready.value||!target.value)))return;busy.value=true;error.value='';const secret=password.value;password.value='';try{if(recoveryId.value)await confirmOnlineUpdateRecovery(recoveryId.value,secret,true);else{const job=await executeOnlineUpdate(target.value!.version,target.value!.digest,secret,true);if(status.value)status.value.job=job}open.value=false;await refresh.refresh()}catch(e){await refresh.refresh();error.value=status.value&&!status.value.available?status.value.reason:errorText(e)}finally{busy.value=false}}
onUnmounted(()=>{password.value=''});
</script>
<template>
 <section class="f8-online" aria-label="在线更新">
  <div class="media-actions"><el-button type="primary" :disabled="!ready" @click="prepare()">一键更新</el-button><el-button @click="detailsOpen=true">更新详情</el-button></div>
  <IdleUpdatePanel/>
  <p class="f8-job-status" role="status">{{result}}</p><p v-if="error&&!open" role="alert">{{error}}</p>
  <el-dialog v-model="detailsOpen" title="更新详情" width="min(680px,94vw)" :close-on-click-modal="false" class="media-dialog">
   <h3>环境与风险</h3><p v-if="status?.socketMounted" class="f8-risk">已挂载 docker.sock，app 拥有宿主机级权限。白名单仅限制正常代码路径，不能隔离被攻陷的应用。</p><p v-else>未挂载 docker.sock，一键更新不可用；仍可检查最新版本。</p>
   <p v-if="status&&!status.available">{{status.reason}}</p><p>更新会短暂中断播放；仅实际 schema 变化时才可能使用已验证备份恢复数据库，备份后产生的业务变更可能丢失。</p><p>{{status?.policyVerified?'Docker API 白名单校验已通过':'Docker API 白名单尚未通过校验'}}</p>
   <p v-for="id in status?.recoveryRequired||[]" :key="id">任务 {{id}} 尚待人工恢复确认。<el-button :disabled="busy||running" @click="prepare(id)">任务已完成人工确认</el-button></p>
   <h3>最近更新记录</h3><template v-if="status?.job"><dl class="f8-job"><div><dt>执行管理员</dt><dd>{{status.job.actor}}</dd></div><div><dt>版本</dt><dd>{{status.job.fromVersion}} → {{status.job.target.version}}</dd></div><div><dt>最近状态</dt><dd>{{date(status.job.updatedAt)}}</dd></div><div><dt>备份目录</dt><dd>{{status.job.backup||'尚未生成'}}</dd></div></dl><p v-if="status.job.failure" role="alert">{{updateFailures[status.job.failure]||'更新未完成，请核对阶段记录'}}</p><ol class="f8-events" aria-label="更新阶段日志"><li v-for="(event,index) in status.job.events" :key="index"><time>{{date(event.at)}}</time> {{updatePhases[event.phase]||'状态待核对'}}</li></ol></template><p v-else>暂无在线更新记录。</p>
   <template #footer><el-button @click="refresh.refresh">读取更新状态</el-button><el-button @click="detailsOpen=false">关闭</el-button></template>
  </el-dialog>
  <el-dialog v-model="open" :title="recoveryId?'确认人工恢复已完成':'确认更新'" width="min(560px,94vw)" :close-on-click-modal="false" @closed="password='';confirmed=false">
   <p v-if="!recoveryId">{{updates.build?.version}} → {{target?.version}}</p><p v-if="error" role="alert">{{error}}</p>
   <p v-if="recoveryId" role="alert">只有确认旧 app、数据库和业务数据已由运维检查恢复完成，才能解除此任务互锁；此操作写入审计，不会触发更新。</p><ReleaseNotes v-if="!recoveryId&&target" :notes="target.notes" :tag="target.tag"/>
   <p v-if="!recoveryId" role="alert">更新会短暂中断播放。失败时启动保留的旧容器；仅实际 schema 变化时恢复数据库，届时备份后产生的业务变更可能丢失。</p>
   <label for="online-update-password">当前管理员密码</label><el-input id="online-update-password" v-model="password" type="password" autocomplete="current-password" :disabled="busy" @input="error=''"/>
   <el-checkbox class="mt-form-checkbox" v-model="confirmed">{{recoveryId?'我已核对上述风险，确认此操作':'我已了解播放中断及数据库恢复风险，确认更新'}}</el-checkbox>
   <template #footer><el-button :disabled="busy" @click="open=false">取消</el-button><el-button type="primary" :disabled="!confirmed||!password||(!recoveryId&&!ready)" :loading="busy" @click="execute">{{recoveryId?'确认变更':'确认并执行更新'}}</el-button></template>
  </el-dialog>
 </section>
</template>
<style scoped>
.f8-online{margin-top:16px;min-width:0}.f8-job-status{color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}.f8-risk{color:var(--el-color-warning-dark-2);line-height:1.6}.f8-job{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.f8-job dd{margin:4px 0;overflow-wrap:anywhere}.f8-events{padding-left:20px;overflow-wrap:anywhere}.f8-events time{font-variant-numeric:tabular-nums}@media(max-width:600px){.f8-job{grid-template-columns:1fr}}
</style>
