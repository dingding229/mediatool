<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { request, post } from '../api/client';
const status = ref<Record<string, any> | null>(null), busy = ref(false), error = ref(''), message = ref(''), unknown = ref(false);
async function load() {
  try {
    status.value = await request('/system/status');
    error.value = '';
  }
  catch {
    error.value = '网关状态读取失败，请检查服务。';
  }
}
async function test() {
  if (busy.value || unknown.value)
    return;
  busy.value = true;
  message.value = '';
  try {
    const result = await post<any>('/emby/test', {});
    message.value = result?.id ? '网关检测已排队，尚未完成。' : `Emby ${result?.version || '—'}：公共信息接口检测完成，不代表实际播放器验收。`;
    if (result?.id)
      unknown.value = true;
  }
  catch {
    unknown.value = true;
    error.value = '网关检测回执未知，请先核对服务状态，不重复提交。';
  }
  finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
<section class="connection-manager">
  <header class="connection-section-heading">
    <div>
      <h2>Emby 网关诊断</h2>
      <p>严格原始 STRM、302 到官方地址；不提供中转、HLS 或转码兜底。</p>
    </div>
    <el-button @click="load">刷新网关状态</el-button>
  </header>
  <dl class="gateway-facts">
    <div>
      <dt>302 网关</dt>
      <dd>{{ status?.emby302Enabled==null?'—':status.emby302Enabled?'已启用':'未启用' }}</dd>
    </div>
    <div>
      <dt>原画直链策略</dt>
      <dd>{{ status?.embyDirectPlayOnly==null?'—':status.embyDirectPlayOnly?'仅原始 STRM / 302':'请核对服务端配置' }}</dd>
    </div>
    <div>
      <dt>原全局播放凭据入口</dt>
      <dd>{{ status?.legacyPlaybackEnabled==null?'—':status.legacyPlaybackEnabled?'仍启用，请核对迁移':'已关闭' }}</dd>
    </div>
    <div>
      <dt>媒体中转配置</dt>
      <dd>{{ status?.forceProxy==null?'—':status.forceProxy?'检测到中转配置，请核对直链策略':'未启用媒体中转' }}</dd>
    </div>
    <div>
      <dt>异常回退配置</dt>
      <dd>{{ status?.proxyFallback==null?'—':status.proxyFallback?'检测到代理回退，请核对直链策略':'未启用代理回退' }}</dd>
    </div>
  </dl>
  <el-button :loading="busy" :disabled="unknown" @click="test">检查Emby可达性</el-button>
  <p v-if="message" role="status">{{ message }}</p>
  <el-alert v-if="error" :title="error" type="warning" :closable="false" />
  <p>文件级播放链接携带独立可撤销的权限凭据，分享链接相当于分享此文件的播放权限。授权检查先于缓存；跳转后的 CDN 错误需在播放器端核对。</p>
  <p>实际验收仍需用有权限的播放器检查启动、拖动、续播、字幕与音轨；取到跳转地址不等于播放成功。</p>
</section>
</template>
