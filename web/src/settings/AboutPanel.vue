<script setup lang="ts">
import {computed,onMounted,ref} from 'vue';
import {useUpdateStore} from '../stores/updates';
import {checkUpdates,setUpdateChecks} from '../api/updates';
import {date,errorText} from '../workflows/readiness';
import {usePageRefresh} from '../ui/page-refresh';
import IpAttribution from '../ui/IpAttribution.vue';
const updates=useUpdateStore(),busy=ref(false),error=ref(''),notice=ref('');
const status=computed(()=>({verified:updates.state?.hasUpdate?'发现新稳定版本':'未发现更高稳定版本',no_release:'尚无稳定版本发布',untrusted:'发布信息与 Docker Hub 镜像不一致，不提示更新',unavailable:'公开发布源暂不可用，未提示更新'}[updates.state?.result.status||'']||'尚未检查'));
async function check(){busy.value=true;error.value='';notice.value='';try{updates.accept(await checkUpdates());notice.value='检查已完成';await updates.refresh()}catch(e){error.value=errorText(e)}finally{busy.value=false}}
async function toggle(value:boolean){busy.value=true;error.value='';try{updates.accept(await setUpdateChecks(value))}catch(e){error.value=errorText(e)}finally{busy.value=false}}
onMounted(()=>{updates.dismiss();void updates.refresh()});usePageRefresh(updates.refresh,'about');
</script>
<template>
 <section class="settings-card f8-about">
  <h2>关于 Media Tool</h2>
  <dl class="f8-build"><div><dt>当前版本</dt><dd>{{updates.build?.version||'读取中'}}</dd></div><div><dt>Commit</dt><dd>{{updates.build?.commit||'未注入'}}</dd></div><div><dt>构建时间（北京时间）</dt><dd>{{date(updates.build?.buildTime)||'未注入'}}</dd></div><div><dt>Schema</dt><dd>{{updates.build?.schema??'—'}}</dd></div><div><dt>插件 pin</dt><dd>{{updates.build?.pluginPin||'读取中'}}</dd></div></dl>
  <h3>版本更新</h3>
  <div class="media-actions"><label>每 12 小时自动检查 <el-switch :model-value="updates.state?.enabled??true" :disabled="busy||!updates.state" aria-label="自动检查更新" @change="toggle(Boolean($event))"/></label><el-button :loading="busy" @click="check">检查更新</el-button></div>
  <p class="f8-result" role="status">{{status}} · 上次检查 {{Date.parse(updates.state?.result.checkedAt||'')>0?date(updates.state?.result.checkedAt):'—'}}<br/>{{notice}}</p><p v-if="error||updates.error" role="alert">{{error||updates.error}}</p>
  <section class="f8-latest"><template v-if="updates.state?.result.status==='verified'&&updates.state.result.latest"><dl class="f8-build"><div><dt>最新稳定版</dt><dd>{{updates.state.result.latest.tag}}</dd></div><div><dt>发布时间</dt><dd>{{date(updates.state.result.latest.publishedAt)}}</dd></div><div><dt>最低 schema</dt><dd>{{updates.state.result.latest.minimumSchema}}</dd></div><div><dt>AMD64 digest</dt><dd>{{updates.state.result.latest.digest}}</dd></div></dl><h3>变更说明</h3><pre class="f8-notes">{{updates.state.result.latest.notes}}</pre></template>
  <p v-else class="f8-notes f8-empty">完成可信校验后，这里显示最新版本与变更说明。</p></section>
  <p class="f8-hint">仅检查固定官方仓库的公开发布，不使用账号凭据；需要 Release 与 Docker Hub 的 digest 一致。本阶段不执行更新，不访问 Docker，也不会中断播放。</p>
 </section>
 <IpAttribution/>
</template>
<style scoped>.f8-about{min-height:640px}.f8-build{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.f8-build>div{min-width:0;min-height:80px}.f8-build dt{font-size:12px;color:var(--mt-text-muted)}.f8-build dd{margin:5px 0;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}.f8-result{min-height:50px}.f8-latest{min-height:480px}.f8-notes{white-space:pre-wrap;overflow-wrap:anywhere;font:inherit;min-height:150px;max-height:220px;overflow:auto;padding:16px;border:1px solid var(--mt-border-color);border-radius:8px}.f8-hint{font-size:13px;line-height:1.6;color:var(--mt-text-muted)}@media(max-width:600px){.f8-build{grid-template-columns:1fr}.f8-latest{min-height:640px}}</style>
