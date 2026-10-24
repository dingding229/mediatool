<script setup lang="ts">
import {computed,ref} from 'vue';
import {useUpdateStore} from '../stores/updates';
import {getOnlineUpdate,executeOnlineUpdate,setOnlineUpdateControl,type OnlineUpdateStatus} from '../api/updates';
import {useRefresh} from '../composables/useRefresh';
import {usePageRefresh} from '../ui/page-refresh';
import {date,errorText} from '../workflows/readiness';
import {newerStable,updateDone,updatePhases,updateFailures} from './online-update';
const control=ref<"enable"|"disable"|"confirm_recovery"|"">(''),recoveryId=ref('');
const updates=useUpdateStore(),status=ref<OnlineUpdateStatus>(),error=ref(''),open=ref(false),password=ref(''),confirmed=ref(false),busy=ref(false);
const target=computed(()=>updates.state?.result.status==='verified'?updates.state.result.latest:null);
const running=computed(()=>Boolean(status.value?.job&&!updateDone(status.value.job.phase)));
const ready=computed(()=>status.value?.executionEnabled&&!(status.value?.recoveryRequired?.length)&&status.value?.available&&status.value.policyVerified&&!running.value&&target.value&&newerStable(updates.build?.version||'',target.value.version));
async function load(){try{status.value=await getOnlineUpdate();error.value=''}catch(e){error.value=errorText(e)}}
const refresh=useRefresh(load,{interval:()=>running.value?3000:0});usePageRefresh(refresh.refresh,'about');
function prepare(){control.value='';error.value='';password.value='';confirmed.value=false;open.value=true}
function prepareControl(action:"enable"|"disable"|"confirm_recovery",id=''){control.value=action;recoveryId.value=id;error.value='';password.value='';confirmed.value=false;open.value=true}
async function execute(){if(control.value){if(!confirmed.value||!password.value)return;busy.value=true;const secret=password.value;password.value='';try{await setOnlineUpdateControl(control.value,recoveryId.value,secret,true);open.value=false;await refresh.refresh()}catch(e){error.value=errorText(e)}finally{busy.value=false}return}if(!ready.value||!target.value||!confirmed.value||!password.value)return;busy.value=true;error.value='';const secret=password.value;password.value='';try{const job=await executeOnlineUpdate(target.value.version,target.value.digest,secret,true);if(status.value)status.value.job=job;open.value=false;await refresh.refresh()}catch(e){await refresh.refresh();error.value=errorText(e)}finally{busy.value=false}}
</script>
<template>
 <section class="settings-card f8-online" aria-label="在线更新">
  <h2>在线更新</h2>
  <div class="f8-risk-slot"><p v-if="status?.socketMounted" class="f8-risk">已挂载 docker.sock，app 拥有宿主机级权限。白名单仅限制正常代码路径，不能隔离被攻陷的应用。</p>
  <p v-else>未挂载 docker.sock，一键更新不可用；仍可检查最新版本。</p>
  <p v-if="status&&!status.available">{{status.reason}}</p>
  </div><div class="f8-controls"><p>执行开关：<strong>{{status?.executionEnabled?'已开启':'已关闭（安全默认）'}}</strong>；不影响检查更新。</p><el-button :disabled="!status||busy||running||(!status.executionEnabled&&!!status.recoveryRequired?.length)" @click="prepareControl(status?.executionEnabled?'disable':'enable')">{{status?.executionEnabled?'关闭执行':'开启执行'}}</el-button><p v-for="id in status?.recoveryRequired||[]" :key="id">任务 {{id}} 尚待人工恢复确认。<el-button :disabled="busy||running" @click="prepareControl('confirm_recovery',id)">任务已完成人工确认</el-button></p></div><div class="media-actions"><el-button :disabled="!ready" @click="prepare">一键更新</el-button><el-button @click="refresh.refresh">读取更新状态</el-button></div>
  <p class="f8-job-status" role="status">{{status?.job?updatePhases[status.job.phase]||'等待核对状态':status?'尚无在线更新记录':'读取更新状态中'}}<br/>{{status?.policyVerified?'Docker API 白名单校验已通过':''}}</p>
  <p v-if="error&&!open" role="alert">{{error}}</p>
  <div class="f8-record" aria-label="最近更新记录"><template v-if="status?.job"><p v-if="status.job.failure" role="alert">{{updateFailures[status.job.failure]||'更新未完成，请检查阶段记录'}}</p><dl class="f8-job"><div><dt>执行管理员</dt><dd>{{status.job.actor}}</dd></div><div><dt>版本</dt><dd>{{status.job.fromVersion}} → {{status.job.target.version}}</dd></div><div><dt>最近状态</dt><dd>{{date(status.job.updatedAt)}}</dd></div><div><dt>备份目录</dt><dd>{{status.job.backup||'尚未生成'}}</dd></div></dl><ol class="f8-events" aria-label="更新阶段日志"><li v-for="(event,index) in status.job.events" :key="index"><time>{{date(event.at)}}</time> {{updatePhases[event.phase]||'状态待核对'}}</li></ol></template><p v-else class="f8-no-record">执行后将在这里显示管理员、版本、备份目录与阶段记录。</p></div>
  <el-dialog v-model="open" :title="control==='confirm_recovery'?'确认人工恢复已完成':control?'变更在线更新执行开关':'确认更新'" width="min(560px, 94vw)" :close-on-click-modal="false" @closed="password='';confirmed=false">
   <p v-if="!control">{{updates.build?.version}} → {{target?.version}}</p><p v-if="error" role="alert">{{error}}</p><p v-if="control==='confirm_recovery'" role="alert">只有确认旧 app、数据库和业务数据已由运维检查恢复完成，才能解除此任务互锁；此操作写入审计，不会开启执行开关。</p><p v-else-if="control" role="alert">{{control==='enable'?'开启后管理员可再次确认并执行更新。docker.sock 等同宿主机级权限，请谨慎开启。':'关闭后禁止提交新的在线更新；更新检查不受影响。'}}</p><pre v-if="!control" class="f8-update-notes">{{target?.notes}}</pre>
   <p v-if="!control" role="alert">更新会短暂中断播放。失败时启动保留的旧容器；仅实际 schema 变化时恢复数据库，届时备份后产生的业务变更可能丢失。</p>
   <label for="online-update-password">当前管理员密码</label><el-input id="online-update-password" v-model="password" type="password" autocomplete="current-password" :disabled="busy" @input="error=''"/>
   <el-checkbox v-model="confirmed">{{control?'我已核对上述风险，确认此操作':'我已了解播放中断及数据库恢复风险，确认更新'}}</el-checkbox>
   <template #footer><el-button :disabled="busy" @click="open=false">取消</el-button><el-button type="primary" :disabled="!confirmed||!password||(!control&&!ready)" :loading="busy" @click="execute">{{control?'确认变更':'确认并执行更新'}}</el-button></template>
  </el-dialog>
 </section>
</template>
<style scoped>.f8-online{min-height:700px}.f8-risk-slot{height:100px;overflow:auto}.f8-record{height:420px;overflow:auto;border-block:1px solid var(--mt-border-color);padding-block:12px}.f8-no-record{color:var(--mt-text-muted)}.f8-risk{color:var(--el-color-warning-dark-2);line-height:1.6}.f8-job-status{min-height:52px}.f8-job{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.f8-job dd{margin:4px 0;overflow-wrap:anywhere}.f8-job dt{color:var(--mt-text-muted);font-size:12px}.f8-events{max-height:240px;overflow:auto;padding-left:24px;line-height:1.8;font-size:13px}.f8-events time{font-variant-numeric:tabular-nums}.f8-update-notes{max-height:180px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere;font:inherit}.el-checkbox{height:auto;white-space:normal;margin-top:16px}.f8-online :deep(.el-checkbox__label){white-space:normal;line-height:1.5}.f8-online :deep(.el-dialog__body){max-height:calc(85vh - 140px);overflow:auto}@media(max-width:600px){.f8-risk-slot{height:160px}.f8-record{height:540px}.f8-job{grid-template-columns:1fr}.f8-events time{display:block}}</style>
