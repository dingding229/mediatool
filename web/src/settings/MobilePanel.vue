<script setup lang="ts">
import { onUnmounted, ref } from 'vue';
import { pwaPrompt, pwaUpdate, installPWA, activateUpdate } from '../pwa/install';
const secure = window.isSecureContext, mode = window.matchMedia('(display-mode: standalone)'), installed = ref(mode.matches || !!(navigator as any).standalone), show = ref(false), error = ref('');
const update = () => installed.value = mode.matches || !!(navigator as any).standalone;
mode.addEventListener?.('change', update);
onUnmounted(() => mode.removeEventListener?.('change', update));
async function install() {
  try {
    await installPWA();
  }
  catch {
    error.value = '安装未完成，请使用浏览器菜单添加到主屏幕。';
  }
}
</script>
<template>
  <section class="settings-card" aria-label="移动端安装">
    <h2>随时打开媒体工作台</h2>
    <p>{{installed?'正在独立窗口中运行':secure?'当前入口支持安装到主屏幕':'请使用有效的 HTTPS 入口后安装'}}</p>
    <p>仅缓存公开界面，不离线保存媒体、账号、数据库或日志；不需要关闭浏览器安全检查。</p>
    <div class="settings-actions">
      <el-button v-if="pwaPrompt" @click="install">安装到主屏幕</el-button>
      <el-button @click="show=!show">安装说明</el-button>
      <el-button v-if="pwaUpdate" @click="activateUpdate">应用新版本并刷新</el-button>
    </div>
    <p v-if="error" role="alert">{{error}}</p>
    <section v-if="show" class="settings-preview">
      <h3>添加到主屏幕</h3>
      <p>Android / Chrome：打开浏览器菜单，选择“安装应用”或“添加到主屏幕”。</p>
      <p>iPhone / iPad：使用 Safari 打开 HTTPS 入口，选择“分享 → 添加到主屏幕”。</p>
      <p>安装按钮是否可用由浏览器决定。断网只提供离线说明，不提供媒体播放；退出或改密仍会撤销会话。</p>
    </section>
  </section>
</template>
