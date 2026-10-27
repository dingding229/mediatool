<script setup lang="ts">
import {computed,onUnmounted,ref} from 'vue';
import {getIdleUpdate,saveIdleUpdate,checkIdleUpdate,type IdleUpdatePolicy,type IdleUpdateCheck} from '../api/updates';
import {useRefresh} from '../composables/useRefresh';
import {usePageRefresh} from '../ui/page-refresh';
import {errorText,date} from '../workflows/readiness';
const policy=ref<IdleUpdatePolicy>(),enabled=ref(false),start=ref('02:00'),end=ref('05:00'),minutes=ref('60'),error=ref(''),notice=ref(''),busy=ref(false),open=ref(false),password=ref(''),confirm=ref(false),check=ref<IdleUpdateCheck>();
const valid=computed(()=>/^\d+$/.test(minutes.value)&&Number(minutes.value)>=15&&Number(minutes.value)<=360&&/^([01]\d|2[0-3]):[0-5]\d$/.test(start.value)&&/^([01]\d|2[0-3]):[0-5]\d$/.test(end.value)&&start.value!==end.value);
function accept(p:IdleUpdatePolicy){policy.value=p;enabled.value=p.enabled;start.value=p.startTime;end.value=p.endTime;minutes.value=String(p.quietMinutes)}
const dirty=computed(()=>policy.value&&(enabled.value!==policy.value.enabled||start.value!==policy.value.startTime||end.value!==policy.value.endTime||minutes.value!==String(policy.value.quietMinutes)));
async function load(){try{const p=await getIdleUpdate();if(!dirty.value)accept(p)}catch(e){error.value=errorText(e)}}
const refresh=useRefresh(load,{interval:()=>policy.value?.enabled?60000:0});usePageRefresh(refresh.refresh,'about');
async function save(){if(!valid.value||!policy.value||busy.value)return;if(enabled.value&&!open.value){open.value=true;return}if(enabled.value&&(!password.value||!confirm.value))return;busy.value=true;error.value='';notice.value='';const secret=password.value;password.value='';try{accept(await saveIdleUpdate({...policy.value,enabled:enabled.value,startTime:start.value,endTime:end.value,quietMinutes:Number(minutes.value)},secret,confirm.value));open.value=false;check.value=undefined;notice.value=enabled.value?'闲时更新已授权，满足全部条件后才执行':'闲时自动更新已关闭'}catch(e){error.value=errorText(e)}finally{busy.value=false}}
async function evaluate(){busy.value=true;error.value='';try{check.value=await checkIdleUpdate()}catch(e){error.value=errorText(e)}finally{busy.value=false}}
onUnmounted(()=>{password.value=''});
</script>
<template>
 <section class="idle-update" aria-label="闲时自动更新配置">
  <h3>闲时自动更新</h3>
  <p class="idle-hint">默认关闭。按服务器时区执行，时间段支持跨午夜；无人播放、安静时长及关键任务等条件全部通过才更新。失败自动关闭，需重新验密授权。</p>
  <form class="idle-form" @submit.prevent="save">
   <label class="idle-toggle">闲时自动更新 <el-switch v-model="enabled" aria-label="闲时自动更新" :disabled="busy||!policy"/></label>
   <label class="settings-field">开始时间<input v-model="start" class="mt-native-time-input" type="time" aria-label="开始时间" :disabled="busy||!policy"/></label>
   <label class="settings-field">结束时间<input v-model="end" class="mt-native-time-input" type="time" aria-label="结束时间" :disabled="busy||!policy"/></label>
   <label class="settings-field">安静时长（分钟）<input v-model="minutes" class="mt-native-form-input" inputmode="numeric" aria-label="安静时长（分钟）" :aria-invalid="!valid" :disabled="busy||!policy"/></label>
   <el-button native-type="submit" :disabled="busy||!policy||!valid">保存闲时设置</el-button>
   <el-button :disabled="busy||!policy" :loading="busy" @click="evaluate">检查执行条件</el-button>
  </form>
  <p v-if="!valid" role="alert">请输入合法且不同的开始、结束时间；安静时长为15–360分钟整数。</p>
  <p v-if="policy?.reason" role="status">{{policy.reason}}</p><p v-if="notice" role="status">{{notice}}</p><p v-if="error" role="alert">{{error}}</p>
  <template v-if="check"><p class="idle-hint">服务器时区：{{check.timezone}} · {{date(check.checkedAt)}} · {{check.ready?'全部条件通过':'暂不满足执行条件'}}（只读检查，不会触发更新）</p>
   <ul class="idle-conditions"><li v-for="condition in check.conditions" :key="condition.key"><span>{{condition.label}}</span><span :class="{'idle-blocked':!condition.passed}">{{condition.passed?'通过':condition.reason}}</span></li></ul>
  </template>
  <el-dialog v-model="open" title="授权闲时自动更新" width="min(560px,94vw)" :close-on-click-modal="false" @closed="password='';confirm=false">
   <p>授权后台在指定时间段内无人值守更新固定官方仓库的已校验稳定版。更新可能短暂中断服务；失败按原流程回滚，实际 schema 变化时可能恢复数据库。</p>
   <p>改密、管理员重置或恢复、主动关闭、更新失败会使授权失效；登录会话自然过期不会撤销这项独立授权。</p>
   <p v-if="error" role="alert">{{error}}</p>
   <label for="idle-update-password">当前管理员密码</label><el-input id="idle-update-password" v-model="password" type="password" autocomplete="current-password" :disabled="busy"/>
   <el-checkbox class="mt-form-checkbox" v-model="confirm">我已了解无人值守更新及数据库恢复风险，确认授权</el-checkbox>
   <template #footer><el-button :disabled="busy" @click="open=false">取消</el-button><el-button type="primary" :disabled="!password||!confirm||!valid" :loading="busy" @click="save">确认授权</el-button></template>
  </el-dialog>
 </section>
</template>
<style scoped>
.idle-update{margin-top:20px;padding-top:16px;border-top:1px solid var(--el-border-color-lighter);min-width:0}.idle-update h3{margin:0 0 8px}.idle-hint{color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}.idle-form{display:flex;align-items:end;gap:12px;flex-wrap:wrap}.idle-form .settings-field{width:180px;max-width:100%;min-width:0}.idle-toggle{display:flex;align-items:center;gap:8px;min-height:44px}.idle-form .el-button+.el-button{margin-left:0}.idle-conditions{list-style:none;padding:0;margin:12px 0;display:grid;gap:8px}.idle-conditions li{display:grid;grid-template-columns:130px minmax(0,1fr);gap:12px;line-height:1.6;overflow-wrap:anywhere}.idle-blocked{color:var(--mt-text-muted)}@media(max-width:600px){.idle-form .settings-field{flex:1 1 130px;max-width:none}.idle-toggle{flex-basis:100%}.idle-conditions li{grid-template-columns:105px minmax(0,1fr);gap:8px}}
</style>
