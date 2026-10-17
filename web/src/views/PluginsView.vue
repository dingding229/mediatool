<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessageBox } from 'element-plus'
import { catalog, instances, inspect, operate, dangerousScope, pluginPin } from '../api/plugins'
import type { PluginDefinition, PluginInstance, PluginOperation, Inspection } from '../api/plugins'
const definitions=ref<PluginDefinition[]>([]),installed=ref<PluginInstance[]>([]),page=ref(1),busy=ref(false),error=ref('')
const selected=ref<PluginInstance|null>(null),details=ref<Inspection|null>(null),configOpen=ref(false),dataOpen=ref(false),configText=ref('{}')
const consent=ref<PluginDefinition|null>(null),consentKind=ref<'install'|'review'>('install'),confirmed=ref(false)
const consentOpen=computed({get:()=>consent.value!==null,set:(value:boolean)=>{if(!value)consent.value=null}})
const names=computed(()=>new Map(definitions.value.map(d=>[d.manifest.id,d])))
const reasons:Record<string,string>={permission_review_required:'权限已变化，请重新确认',definition_missing:'内置库已移除此插件；数据保留',definition_unsupported:'当前版本不支持',worker_failed:'工作进程异常，自动退避',resource_limit:'资源超限，自动退避',data_unavailable:'数据目录暂不可用'}
const states:Record<string,string>={stopped:'已停止',starting:'启动中',running:'运行中',stopping:'停止中',backoff:'退避中',needs_action:'需要处理'}
const pending=new Map<string,string>()
async function load(){error.value='';try{const [c,i]=await Promise.all([catalog(),instances(page.value)]);definitions.value=c.items;installed.value=i}catch(e){error.value=e instanceof Error?e.message:'读取失败'}}
async function apply(input:Omit<PluginOperation,'operationKey'|'confirm'>){
 if(busy.value)return
 busy.value=true;error.value=''
 const fingerprint=JSON.stringify(input);let key=pending.get(fingerprint);if(!key){key=crypto.randomUUID();pending.set(fingerprint,key)}
 try{await operate({...input,confirm:true,operationKey:key});pending.delete(fingerprint);consent.value=null;configOpen.value=false;await load()}
 catch(e){error.value=e instanceof Error?e.message:'操作失败'}finally{busy.value=false}
}
function install(d:PluginDefinition){selected.value=null;consentKind.value='install';confirmed.value=false;consent.value=d}
function review(i:PluginInstance){const d=names.value.get(i.pluginId);if(!d)return;selected.value=i;consentKind.value='review';confirmed.value=false;consent.value=d}
async function accept(){const d=consent.value;if(!d||!confirmed.value)return;await apply({kind:consentKind.value,pluginId:d.manifest.id,instanceId:selected.value?.id,fingerprint:d.fingerprint,expectedRevision:selected.value?.revision??0,scopes:d.manifest.permissions.map(p=>p.scope),...(consentKind.value==='install'?{config:{}}:{})})}
async function action(i:PluginInstance,kind:'enable'|'disable'|'clone'|'remove'){
 const d=names.value.get(i.pluginId)
 try{await ElMessageBox.confirm(kind==='remove'?'停止并卸载此实例？保留加密数据，重新安装创建独立实例。':kind==='clone'?'复制配置及 KV，新分身默认停用，数据目录独立。':kind==='enable'?'启用此受信插件？它可以在已确认的权限内执行代码。':'停止此实例及其全部子进程？','确认插件操作',{type:'warning',confirmButtonText:'确认操作',cancelButtonText:'取消'})}catch{return}
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
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/>
 <section class="note-panel"><h2>运行与权限</h2><p>每个启用实例在独立子进程中运行。受信插件可执行代码；同 UID 与 Landlock 是尽力加固，不是恶意代码沙箱。安装不代表自动启用。</p><p>升级随主程序镜像发布，不支持上传、下载或运行期编译。SDK 契约 V2；扩权后停用，需重新确认。</p><small>内置库版本：{{pluginPin}}</small></section>
 <h2>内置目录</h2><section class="plugin-grid" aria-label="内置插件目录"><article v-for="d in definitions" :key="d.manifest.id" class="note-panel"><h3>{{d.manifest.name}}</h3><p>{{d.manifest.description}}</p><p>版本 {{d.manifest.version}} · {{d.manifest.permissions.length}} 项权限</p><el-button type="primary" :disabled="busy" @click="install(d)">安装插件</el-button></article></section>
 <h2>已安装实例</h2><p v-if="!installed.length">尚无实例。未选择的插件不会创建实例或数据。</p>
 <section class="plugin-grid" aria-label="已安装插件实例"><article v-for="i in installed" :key="i.id" class="note-panel instance-card"><h3>{{names.get(i.pluginId)?.manifest.name??i.pluginId}}</h3><code>{{i.id}}</code><p>{{states[i.state]??i.state}} · {{i.desiredState==='enabled'?'期望启用':'期望停用'}} · 修订 {{i.revision}}</p><el-alert v-if="i.reason" :title="reasons[i.reason]??i.reason" type="warning" :closable="false"/><div class="plugin-actions"><el-button v-if="i.desiredState!=='enabled'" :disabled="busy||i.reason==='permission_review_required'||!names.has(i.pluginId)" @click="action(i,'enable')">启用</el-button><el-button v-else :disabled="busy" @click="action(i,'disable')">停用</el-button><el-button v-if="i.reason==='permission_review_required'" @click="review(i)">重新确认权限</el-button><el-button :disabled="busy||!names.has(i.pluginId)" @click="action(i,'clone')">创建分身</el-button><el-button @click="open(i,'config')">配置</el-button><el-button @click="open(i,'data')">查看 KV</el-button><router-link :to="{path:'/logs',query:{module:'plugins',instance:i.id}}">实例日志</router-link><el-button type="danger" plain :disabled="busy" @click="action(i,'remove')">卸载</el-button></div></article></section>
 <nav class="plugin-actions" aria-label="实例分页"><el-button :disabled="page===1||busy" @click="changePage(-1)">上一页</el-button><span>第 {{page}} 页</span><el-button :disabled="installed.length<50||busy" @click="changePage(1)">下一页</el-button></nav>
 <el-dialog v-model="consentOpen" title="插件权限确认" width="min(680px,94vw)" :close-on-click-modal="false"><h3>{{consent?.manifest.name}}</h3><p>以下权限与当前定义指纹绑定。危险权限醒目标注；配置、KV 与数据目录按实例隔离。</p><ul><li v-for="p in consent?.manifest.permissions" :key="p.scope"><el-tag :type="dangerousScope(p.scope)?'danger':'info'">{{dangerousScope(p.scope)?'危险权限':'权限'}}</el-tag> <code>{{p.scope}}</code><pre v-if="p.resources">{{JSON.stringify(p.resources,null,2)}}</pre></li></ul><label class="permission-consent"><input v-model="confirmed" type="checkbox">我已阅读并同意以上权限</label><template #footer><el-button @click="consent=null">取消</el-button><el-button type="primary" :disabled="!confirmed||busy" :loading="busy" @click="accept">确认安装或授权</el-button></template></el-dialog>
 <el-dialog v-model="configOpen" title="实例配置" width="min(720px,94vw)"><p>加密保存。保存后当前 generation 失效并重启；冲突时请刷新。</p><label for="plugin-config">配置 JSON</label><el-input id="plugin-config" v-model="configText" type="textarea" :rows="12" :maxlength="131072"/><template #footer><el-button type="primary" :loading="busy" @click="save">保存配置</el-button></template></el-dialog>
 <el-dialog v-model="dataOpen" title="实例 KV 数据" width="min(760px,94vw)"><p>仅此实例，每页最多 50 条。日志单独按 instance 精确筛选。</p><pre>{{JSON.stringify(details?.entries,null,2)}}</pre><el-button v-if="details?.next&&selected" @click="open(selected,'data',details.next)">下一页 KV</el-button></el-dialog>
</template>
<style scoped>
.permission-consent{display:flex;align-items:center;gap:10px;min-height:44px;cursor:pointer}.permission-consent input{width:18px;height:18px;accent-color:var(--el-color-primary)}.plugin-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,320px),1fr));gap:16px;margin-block:16px}.note-panel p{line-height:1.7;margin-block:12px}.plugin-actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-block:16px}.plugin-actions :deep(.el-button),.plugin-actions a{min-height:44px;margin-left:0}.instance-card code,small{overflow-wrap:anywhere}pre{white-space:pre-wrap;overflow-wrap:anywhere;max-height:340px;overflow:auto}li{margin-block:12px}.page-header{display:flex;justify-content:space-between;gap:16px}
</style>
