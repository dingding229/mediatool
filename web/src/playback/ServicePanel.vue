<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { request } from '../api/client';
import { getNativeServerSettings, saveNativeServerSettings, listNativeLibraryMappings } from '../api/mediaServer';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { errorText } from '../workflows/readiness';
const status = reactive(useResource(getNativeServerSettings)), providers = reactive(useResource(() => request('/media-server/providers'))), page = ref(1), libraries = reactive(useResource(() => listNativeLibraryMappings((page.value - 1) * 50))), op = reactive(useOperation('service')), form = reactive({ serverName: '', enabled: false, revision: 0 }), latest = ref<any>(null), technical = ref(false), notice = ref('');
async function load() {
  const pristine = status.data && form.serverName === status.data.serverName && form.enabled === status.data.enabled && form.revision === status.data.revision;
  const s = await status.load();
  if (s && (!form.revision || pristine && !op.pending))
    Object.assign(form, { serverName: s.serverName, enabled: s.enabled, revision: s.revision });
  if (s && form.revision && s.revision !== form.revision)
    latest.value = s;
  await Promise.all([libraries.load(), providers.load()]);
  if (op.pending && s && s.revision !== Number(op.pending.key.split(':')[1])) {
    latest.value = s;
    op.clear();
  }
}
async function save() {
  if (!form.serverName.trim() || [...form.serverName.trim()].length > 100) {
    op.error = Error('服务名称需为1–100个字符');
    return;
  }
  const old = form.revision;
  const out = await op.run('settings:' + old, () => saveNativeServerSettings({ ...form, serverName: form.serverName.trim() }));
  if (out !== undefined) {
    const pristine = status.data && form.serverName === status.data.serverName && form.enabled === status.data.enabled && form.revision === status.data.revision;
    const s = await status.load();
    if (s && s.revision !== old) {
      Object.assign(form, { revision: s.revision });
      op.clear();
      notice.value = '已重新读取当前设置';
    }
  }
  else if (op.error) {
    latest.value = await status.load();
    notice.value = '保存未完成，输入已保留；请对照当前设置后采用最新修订。';
  }
}
onMounted(load);
defineExpose({ load, form, save, status, libraries, op });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <h2>服务设置</h2>
      <ResourceState :stale="status.loaded" :loading="status.loading" :error="status.error" @reload="load"/>
      <p>服务状态：{{ status.data?status.data.enabled?'已启用':'已停用':'—' }}</p>
      <ResourceState :error="providers.error" @reload="providers.load"/>
      <form class="u5-form" @submit.prevent="save">
        <label>
          服务名称
          <input v-model="form.serverName" aria-label="内置媒体库服务名称" maxlength="100" :disabled="op.busy"/>
        </label>
        <label class="u5-check">
          <input v-model="form.enabled" type="checkbox" aria-label="启用内置媒体库" :disabled="op.busy"/>
          启用播放服务
        </label>
        <p>停用后无法浏览和播放，探测和截图也会暂停；账号、授权和媒体文件保留。</p>
        <p>播放地址：{{ status.data?.publicUrl||'—' }} · 监听端口：{{ status.data?.listenPort||'—' }}</p>
        <p v-if="notice" role="status">{{ notice }}</p>
        <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
        <section v-if="latest">
          <p>当前服务：{{ latest.serverName }} · {{ latest.enabled?'已启用':'已停用' }}；你的输入：{{ form.serverName }}</p>
          <el-button @click="form.revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
        </section>
        <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
        <el-button native-type="submit" :loading="op.busy" :disabled="!status.data||!!status.error||!!latest||!!op.pending" class="mt-save-button" type="primary">保存服务设置</el-button>
      </form>
    </section>
    <section class="media-surface">
      <h2>播放如何到达你的设备</h2>
      <ol class="u5-chain" aria-label="播放链路">
        <li>播放器</li>
        <li>固定解析入口</li>
        <li>302</li>
        <li>115 官方地址</li>
      </ol>
      <p>服务器不中转流量。</p>
      <el-button @click="technical=true">技术详情</el-button>
      <div class="media-actions">
        <RouterLink to="/playback?tab=users">管理媒体用户</RouterLink>
      </div>
    </section>
    <section class="media-surface">
      <h2>连接播放器</h2>
      <p>使用下方服务地址和媒体用户账号，不是管理后台账号或外部 Emby 账号。</p>
      <h3>Infuse 添加</h3>
      <p>在设置中添加共享，选择 Emby，输入服务地址、媒体用户名和密码，再选择已授权的媒体库。</p>
      <h3>SenPlayer 添加</h3>
      <p>在媒体源或服务器管理中添加 Emby 类型服务器，填写同一服务地址与媒体账号；无法连接时检查网络和播放器准入规则。</p>
      <p>服务地址：{{ status.data?.publicUrl||'—' }}；地址和端口由部署配置提供，修改后需要重启服务。</p>
    </section>
    <section class="media-surface">
      <h2>媒体库目录</h2>
      <p>目录名称来自分类规则；新媒体库默认未授权，请在媒体用户中分配。</p>
      <ResourceState :stale="libraries.loaded" :loading="libraries.loading" :error="libraries.error" :empty="!libraries.data?.items.length" @reload="libraries.load"/>
      <article v-for="library in libraries.data?.items||[]" :key="library.id" class="organize-record">
        <h3>{{ library.name||'—' }}</h3>
        <p>{{ library.groupName||'—' }} / {{ library.ruleName||'—' }} · {{ library.hidden?'已隐藏':'可见' }} · {{ library.assetCount??'—' }} 项</p>
      </article>
      <PagePagination v-if="libraries.data?.total>50" v-model:current-page="page" :page-size="50" :total="libraries.data.total" layout="prev,pager,next,total" @current-change="libraries.load"/>
    </section>
    <DetailDrawer v-model:visible="technical" title="服务技术详情" :technical-data="{settings:status.data,services:providers.data,libraries:libraries.data}" class="media-detail">
      <ResourceState :stale="providers.loaded" :loading="providers.loading" :error="providers.error" @reload="providers.load"/>
      <p>这里记录服务能力与地址配置；是否能正常观看仍以实际设备测试为准。</p>
    </DetailDrawer>
  </div>
</template>
