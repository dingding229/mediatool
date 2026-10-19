<script setup lang="ts">
import PluginTools from '../plugins/PluginTools.vue'
import PluginAutomation from '../plugins/PluginAutomation.vue'
import PluginAuth from '../plugins/PluginAuth.vue'
import PluginSnapshots from '../plugins/PluginSnapshots.vue'
import PluginMetadata from '../plugins/PluginMetadata.vue'
import PluginForm from '../plugins/PluginForm.vue'
import {installIntent,hasInstallIntent,completeInstallIntent} from '../plugins/install-intent'
import {operationError} from '../plugins/operation-error'
import {APIError} from '../api/client'
import { requestUUID } from '../utils/request-uuid'
import inventory from '../../../internal/modules/plugins/v2/library.json'
import { computed, onMounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { catalog, instances, inspect, operate, dangerousScope, reviewPlan, extensions,runHistory,runExtension } from '../api/plugins'
import type { PluginDefinition, PluginInstance, PluginOperation, Inspection,ReviewPlan,ExtensionDescriptor,ExtensionSnapshot,PluginRun } from '../api/plugins'
const definitions=ref<PluginDefinition[]>([]),installed=ref<PluginInstance[]>([]),page=ref(1),catalogPage=ref(1),catalogTotal=ref(0),busy=ref(false),error=ref(''),notice=ref('')
const selected=ref<PluginInstance|null>(null),details=ref<Inspection|null>(null),configOpen=ref(false),dataOpen=ref(false),configText=ref('{}')
const extensionsOpen=ref(false),extensionSnapshot=ref<ExtensionSnapshot|null>(null),history=ref<PluginRun[]>([]),historyPage=ref(1),extensionBusy=ref(false),queued=ref('')
const runnableExtensions=computed(()=>extensionSnapshot.value?.metadata.descriptors.filter(d=>['service','command','action'].includes(d.kind))??[])
const extensionStates:Record<string,string>={pending:'等待执行',running:'执行中',succeeded:'已完成',failed:'失败',unknown:'结果未知（可能已产生副作用，不会自动重试）',canceled:'已取消'}
async function openExtensions(i:PluginInstance){selected.value=i;extensionsOpen.value=true;historyPage.value=1;await refreshExtensions()}
async function refreshExtensions(){if(!selected.value)return;extensionBusy.value=true;try{const [snapshot,runs]=await Promise.all([extensions(selected.value.id),runHistory(selected.value.id,historyPage.value)]);extensionSnapshot.value=snapshot;history.value=runs}catch(e){error.value=e instanceof Error?e.message:'扩展信息读取失败'}finally{extensionBusy.value=false}}
async function triggerExtension(d:ExtensionDescriptor){if(!selected.value||extensionBusy.value)return
 try{await ElMessageBox.confirm('执行此实例的'+(d.name??d.id)+'？只提交一次，unknown不自动重试。','确认执行扩展',{type:'warning',confirmButtonText:'确认执行',cancelButtonText:'取消',customClass:'plugin-confirm-dialog'})}catch{return}
 extensionBusy.value=true;error.value='';queued.value=''
 try{const fingerprint=selected.value.id+':'+d.handler;let key=pending.get(fingerprint);if(!key){key=requestUUID();pending.set(fingerprint,key)}
 await runExtension(selected.value.id,{kind:d.kind,id:d.id,operationKey:key,confirm:true});pending.delete(fingerprint);queued.value='已加入队列，等待执行结果（不是执行成功）';await refreshExtensions()}catch(e){error.value=e instanceof Error?e.message:'提交失败'}finally{extensionBusy.value=false}}
async function historyMove(delta:number){historyPage.value=Math.max(1,historyPage.value+delta);await refreshExtensions()}
const toolsOpen=ref(false),automationOpen=ref(false),authOpen=ref(false)
const snapshotOpen=ref(false)
const reviewDetails=ref<ReviewPlan|null>(null)
const consent=ref<PluginDefinition|null>(null),consentKind=ref<'install'|'review'>('install'),confirmed=ref(false)
const consentOpen=computed({get:()=>consent.value!==null,set:(value:boolean)=>{if(!value)consent.value=null}})
const names=computed(()=>new Map(definitions.value.map(d=>[d.manifest.id,d])))
const reasons:Record<string,string>={permission_review_required:'权限已变化，请重新确认',definition_missing:'内置库已移除此插件；数据保留',definition_unsupported:'当前版本不支持',worker_failed:'工作进程异常，自动退避',resource_limit:'资源超限，自动退避',data_unavailable:'数据目录暂不可用'}
const states:Record<string,string>={stopped:'已停止',starting:'启动中',running:'运行中',stopping:'停止中',backoff:'退避中',needs_action:'需要处理'}
const pending=new Map<string,string>()
const operationPhase=ref<'idle'|'confirming'|'submitting'|'committed'|'sync_failed'|'uncertain'|'rejected'>('idle')
const retryInput=ref<Omit<PluginOperation,'operationKey'|'confirm'>|null>(null)
let loadEpoch=0
function installLabel(d:PluginDefinition){return hasInstallIntent(d.manifest.id,d.fingerprint)?'核对上次安装':d.installedCount?'新增实例':'安装插件'}
const available=(i:PluginInstance)=>i.availability?i.availability==='available':names.value.has(i.pluginId)
async function changeCatalog(delta:number){catalogPage.value=Math.max(1,catalogPage.value+delta);await load()}
async function load(){
 const token=++loadEpoch
 if(typeof window!=='undefined')window.dispatchEvent(new Event('plugins:changed'))
 error.value=''
 try{const [c,i]=await Promise.all([catalog(catalogPage.value),instances(page.value)])
 if(token!==loadEpoch)return false
 definitions.value=c.items;catalogTotal.value=c.total;installed.value=i
 if(operationPhase.value==='sync_failed')operationPhase.value='committed'
 return true
 }catch(e){if(token===loadEpoch)error.value='列表同步失败；'+operationError(e).label;return false}
}
async function apply(input:Omit<PluginOperation,'operationKey'|'confirm'>){
 if(busy.value)return
 busy.value=true;error.value='';notice.value='';operationPhase.value='submitting'
 let submitted=false
 try{
 const fingerprint=JSON.stringify(input);let key=pending.get(fingerprint)
 if(!key){if(pending.size>=32)throw new Error('operation_intent_limit');key=input.kind==='install'?installIntent(input.pluginId!,input.fingerprint,requestUUID):requestUUID();pending.set(fingerprint,key)}
 retryInput.value=JSON.parse(fingerprint)
 submitted=true
 const out=await operate({...input,confirm:true,operationKey:key})
 if(!out||typeof out.id!=='string'||!out.id||out.id.length>64||!Number.isSafeInteger(out.revision)||out.revision<1||(input.kind==='install'&&out.pluginId!==input.pluginId)||(input.instanceId&&input.kind!=='clone'&&out.id!==input.instanceId))throw new APIError(200,'invalid_response','invalid_operation_receipt')
 pending.delete(fingerprint);retryInput.value=null
 if(input.kind==='install'){
 completeInstallIntent(input.pluginId!,input.fingerprint)
 installed.value=[out,...installed.value.filter(i=>i.id!==out.id)].slice(0,50)
 definitions.value=definitions.value.map(d=>d.manifest.id===out.pluginId?{...d,installedCount:Math.max(d.installedCount??0,installed.value.filter(i=>i.pluginId===out.pluginId).length)}:d)
 }
 operationPhase.value='committed'
 notice.value=input.kind==='install'?`已安装 · 实例 ${out.id} · 安装不会自动启用，请核对当前状态。`:input.kind==='review'?'权限已确认。实例仍为停用，请显式启用。':`操作已提交 · 实例 ${out.id}`
 consent.value=null;configOpen.value=false
 if(!await load()&&error.value)operationPhase.value='sync_failed'
 }catch(e){
 const problem=operationError(e);operationPhase.value=submitted&&problem.uncertain?'uncertain':'rejected'
 if(submitted&&!problem.uncertain){pending.delete(JSON.stringify(input));retryInput.value=null;if(input.kind==='install')completeInstallIntent(input.pluginId!,input.fingerprint)}
 error.value=(operationPhase.value==='uncertain'?'结果未确认；请核对原请求，不要新增实例。':'操作未完成；')+problem.label
 }finally{busy.value=false}
}
function install(d:PluginDefinition){selected.value=null;reviewDetails.value=null;notice.value='';consentKind.value='install';confirmed.value=false;consent.value=d;operationPhase.value='confirming'}
async function retryOperation(){if(retryInput.value)await apply(retryInput.value)}
async function review(i:PluginInstance){if(busy.value)return;busy.value=true;error.value='';confirmed.value=false
 try{const plan=await reviewPlan(i.id);selected.value=plan.instance;reviewDetails.value=plan;consentKind.value='review';consent.value={manifest:plan.current,fingerprint:plan.fingerprint}}catch(e){error.value=e instanceof Error?e.message:'确认预览读取失败'}finally{busy.value=false}}
async function accept(){const d=consent.value;if(!d||!confirmed.value||(consentKind.value==='review'&&(reviewDetails.value?.availability!=='available'||!reviewDetails.value.needsReview)))return;await apply({kind:consentKind.value,pluginId:d.manifest.id,instanceId:selected.value?.id,fingerprint:d.fingerprint,expectedRevision:selected.value?.revision??0,scopes:d.manifest.permissions.map(p=>p.scope),...(consentKind.value==='install'?{config:{}}:{})})}
async function action(i:PluginInstance,kind:'enable'|'disable'|'clone'|'remove'){
 const d=names.value.get(i.pluginId)
 try{await ElMessageBox.confirm(kind==='remove'?'停止并卸载此实例？保留加密数据，重新安装创建独立实例。':kind==='clone'?'复制配置及 KV，新分身默认停用，数据目录独立。':kind==='enable'?'启用此受信插件？它可以在已确认的权限内执行代码。':'停止此实例及其全部子进程？','确认插件操作',{type:'warning',confirmButtonText:'确认操作',cancelButtonText:'取消',customClass:'plugin-confirm-dialog'})}catch{return}
 let fp=d?.fingerprint;if(!fp){try{fp=(await inspect(i.id)).fingerprint}catch(e){error.value=String(e);return}}
 await apply({kind,instanceId:i.id,fingerprint:fp,expectedRevision:i.revision,scopes:d?.manifest.permissions.map(p=>p.scope)??[]})
}
async function open(i:PluginInstance,mode:'config'|'data',after=''){
 try{selected.value=i;details.value=await inspect(i.id,after);configText.value=JSON.stringify(details.value.config.value,null,2);configOpen.value=mode==='config';dataOpen.value=mode==='data'}catch(e){error.value=e instanceof Error?e.message:'读取失败'}
}
async function save(){if(!selected.value||!details.value)return;let config:unknown;try{config=JSON.parse(configText.value)}catch{error.value='配置必须是有效 JSON';return}
 await apply({kind:'configure',instanceId:selected.value.id,fingerprint:details.value.fingerprint,expectedRevision:details.value.instance.revision,scopes:[],config})}
async function changePage(delta:number){page.value=Math.max(1,page.value+delta);await load()}
onMounted(load)
</script>
<template>
 <header class="page-header"><div><h1>插件</h1><p>维护者审核的内置 Go 插件库 · 按需安装，默认停用</p></div><el-button :loading="busy" @click="load">刷新</el-button></header>
 <el-alert v-if="notice" role="status" aria-live="polite" :title="notice" type="success" :closable="false"/>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/>
 <section class="note-panel"><h2>运行与权限</h2><p>每个启用实例在独立子进程中运行。受信插件可执行代码；同 UID 与 Landlock 是尽力加固，不是恶意代码沙箱。安装不代表自动启用。</p><p>更新随应用镜像发布，仅使用镜像内置定义。SDK 契约 V2；扩权后停用，需重新确认并显式启用。配置、KV 与实例数据保留；不提供单插件代码回退。</p></section>
 <p v-if="operationPhase==='submitting'" role="status" aria-live="polite">正在提交；请勿重复操作。</p><el-button v-if="operationPhase==='uncertain'&&retryInput" :disabled="busy" @click="retryOperation">重试核对原请求</el-button>
 <el-button @click="automationOpen=true">工作流管理</el-button><h2>内置目录</h2><section class="plugin-grid" aria-label="内置插件目录"><article v-for="d in definitions" :key="d.manifest.id" class="note-panel"><PluginMetadata :metadata="d.manifest"/><p><el-tag type="success">可用</el-tag> · {{d.installedCount===undefined?'安装状态待读取':d.installedCount?`已安装 ${d.installedCount} 个实例`:'未安装'}}</p><el-button type="primary" :disabled="busy" @click="install(d)">{{installLabel(d)}}</el-button></article></section>
 <nav class="plugin-actions" aria-label="目录分页"><el-button :disabled="catalogPage===1||busy" @click="changeCatalog(-1)">上一页目录</el-button><span>第 {{catalogPage}} 页 · 共 {{catalogTotal}} 项</span><el-button :disabled="catalogPage*200>=catalogTotal||busy" @click="changeCatalog(1)">下一页目录</el-button></nav>
 <h2>已安装实例</h2><p v-if="!installed.length">尚无实例。未选择的插件不会创建实例或数据。</p>
 <section class="plugin-grid" aria-label="已安装插件实例"><article v-for="i in installed" :key="i.id" class="note-panel instance-card"><h3>{{i.metadata?.name??names.get(i.pluginId)?.manifest.name??i.pluginId}}</h3><code>{{i.id}}</code><p>定义版本 {{i.metadata?.version??names.get(i.pluginId)?.manifest.version??'未知'}} · {{available(i)?'定义可用':'定义不可用，配置 / KV / 数据保留'}}</p><p>{{states[i.state]??i.state}} · {{i.desiredState==='enabled'?'期望启用':'期望停用'}} · 修订 {{i.revision}}</p><el-alert v-if="i.reason" :title="reasons[i.reason]??i.reason" type="warning" :closable="false"/><p v-if="i.state!=='running'">动态表单与插件状态页仅运行时可用；停用时可保存 JSON 配置。</p><div class="plugin-actions"><el-button v-if="i.desiredState!=='enabled'" :disabled="busy||i.reason==='permission_review_required'||!available(i)" @click="action(i,'enable')">启用</el-button><el-button v-else :disabled="busy" @click="action(i,'disable')">停用</el-button><el-button :disabled="busy" @click="review(i)">{{i.reason==='permission_review_required'?'重新确认权限':'版本与权限'}}</el-button><el-button :disabled="busy||!available(i)" @click="action(i,'clone')">创建分身</el-button><el-button @click="open(i,'config')">配置</el-button><el-button @click="open(i,'data')">查看 KV</el-button><el-button @click="selected=i;snapshotOpen=true">数据快照</el-button><el-button :disabled="i.state!=='running'" @click="selected=i;toolsOpen=true">工具测试</el-button><el-button @click="selected=i;authOpen=true">认证状态</el-button><el-button :disabled="i.desiredState!=='enabled'||i.state!=='running'" @click="openExtensions(i)">服务与命令</el-button><router-link v-if="i.state==='running'" :to="`/plugins/${i.id}/pages/${inventory.manifests.find(m=>m.id===i.pluginId)?.extensions.pages?.[0]??'overview'}`">插件页面</router-link><router-link :to="{path:'/logs',query:{module:'plugins',instance:i.id}}">实例日志</router-link><el-button type="danger" plain :disabled="busy" @click="action(i,'remove')">卸载</el-button></div></article></section>
 <nav class="plugin-actions" aria-label="实例分页"><el-button :disabled="page===1||busy" @click="changePage(-1)">上一页</el-button><span>第 {{page}} 页</span><el-button :disabled="installed.length<50||busy" @click="changePage(1)">下一页</el-button></nav>
 <el-dialog v-model="consentOpen" title="插件权限确认" width="min(680px,94vw)" :close-on-click-modal="false" :close-on-press-escape="!busy" :show-close="!busy"><p role="status">{{operationPhase==='submitting'?'正在提交，请等待服务端回执。':operationPhase==='uncertain'?'结果未确认：再次确认会复用原 operationKey 核对。':operationPhase==='rejected'?'请求未发出或已被拒绝；尚未取得成功回执，请查看结果码。':'尚未提交：勾选同意后，还需点击“确认安装或授权”。确认成功后实例仍停用。'}}</p><p v-if="consentKind==='install'&&consent&&hasInstallIntent(consent.manifest.id,consent.fingerprint)">检测到待核对安装，将复用原 operationKey 核对，不会发起新增实例。</p><p v-else-if="consentKind==='install'&&consent?.installedCount">已安装 {{consent.installedCount}} 个实例；本操作将新增独立实例。结果未确认的请求优先核对原请求。</p><PluginMetadata v-if="consent" :metadata="consent.manifest"/>
 <section v-if="reviewDetails" aria-label="升级权限差异"><p>已批准版本：{{reviewDetails.approvedVersion||'历史基线未知'}} → 当前定义版本：{{reviewDetails.current.version}}</p><el-alert v-if="!reviewDetails.baselineKnown" title="历史版本及资源范围基线未知；仅按现有授权比较 scope，不推测旧范围。" type="warning" :closable="false"/>
 <el-alert v-if="reviewDetails.availability!=='available'" title="当前定义不可用；配置、KV 和实例数据保留，不能确认或启用。" type="warning" :closable="false"/>
 <div v-for="[field,label] in ([['added','新增'],['removed','移除'],['changed','资源范围变化']] as const)" :key="field"><h4>{{label}}</h4><p v-if="!reviewDetails[field].length">无</p><ul v-else><li v-for="scope in reviewDetails[field]" :key="scope"><el-tag :type="dangerousScope(scope)?'danger':'info'">{{dangerousScope(scope)?'危险':'权限'}}</el-tag> <code>{{scope}}</code></li></ul></div><p v-if="!reviewDetails.needsReview">当前定义已批准，无需重复确认。</p><p>确认不会启用实例。权限已确认后，仍需显式点击启用。</p><el-button :disabled="busy" @click="selected&&review(selected)">重新读取确认预览</el-button></section><p>以下权限与当前定义指纹绑定。危险权限醒目标注；配置、KV 与数据目录按实例隔离。</p><ul><li v-for="p in consent?.manifest.permissions" :key="p.scope"><el-tag :type="dangerousScope(p.scope)?'danger':'info'">{{dangerousScope(p.scope)?'危险权限':'权限'}}</el-tag> <code>{{p.scope}}</code><pre v-if="p.resources">{{JSON.stringify(p.resources,null,2)}}</pre></li></ul><label class="permission-consent"><input v-model="confirmed" type="checkbox">我已阅读并同意以上权限</label><template #footer><el-button :disabled="busy" @click="consent=null">取消</el-button><el-button type="primary" :disabled="!confirmed||busy||(reviewDetails!==null&&(reviewDetails.availability!=='available'||!reviewDetails.needsReview))" :loading="busy" @click="accept">确认安装或授权</el-button></template></el-dialog>
 <el-dialog v-model="automationOpen" title="工作流管理" width="min(960px,94vw)" destroy-on-close><PluginAutomation/></el-dialog>
 <el-dialog v-model="toolsOpen" title="人工工具测试" width="min(820px,94vw)" destroy-on-close><PluginTools v-if="selected" :instance-id="selected.id" :key="selected.id"/></el-dialog>
 <el-dialog v-model="authOpen" title="认证状态" width="min(760px,94vw)" destroy-on-close><PluginAuth v-if="selected" :instance-id="selected.id" :key="selected.id"/></el-dialog>
 <el-dialog v-model="snapshotOpen" title="实例数据快照" width="min(920px,94vw)" destroy-on-close><PluginSnapshots v-if="selected" :instance-id="selected.id" :key="selected.id" @changed="load"/></el-dialog>
 <el-dialog v-model="configOpen" title="实例配置" width="min(720px,94vw)"><p>加密保存。停用时可直接保存 JSON，实例仍保持停用。动态表单和状态页需要 worker 运行，不会为显示页面自动启用插件。</p><PluginForm v-if="selected" :running="selected.state==='running'" :instance-id="selected.id" v-model="configText"/><label for="plugin-config">配置 JSON</label><el-input id="plugin-config" v-model="configText" type="textarea" :rows="12" :maxlength="131072"/><template #footer><el-button type="primary" :loading="busy" @click="save">保存配置</el-button></template></el-dialog>
 <el-dialog v-model="dataOpen" title="实例 KV 数据" width="min(760px,94vw)"><p>仅此实例，每页最多 50 条。日志单独按 instance 精确筛选。</p><pre>{{JSON.stringify(details?.entries,null,2)}}</pre><el-button v-if="details?.next&&selected" @click="open(selected,'data',details.next)">下一页 KV</el-button></el-dialog>
 <el-dialog v-model="extensionsOpen" title="服务与命令" width="min(920px,94vw)">
 <p>仅注册表已确认的服务、命令和动作可手动提交。定时服务采用合并错过执行策略，同一服务不积压。</p><el-alert v-if="queued" :title="queued" type="info" :closable="false"/><el-button :loading="extensionBusy" @click="refreshExtensions">刷新执行状态</el-button>
 <el-table :data="runnableExtensions" empty-text="此实例没有可执行扩展"><el-table-column prop="kind" label="类型" width="110"/><el-table-column prop="id" label="标识" min-width="140"/><el-table-column prop="name" label="名称" min-width="130"/><el-table-column prop="trigger" label="调度" width="110"/><el-table-column label="操作" width="120"><template #default="{row}"><el-button :disabled="extensionBusy" @click="triggerExtension(row)">手动执行</el-button></template></el-table-column></el-table>
 <h3>运行历史</h3><el-table :data="history" empty-text="暂无运行记录"><el-table-column prop="kind" label="类型" width="100"/><el-table-column prop="handler" label="处理器" min-width="210" show-overflow-tooltip/><el-table-column label="状态" min-width="180"><template #default="{row}">{{extensionStates[row.state]??row.state}}</template></el-table-column><el-table-column prop="reason" label="原因码" min-width="120"/><el-table-column prop="createdAt" label="时间" min-width="190"/></el-table><nav class="plugin-actions" aria-label="运行历史分页"><el-button :disabled="historyPage===1||extensionBusy" @click="historyMove(-1)">上一页历史</el-button><span>第 {{historyPage}} 页</span><el-button :disabled="history.length<50||extensionBusy" @click="historyMove(1)">下一页历史</el-button></nav>
 </el-dialog>
</template>
<style scoped>
/* MessageBox teleports outside this SFC. Bound the box, not only its viewport-sized overlay. */
:global(.el-message-box.plugin-confirm-dialog){width:min(420px,calc(100vw - 32px));max-width:420px;overflow-wrap:anywhere}

.permission-consent{display:flex;align-items:center;gap:10px;min-height:44px;cursor:pointer}.permission-consent input{width:18px;height:18px;accent-color:var(--el-color-primary)}.plugin-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr));gap:16px;margin-block:16px}.note-panel p{line-height:1.7;margin-block:12px}.plugin-actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-block:16px}.plugin-actions :deep(.el-button),.plugin-actions a{min-height:44px;margin-left:0}.instance-card code,small{overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:340px;overflow:auto}li{margin-block:12px}.page-header{display:flex;justify-content:space-between;gap:16px}
</style>
