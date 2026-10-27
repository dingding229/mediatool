<script setup lang="ts">
import {useRefresh} from '../composables/useRefresh';
import {providePageRefresh,usePageRefresh} from '../ui/page-refresh';
const ownRefresh=providePageRefresh();
import { computed, onMounted, reactive, ref } from 'vue';
import {ElDropdown,ElDropdownMenu,ElDropdownItem} from 'element-plus';
import 'element-plus/es/components/dropdown/style/css';
import 'element-plus/es/components/dropdown-menu/style/css';
import 'element-plus/es/components/dropdown-item/style/css';
import { useRoute } from 'vue-router';
import { inspect, reviewPlan, staticPluginForm } from '../api/plugins';
import inventory from '../../../internal/modules/plugins/v2/library.json';
import { useResource } from '../workflows/useResource';
import { usePluginOperation, pluginState, reasonName, safePluginError, permissionName } from '../plugin-workspace/model';
import Permissions from '../plugin-workspace/Permissions.vue';
import ExtensionsPanel from '../plugin-workspace/ExtensionsPanel.vue';
import PluginForm from '../plugins/PluginForm.vue';
import InstanceDashboards from './InstanceDashboards.vue';
import {validateForm} from '../plugins/form-validation';
import PageHeader from '../ui/PageHeader.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import ResourceState from '../workflows/ResourceState.vue';
import '../media/media.css';
import '../workflows/u5.css';
import '../operations/u6.css';
const route = useRoute(), id = String(route.params.id), data = reactive(useResource(() => inspect(id))), plan = reactive(useResource(() => reviewPlan(id))), op = reactive(usePluginOperation()), config = ref(''), draftLoaded = ref(false), draftRevision = ref(0), latest = ref(false), confirm = ref(false), kind = ref(''), consent = ref(false), confirmed = ref(false), formValid = ref(false);
const row = computed(() => data.data?.instance), unavailable = computed(() => !!row.value && ((row.value.availability && row.value.availability !== 'available') || ['definition_missing', 'definition_unsupported'].includes(row.value.reason))), pages = computed(() => inventory.manifests.find(m => m.id === row.value?.pluginId)?.extensions.pages || []);
async function read() {
  await data.load();
  if (data.error)
    throw Error('read-failed');
  if (data.data) {
    if (!draftLoaded.value) {
      config.value = JSON.stringify(data.data.config.value, null, 2);
      draftRevision.value = data.data.instance.revision;
      draftLoaded.value = true;
    }
    else if (data.data.instance.revision !== draftRevision.value)
      latest.value = true;
  }
}
async function load() {
  await op.reconcile(read);
}
function ask(value: string) {
  kind.value = value;
  confirm.value = true;
}
async function apply() {
  if (!data.data)
    return;
  let value: unknown;
  if (kind.value === 'configure') {
    try {
      value = JSON.parse(config.value);
    }
    catch {
      op.error = '配置值无效，请核对表单。';
      return;
    }
  }
  if(kind.value==='configure'){const form=staticPluginForm(data.data.instance.pluginId);if(!form){op.error='设置表单暂不可用，未修改已有配置。';return;}const message=validateForm(form.content,{...form.defaults,...value as Record<string,unknown>});if(message){op.error=message;return;}}
  const r = data.data.instance;
  const result = await op.run({ kind: kind.value as any, instanceId: id, fingerprint: data.data.fingerprint, expectedRevision: kind.value === 'configure' ? draftRevision.value : r.revision, scopes: [], ...(kind.value === 'configure' ? { config: value } : {}) });
  const receipt=result||op.pending?.receipt;
  if(kind.value==='configure'&&receipt){draftRevision.value=receipt.revision;latest.value=false;}
  confirm.value = false;
  await load();
  settle();
}
let settleLeft=0;
const transitional = (i?: { state: string; desiredState?: string }) => !!i && (['starting', 'stopping'].includes(i.state) || (i.state === 'stopped' && i.desiredState === 'enabled') || (i.state === 'running' && i.desiredState === 'disabled'));
function settle(left=20){settleLeft=left;}
useRefresh(async()=>{settleLeft--;await data.load()},{interval:1500,immediate:false,active:()=>settleLeft>0&&transitional(row.value)});
async function review() {
  await plan.load();
  confirmed.value = false;
  consent.value = !!plan.data && !plan.error;
}
async function approve() {
  if (!confirmed.value || !plan.data || plan.data.availability !== 'available' || !plan.data.needsReview)
    return;
  const p = plan.data;
  await op.run({ kind: 'review', instanceId: id, pluginId: p.current.id, fingerprint: p.fingerprint, expectedRevision: p.instance.revision, scopes: p.current.permissions.map(s => s.scope) });
  consent.value = false;
  await load();
}
onMounted(load);
ownRefresh.register(load,()=>true);
</script>
<template>
 <div class="media-workspace u5-workspace u6-workspace f5-plugin-detail">
  <PageHeader :title="row?.metadata?.name||inventory.manifests.find(m=>m.id===row?.pluginId)?.name||'插件详情'" description="设置插件、查看日志与执行常用操作。"/>
  <RouterLink to="/plugins?tab=installed">返回已安装插件</RouterLink>
  <ResourceState :stale="data.loaded" :loading="data.loading" :error="safePluginError(data.error)" @reload="load"/>
  <el-alert v-if="op.error" :title="op.error" type="error" :closable="false"/>
  <p v-if="op.notice" role="status">{{op.notice}}</p>
  <AsyncResultBanner v-if="op.phase" :status="op.phase" message="上次操作结果尚未确认，先核对原请求，不重复提交。" @refresh="load"/>
  <RouterLink v-if="op.pending?.input.instanceId&&op.pending.input.instanceId!==id" :to="'/plugins/'+encodeURIComponent(op.pending.input.instanceId)">返回待核对插件</RouterLink>
  <el-button v-if="op.canReconcile&&op.pending?.input.kind!=='configure'" :disabled="op.busy" @click="op.replay(read).then(load)">核对原请求</el-button>
  <section v-if="row" class="media-surface">
   <header class="media-section-heading"><div><h2>插件状态</h2><p>{{pluginState(row)}}</p></div><el-switch :model-value="row.desiredState==='enabled'" :disabled="!!op.pending||op.busy||!!unavailable||row.reason==='permission_review_required'" aria-label="启用插件" active-text="启用" :before-change="()=>{ask(row.desiredState==='enabled'?'disable':'enable');return false}"/></header>
   <p v-if="row.reason">{{reasonName(row.reason)}}</p>
   <p v-if="unavailable">此插件当前不可用，已有配置和数据仍保留。</p>
   <el-button v-if="row.reason==='permission_review_required'" :disabled="!!op.pending" @click="review">确认访问范围变化</el-button>
   <div class="media-actions">
    <RouterLink :to="{path:'/ops',query:{tab:'logs',module:'plugins',instance:id}}">查看实例日志</RouterLink>
    <RouterLink v-for="key in pages" v-show="row.state==='running'" :key="key" :to="'/plugins/'+encodeURIComponent(id)+'/pages/'+encodeURIComponent(key)">打开插件应用</RouterLink>
    <el-dropdown trigger="click"><el-button aria-label="更多插件操作">更多</el-button><template #dropdown><el-dropdown-menu><el-dropdown-item :disabled="!!op.pending||!!unavailable" @click="ask('clone')">创建分身</el-dropdown-item><el-dropdown-item :disabled="!!op.pending" @click="ask('remove')">卸载插件</el-dropdown-item></el-dropdown-menu></template></el-dropdown>
   </div>
  </section>
  <section v-if="row" class="media-surface">
   <h2>插件配置</h2>
   <PluginForm v-model="config" :plugin-id="row.pluginId" :readonly="!!op.pending||op.busy" @valid="formValid=$event"/>
   <p v-if="latest" role="status">配置已在其他页面修改；你的输入仍保留。请重新载入，或确认采用当前修订后保存。</p>
   <div v-if="latest" class="media-actions"><el-button @click="draftLoaded=false;latest=false;load()">重新载入配置</el-button><el-button @click="draftRevision=data.data!.instance.revision;latest=false">保留输入并采用最新修订</el-button></div>
   <el-button :disabled="!formValid||!!op.pending||op.busy||latest||!!data.error" @click="ask('configure')" type="primary">保存插件配置</el-button>
  </section>
  <InstanceDashboards v-if="row" :instance-id="id" :plugin-id="row.pluginId" :running="row.state==='running'"/>
  <section v-if="row?.state==='running'" class="media-surface"><ExtensionsPanel :id="id" :running="row.state==='running'"/></section>
  <ConfirmAction v-model:visible="confirm" title="确认插件操作" :description="({enable:'启用此插件？',disable:'停用此插件？',clone:'创建独立分身？',remove:'停止并卸载此插件？',configure:'保存此插件配置？'} as any)[kind]" :impact-scope="kind==='remove'?'停止实例并卸载，保留加密数据。':kind==='clone'?'复制配置和数据，分身默认停用。':kind==='enable'?'插件可能处理媒体、网络或系统数据；只启用你信任的插件。':kind==='configure'?'只保存此实例配置，不会自动启用。':'停止后台处理，保留配置和数据。'" confirm-text="确认操作" :danger-level="kind==='remove'?'danger':'warning'" :loading="op.busy" @confirm="apply"/>
  <el-dialog v-model="consent" title="插件权限确认" width="min(760px,94vw)">
   <ResourceState :error="safePluginError(plan.error)" @reload="review"/>
   <p>访问范围发生变化，插件保持停用；确认后也不会自动启用。</p>
   <p v-if="plan.data&&!plan.data.baselineKnown">历史授权记录不完整，请核对下列访问范围。</p>
   <section v-for="(label,key) in {added:'新增访问',removed:'移除访问',changed:'范围变化'}" :key="key"><h3>{{label}}</h3><p v-for="scope in plan.data?.[key]||[]" :key="scope">{{permissionName(scope)}}</p><p v-if="!plan.data?.[key].length">无</p></section>
   <Permissions :permissions="plan.data?.current.permissions||[]"/>
   <p v-if="plan.data?.availability!=='available'">当前定义不可用，已有配置和数据保留。</p>
   <label class="u5-check mt-form-checkbox"><input v-model="confirmed" type="checkbox"/>我已阅读并同意以上访问范围</label>
   <template #footer><el-button @click="consent=false">取消</el-button><el-button @click="review">核对结果</el-button><el-button :disabled="!confirmed||!plan.data?.needsReview||plan.data?.availability!=='available'||!!op.pending" @click="approve">确认权限并保持停用</el-button></template>
  </el-dialog>
 </div>
</template>
