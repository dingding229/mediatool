<script setup lang="ts">
import {computed,onMounted,ref} from 'vue';
import {useUpdateStore} from '../stores/updates';
import {checkUpdates,setUpdateChecks} from '../api/updates';
import {date,errorText} from '../workflows/readiness';
import {usePageRefresh} from '../ui/page-refresh';
import OnlineUpdatePanel from './OnlineUpdatePanel.vue';
import ReleaseNotes from './ReleaseNotes.vue';
import {newerStable} from './online-update';
const updates=useUpdateStore(),busy=ref(false),error=ref(''),notice=ref('');
const latest=computed(()=>updates.state?.result.status==='verified'?updates.state.result.latest:null);
const version=computed(()=>{const value=updates.build?.version;return value?`v${value.replace(/^v/,'')}`:'读取中'});
const publishedAt=computed(()=>latest.value&&latest.value.version===updates.build?.version?latest.value.publishedAt:updates.build?.buildTime);
const status=computed(()=>({verified:updates.state?.hasUpdate?'发现新稳定版本':'未发现更高稳定版本',no_release:'尚无稳定版本发布',untrusted:'发布信息与 Docker Hub 镜像不一致，不提示更新',unavailable:'公开发布源暂不可用，未提示更新'}[updates.state?.result.status||'']||'尚未检查'));
async function check(){busy.value=true;error.value='';notice.value='';try{updates.accept(await checkUpdates());notice.value='检查已完成';await updates.refresh()}catch(e){error.value=errorText(e)}finally{busy.value=false}}
async function toggle(value:boolean){busy.value=true;error.value='';try{updates.accept(await setUpdateChecks(value))}catch(e){error.value=errorText(e)}finally{busy.value=false}}
onMounted(()=>{updates.dismiss();void updates.refresh()});usePageRefresh(updates.refresh,'about');
</script>
<template>
 <section class="f8-about" aria-label="关于 Media Tool">
  <section class="settings-card"><h2>当前版本</h2><p class="f8-version">{{version}}</p></section>
  <section class="settings-card"><h2>发布时间</h2><p>{{date(publishedAt)||'尚未提供发布时间'}}</p></section>
  <section class="settings-card f8-update"><h2>版本更新</h2>
   <div class="media-actions"><label class="f8-auto">每 12 小时自动检查 <el-switch :model-value="updates.state?.enabled??true" :disabled="busy||!updates.state" aria-label="自动检查更新" @change="toggle(Boolean($event))"/></label><el-button :loading="busy" @click="check">检查更新</el-button></div>
   <p class="f8-result" role="status">{{status}} · 上次检查 {{Date.parse(updates.state?.result.checkedAt||'')>0?date(updates.state?.result.checkedAt):'—'}}<span v-if="notice"> · {{notice}}</span></p><p v-if="error||updates.error" role="alert">{{error||updates.error}}</p>
   <template v-if="latest"><p>最新版本 {{latest.tag}}</p><ReleaseNotes :key="latest.tag" :notes="latest.notes" :tag="latest.tag" :expanded="!newerStable(latest.version,updates.build?.version||'')"/></template>
   <p v-else class="f8-empty">完成可信校验后，这里显示最新版本与变更说明。</p>
   <OnlineUpdatePanel/>
  </section>
 </section>
</template>
<style scoped>
.f8-about{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.f8-about>.settings-card{margin:0;min-width:0;min-height:0}.f8-update{grid-column:1/-1}.f8-version{font-size:20px;font-weight:600;font-variant-numeric:tabular-nums}.f8-auto{display:inline-flex;align-items:center;gap:8px;min-height:44px}.f8-result,.f8-empty{color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}@media(max-width:600px){.f8-about{grid-template-columns:1fr}}
</style>
