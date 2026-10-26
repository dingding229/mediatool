<script setup lang="ts">
import {computed,onMounted,ref,watch} from 'vue';
import {useUpdateStore} from '../stores/updates';
import {checkUpdates,setUpdateChecks} from '../api/updates';
import {date,errorText} from '../workflows/readiness';
import {usePageRefresh} from '../ui/page-refresh';
import OnlineUpdatePanel from './OnlineUpdatePanel.vue';
import ReleaseNotes from './ReleaseNotes.vue';
import {newerStable} from './online-update';
const updates=useUpdateStore(),busy=ref(false),error=ref(''),notice=ref('');
const hours=ref('12'),enabled=ref(true),dirty=ref(false);
const intervalValid=computed(()=>/^\d+$/.test(hours.value)&&Number(hours.value)>=1&&Number(hours.value)<=168);
watch(()=>updates.state,value=>{if(value&&!dirty.value){hours.value=String(value.intervalHours);enabled.value=value.enabled}},{immediate:true});
const latest=computed(()=>updates.state?.result.status==='verified'?updates.state.result.latest:null);
const version=computed(()=>{const value=updates.build?.version;return value?`v${value.replace(/^v/,'')}`:'读取中'});
const publishedAt=computed(()=>latest.value&&latest.value.version===updates.build?.version?latest.value.publishedAt:updates.build?.buildTime);
const status=computed(()=>({verified:updates.state?.hasUpdate?'发现新稳定版本':'未发现更高稳定版本',no_release:'尚无稳定版本发布',untrusted:'发布信息与 Docker Hub 镜像不一致，不提示更新',unavailable:'公开发布源暂不可用，未提示更新'}[updates.state?.result.status||'']||'尚未检查'));
async function check(){busy.value=true;error.value='';notice.value='';try{updates.accept(await checkUpdates());notice.value='检查已完成';await updates.refresh()}catch(e){error.value=errorText(e)}finally{busy.value=false}}
const checkSummary=computed(()=>status.value+' · 上次检查 '+(Date.parse(updates.state?.result.checkedAt||'')>0?date(updates.state?.result.checkedAt):'—')+(notice.value?' · '+notice.value:''));
async function saveSettings(){if(!intervalValid.value||busy.value)return;busy.value=true;error.value='';try{const saved=await setUpdateChecks(enabled.value,Number(hours.value));dirty.value=false;updates.accept(saved);hours.value=String(saved.intervalHours);enabled.value=saved.enabled;notice.value='检查设置已保存'}catch(e){error.value=errorText(e)}finally{busy.value=false}}
onMounted(()=>{updates.dismiss();void updates.refresh()});usePageRefresh(updates.refresh,'about');
</script>
<template>
 <section class="f8-about" aria-label="关于 Media Tool">
  <section class="settings-card"><h2>当前版本</h2><p class="f8-version">{{version}}</p></section>
  <section class="settings-card"><h2>发布时间</h2><p>{{date(publishedAt)||'尚未提供发布时间'}}</p></section>
  <section class="settings-card f8-update"><h2>版本更新</h2>
   <form class="f8-check-settings" @submit.prevent="saveSettings">
    <label class="f8-auto">自动检查更新 <el-switch v-model="enabled" :disabled="busy||!updates.state" aria-label="自动检查更新" @change="dirty=true"/></label>
    <label class="settings-field">检查间隔（小时）<input v-model="hours" aria-label="检查间隔（小时）" inputmode="numeric" :disabled="busy||!updates.state" :aria-invalid="!intervalValid" aria-describedby="update-interval-help" @input="dirty=true"/></label>
    <el-button native-type="submit" :disabled="busy||!updates.state||!intervalValid">保存检查设置</el-button><el-button :loading="busy" @click="check">检查更新</el-button>
   </form>
   <p id="update-interval-help" v-if="!intervalValid" role="alert">检查间隔必须是1–168小时的整数。</p><p v-if="error||updates.error" role="alert">{{error||updates.error}}</p>
   <template v-if="latest"><p>最新版本 {{latest.tag}}</p><ReleaseNotes :key="latest.tag" :notes="latest.notes" :tag="latest.tag" :expanded="!newerStable(latest.version,updates.build?.version||'')"/></template>
   <p v-else class="f8-empty">完成可信校验后，这里显示最新版本与变更说明。</p>
   <OnlineUpdatePanel :check-summary="checkSummary"/>
  </section>
 </section>
</template>
<style scoped>
.f8-check-settings{display:flex;align-items:end;gap:12px;flex-wrap:wrap}.f8-check-settings .el-button+.el-button{margin-left:0}.f8-check-settings .settings-field{max-width:180px}.f8-check-settings input{height:44px;min-width:0;width:100%;box-sizing:border-box;padding:8px 12px;border:1px solid var(--mt-border-color);border-radius:var(--el-border-radius-base);background:var(--mt-bg-surface);color:var(--mt-text-primary)}.f8-about{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.f8-about>.settings-card{margin:0;min-width:0;min-height:0}.f8-update{grid-column:1/-1}.f8-version{font-size:20px;font-weight:600;font-variant-numeric:tabular-nums}.f8-auto{display:inline-flex;align-items:center;gap:8px;min-height:44px}.f8-result,.f8-empty{color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}@media(max-width:600px){.f8-about{grid-template-columns:1fr}}
</style>
