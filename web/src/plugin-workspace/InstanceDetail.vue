<script setup lang="ts">
import {useRefresh} from '../composables/useRefresh';
import {providePageRefresh,usePageRefresh} from '../ui/page-refresh';
const ownRefresh=providePageRefresh();
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import { inspect, reviewPlan } from '../api/plugins';
import inventory from '../../../internal/modules/plugins/v2/library.json';
import { useResource } from '../workflows/useResource';
import { usePluginOperation, pluginState, reasonName, safePluginError, permissionName } from '../plugin-workspace/model';
import Permissions from '../plugin-workspace/Permissions.vue';
import ExtensionsPanel from '../plugin-workspace/ExtensionsPanel.vue';
import PluginAutomation from '../plugins/PluginAutomation.vue';
import PluginForm from '../plugins/PluginForm.vue';
import PluginTools from '../plugins/PluginTools.vue';
import PluginAuth from '../plugins/PluginAuth.vue';
import PluginSnapshots from '../plugins/PluginSnapshots.vue';
import PageHeader from '../ui/PageHeader.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import ResourceState from '../workflows/ResourceState.vue';
import '../media/media.css';
import '../workflows/u5.css';
import '../operations/u6.css';
const route = useRoute(), id = String(route.params.id), after = ref(''),cursors=ref(['']),cursorIndex=ref(1), data = reactive(useResource(() => inspect(id, after.value))), plan = reactive(useResource(() => reviewPlan(id))), op = reactive(usePluginOperation()), config = ref(''), draftLoaded = ref(false), draftRevision = ref(0), latest = ref(false), confirm = ref(false), kind = ref(''), consent = ref(false), confirmed = ref(false), panel = ref('config'), tool = ref(''), detail = ref(false), kv = ref(false);
const row = computed(() => data.data?.instance), unavailable = computed(() => !!row.value && ((row.value.availability && row.value.availability !== 'available') || ['definition_missing', 'definition_unsupported'].includes(row.value.reason))), pages = computed(() => inventory.manifests.find(m => m.id === row.value?.pluginId)?.extensions.pages || []);
async function read() {
  await Promise.all([data.load(), plan.load()]);
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
      op.error = '配置需要是有效的 JSON';
      return;
    }
  }
  const r = data.data.instance;
  await op.run({ kind: kind.value as any, instanceId: id, fingerprint: data.data.fingerprint, expectedRevision: kind.value === 'configure' ? draftRevision.value : r.revision, scopes: [], ...(kind.value === 'configure' ? { config: value } : {}) });
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
function dataPage(n:number){if(n>cursorIndex.value){if(!data.data?.next)return;cursors.value.push(data.data.next)}cursorIndex.value=n;after.value=cursors.value[n-1]||'';void data.load()}
</script>
<template>
  <div class="media-workspace u5-workspace u6-workspace">
    <PageHeader :title="row?.metadata?.name||'插件详情'" description="查看权限、配置和运行状态；启停与卸载都需要明确确认。">
      <template #actions>

      </template>
    </PageHeader>
    <RouterLink to="/plugins?tab=installed">返回已安装插件</RouterLink>
    <ResourceState :stale="data.loaded" :loading="data.loading" :error="safePluginError(data.error)" @reload="load"/>
    <el-alert v-if="op.error" :title="op.error" type="error" :closable="false"/>
    <p v-if="op.notice" role="status">{{ op.notice }}</p>
    <AsyncResultBanner v-if="op.phase" :status="op.phase" message="上次插件操作结果未确认，正在核对，不会重复操作。" @refresh="load"/>
    <RouterLink v-if="op.pending?.input.instanceId&&op.pending.input.instanceId!==id" :to="'/plugins/'+encodeURIComponent(op.pending.input.instanceId)">返回待核对插件</RouterLink>
    <el-button v-if="op.canReconcile&&op.pending?.input.kind!=='configure'" :disabled="op.busy" @click="op.replay(read).then(load)">核对原请求</el-button>
    <section v-if="row" class="media-surface">
      <header class="media-section-heading">
        <h2>实例状态</h2>
        <strong>{{ pluginState(row) }}</strong>
      </header>
      <p v-if="row.reason">{{ reasonName(row.reason) }}</p>
      <p v-if="unavailable">此插件当前不可用，配置和数据仍保留；不能启用或创建分身。</p>
      <div class="media-actions">
        <el-button v-if="row.desiredState!=='enabled'" :disabled="!!op.pending||!!unavailable||row.reason==='permission_review_required'" @click="ask('enable')">启用插件</el-button>
        <el-button v-else :disabled="!!op.pending" @click="ask('disable')">停用插件</el-button>
        <el-button :disabled="!!op.pending||!!unavailable" @click="ask('clone')">创建分身</el-button>
        <el-button :disabled="!!op.pending" @click="review">{{ row.reason==='permission_review_required'?'重新确认权限':'查看权限' }}</el-button>
        <el-button :disabled="!!op.pending" @click="ask('remove')">卸载插件</el-button>
        <RouterLink :to="{path:'/ops',query:{tab:'logs',module:'plugins',instance:id}}">实例日志</RouterLink>
        <el-button @click="detail=true">技术详情</el-button>
      </div>
    </section>
    <section v-if="row" class="media-surface">
      <nav class="media-actions mt-segments" aria-label="插件管理功能">
        <el-button v-for="(label,key) in {config:'配置',data:'实例数据',extensions:'服务与命令'}" :key="key" :aria-pressed="panel===key" @click="panel=String(key)">{{ label }}</el-button>
      </nav>
      <template v-if="panel==='config'">
        <h2>插件配置</h2>
        <PluginForm v-model="config" :instance-id="id" :running="row.state==='running'"/>
        <label class="u5-field">
          配置 JSON
          <textarea v-model="config" rows="12" aria-label="插件配置 JSON" :disabled="!!op.pending"/>
        </label>
        <p v-if="latest">配置已被修改，已重新读取；你的输入仍保留供对照。</p>
        <details v-if="latest">
          <summary>对照当前保存配置（只读）</summary>
          <pre class="u6-raw">{{ JSON.stringify(data.data?.config.value,null,2) }}</pre>
        </details>
        <el-button v-if="latest" @click="draftRevision=data.data!.instance.revision;latest=false">保留输入并采用最新修订</el-button>
        <el-button :disabled="!!op.pending||latest||!!data.error" @click="ask('configure')" type="primary" class="mt-save-button">保存插件配置</el-button>
      </template>
      <template v-else-if="panel==='data'">
        <h2>实例数据</h2>
        <div class="media-actions">
          <el-button @click="kv=true">查看 KV</el-button>
          <el-button @click="tool='snapshots'">数据快照</el-button>
          <el-button @click="tool='auth'">认证状态</el-button>
          <el-button @click="tool='automation'">工作流管理</el-button>
          <el-button :disabled="row.state!=='running'" @click="tool='tools'">工具测试</el-button>
        </div>
      </template>
      <ExtensionsPanel v-else :id="id" :running="row.state==='running'"/>
      <div class="media-actions">
        <RouterLink v-for="key in pages" v-show="row.state==='running'" :key="key" :to="'/plugins/'+encodeURIComponent(id)+'/pages/'+encodeURIComponent(key)">打开插件页面 · {{ key }}</RouterLink>
      </div>
    </section>
    <ConfirmAction v-model:visible="confirm" title="确认插件操作" :description="({enable:'启用此插件？',disable:'停用此插件？',clone:'创建独立分身？',remove:'停止并卸载此插件？',configure:'保存此插件配置？'} as any)[kind]" :impact-scope="kind==='remove'?'停止实例并卸载，保留加密数据；重新安装会创建独立实例。':kind==='clone'?'复制配置和 KV，分身默认停用，数据目录独立。':kind==='enable'?'插件将以本服务权限运行，可能处理媒体、网络或系统数据；只启用你信任的插件，隔离保护不能替代信任。':kind==='configure'?'修改此实例配置，不会自动启用；运行中的插件会使用更新后的配置。':'停止此实例及其后台处理，保留配置和数据。'" confirm-text="确认操作" :danger-level="kind==='remove'?'danger':'warning'" :loading="op.busy" class="media-dialog" @confirm="apply"/>
    <el-dialog v-model="consent" title="插件权限确认" width="min(760px,94vw)" class="media-dialog u5-dialog">
      <ResourceState :error="safePluginError(plan.error)" @reload="review"/>
      <p>权限变化后插件保持停用；确认权限不会自动启用。</p>
      <p v-if="plan.data&&!plan.data.baselineKnown">历史授权范围不完整，以下按现有记录比较，不推测旧范围。</p>
      <section v-for="(label,key) in {added:'新增权限',removed:'移除权限',changed:'资源范围变化'}" :key="key">
        <h3>{{ label }}</h3>
        <p v-if="!plan.data?.[key].length">无</p>
        <p v-for="scope in plan.data?.[key]||[]" :key="scope">{{ permissionName(scope) }}</p>
      </section>
      <Permissions :permissions="plan.data?.current.permissions||[]"/>
      <p v-if="plan.data?.availability!=='available'">当前定义不可用，配置和数据保留。</p>
      <label class="u5-check">
        <input v-model="confirmed" type="checkbox"/>
        我已阅读并同意以上权限
      </label>
      <template #footer>
        <el-button @click="consent=false">取消</el-button>
        <el-button @click="review">核对结果</el-button>
        <el-button :disabled="!confirmed||!plan.data?.needsReview||plan.data?.availability!=='available'||!!op.pending" @click="approve">确认权限并保持停用</el-button>
      </template>
    </el-dialog>
    <el-dialog :model-value="!!tool" :title="({tools:'人工工具测试',auth:'认证状态',snapshots:'实例数据快照',automation:'工作流管理'} as any)[tool]" width="min(960px,94vw)" class="media-dialog u6-runtime" destroy-on-close @update:model-value="tool=''">
      <PluginAutomation v-if="tool==='automation'"/>
      <PluginTools v-if="tool==='tools'" :instance-id="id"/>
      <PluginAuth v-if="tool==='auth'" :instance-id="id"/>
      <PluginSnapshots v-if="tool==='snapshots'" :instance-id="id" @changed="load"/>
    </el-dialog>
    <DetailDrawer v-model:visible="kv" title="实例 KV 数据" :technical-data="data.data?.entries" class="media-detail">
      <article v-for="entry in data.data?.entries||[]" :key="entry.key" class="organize-record">
        <strong>{{ entry.key }}</strong>
        <pre class="u6-raw">{{ JSON.stringify(entry.value,null,2) }}</pre>
      </article>
      <PagePagination v-if="data.data?.next||cursorIndex>1" :current-page="cursorIndex" :has-next="!!data.data?.next" @current-change="dataPage"/>
    </DetailDrawer>
    <DetailDrawer v-model:visible="detail" title="插件技术详情" :technical-data="{inspection:data.data,review:plan.data,error:op.technical}" class="media-detail"/>
  </div>
</template>
