<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useRoute } from 'vue-router';
import { catalog, instances, type PluginDefinition } from '../api/plugins';
import { useResource } from '../workflows/useResource';
import { usePluginOperation, pluginState, reasonName, safePluginError } from '../plugin-workspace/model';
import { hasInstallIntent } from '../plugins/install-intent';
import Permissions from '../plugin-workspace/Permissions.vue';
import PluginAutomation from '../plugins/PluginAutomation.vue';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import ResourceState from '../workflows/ResourceState.vue';
import '../media/media.css';
import '../workflows/u5.css';
import '../operations/u6.css';
const route = useRoute(), tab = computed(() => route.query.tab === 'catalog' ? 'catalog' : 'installed'), page = ref(1), catalogPage = ref(1), query = ref(''), list = reactive(useResource(() => instances(page.value), [])), directory = reactive(useResource(() => catalog(catalogPage.value))), op = reactive(usePluginOperation()), consent = ref<PluginDefinition | null>(null), confirmed = ref(false), automation = ref(false), tech = ref(false);
const shown = computed(() => list.data.filter(i => (i.metadata?.name || i.pluginId).toLowerCase().includes(query.value.toLowerCase()))), definitions = computed(() => directory.data?.items.filter(d => (d.manifest.name + ' ' + d.manifest.description).toLowerCase().includes(query.value.toLowerCase())) || []);
async function read() {
  await Promise.all([list.load(), directory.load()]);
  if (list.error || directory.error)
    throw Error('read-failed');
}
async function load() {
  await op.reconcile(read);
}
function select(d: PluginDefinition) {
  consent.value = d;
  confirmed.value = false;
}
async function install() {
  if (!consent.value || !confirmed.value)
    return;
  const d = consent.value;
  await op.run({ kind: 'install', pluginId: d.manifest.id, fingerprint: d.fingerprint, expectedRevision: 0, scopes: d.manifest.permissions.map(p => p.scope), config: {} });
  if (op.pending || op.notice)
    consent.value = null;
  await load();
}
onMounted(load);
</script>
<template>
  <div class="media-workspace u5-workspace u6-workspace">
    <PageHeader title="插件" description="按需安装内置插件，确认权限后再启用。">
      <template #actions>
        <el-button type="primary" @click="load">刷新插件状态</el-button>
      </template>
    </PageHeader>
    <PageTabs :tabs="[{id:'installed',label:'已安装'},{id:'catalog',label:'插件目录'}]" default-tab="installed"/>
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>{{ tab==='installed'?'已安装插件':'插件目录' }}</h2>
        <el-button @click="automation=true">工作流管理</el-button>
      </header>
      <label class="u5-field">
        查找插件
        <input v-model="query" aria-label="查找插件"/>
      </label>
      <p>扩展权限后插件会停用；需要重新确认权限，再明确启用。不可用插件的配置和数据仍然保留。</p>
      <el-alert v-if="op.error" :title="op.error" type="error" :closable="false"/>
      <p v-if="op.notice" role="status">{{ op.notice }}</p>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.pending?.input.kind==='install'?'上次安装结果未确认，正在核对，不会重复安装。':'上次操作尚未确认，请先核对当前实例。'" @refresh="load"/>
      <el-button v-if="op.pending&&op.canReconcile" :disabled="op.busy" @click="op.replay(read).then(load)">核对原请求</el-button>
      <el-button v-if="op.technical" @click="tech=true">查看技术详情</el-button>
      <template v-if="tab==='installed'">
        <ResourceState :loading="list.loading" :error="safePluginError(list.error)" :empty="!shown.length" empty-title="尚未安装匹配的插件" @reload="load"/>
        <div v-if="!list.error" class="u6-card-grid">
          <article v-for="i in shown" :key="i.id" class="u6-card">
            <h3>{{ i.metadata?.name||i.pluginId }}</h3>
            <p>{{ pluginState(i) }} · {{ i.desiredState==='enabled'?'期望启用':'期望停用' }}</p>
            <p v-if="i.reason">{{ reasonName(i.reason) }}</p>
            <RouterLink :to="'/plugins/'+encodeURIComponent(i.id)" class="el-button">管理插件</RouterLink>
          </article>
        </div>
        <div class="media-actions">
          <el-button :disabled="page===1||list.loading" @click="page--;load()">上一页实例</el-button>
          <span>第 {{ page }} 页</span>
          <el-button :disabled="list.data.length<50||list.loading" @click="page++;load()">下一页实例</el-button>
        </div>
      </template>
      <template v-else>
        <ResourceState :loading="directory.loading" :error="safePluginError(directory.error)" :empty="!definitions.length" @reload="load"/>
        <div v-if="!directory.error" class="u6-card-grid">
          <article v-for="d in definitions" :key="d.manifest.id" class="u6-card">
            <h3>{{ d.manifest.name }}</h3>
            <p>{{ d.manifest.description||'—' }}</p>
            <p>{{ d.installedCount==null?'安装数量 —':'已安装 '+d.installedCount+' 个实例' }}</p>
            <el-button :disabled="!!op.pending||op.busy" @click="select(d)">{{ hasInstallIntent(d.manifest.id,d.fingerprint)?'核对上次安装':d.installedCount?'新增独立实例':'安装插件' }}</el-button>
          </article>
        </div>
        <div class="media-actions">
          <el-button :disabled="catalogPage===1||directory.loading" @click="catalogPage--;load()">上一页目录</el-button>
          <span>第 {{ catalogPage }} 页 · 共 {{ directory.data?.total??'—' }} 项</span>
          <el-button :disabled="catalogPage*200>=(directory.data?.total||0)||directory.loading" @click="catalogPage++;load()">下一页目录</el-button>
        </div>
      </template>
    </section>
    <el-dialog :model-value="!!consent" title="插件权限确认" width="min(700px,94vw)" class="media-dialog u5-dialog" :close-on-click-modal="false" @update:model-value="consent=null">
      <h3>{{ consent?.manifest.name }}</h3>
      <el-alert v-if="op.error" :title="op.error" type="error" :closable="false"/>
      <p>安装将创建独立实例，默认停用；启用后可在以下权限范围内运行。</p>
      <Permissions :permissions="consent?.manifest.permissions||[]"/>
      <label class="u5-check">
        <input v-model="confirmed" type="checkbox"/>
        我已阅读并同意以上权限
      </label>
      <template #footer>
        <el-button @click="consent=null">取消</el-button>
        <el-button :disabled="!confirmed||!!op.pending" :loading="op.busy" @click="install">确认安装插件</el-button>
      </template>
    </el-dialog>
    <el-dialog v-model="automation" title="工作流管理" width="min(960px,94vw)" class="media-dialog" destroy-on-close>
      <PluginAutomation/>
    </el-dialog>
    <DetailDrawer v-model:visible="tech" title="操作技术详情" :technical-data="op.technical" class="media-detail"/>
  </div>
</template>
