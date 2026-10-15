<script setup lang="ts">
import { ref } from "vue";
import {
  pwaPrompt,
  pwaUpdate,
  installPWA,
  activateUpdate,
} from "../pwa/install";
const secure = window.isSecureContext,
  installed =
    window.matchMedia("(display-mode: standalone)").matches ||
    Boolean((navigator as Navigator & { standalone?: boolean }).standalone),
  show = ref(false);
</script>
<template>
  <section class="panel section-gap">
    <div class="panel-title">
      <h2>移动端应用 / PWA</h2>
      <el-tag>{{
        installed
          ? "独立窗口运行"
          : secure
            ? "HTTPS / 本地安全入口"
            : "需 HTTPS"
      }}</el-tag>
    </div>
    <p class="field-hint">
      {{
        secure
          ? "可将工作台添加到主屏幕；只缓存公开界面资源，不离线保存媒体、账号、数据库或日志。"
          : "当前公网HTTP入口不能安装Service Worker。请通过有效HTTPS域名访问后安装；不要求或引导关闭浏览器安全检查。"
      }}
    </p>
    <el-button v-if="pwaPrompt" type="primary" @click="installPWA"
      >安装到主屏幕</el-button
    ><el-button @click="show = !show">安装说明</el-button
    ><el-button v-if="pwaUpdate" type="primary" @click="activateUpdate"
      >应用新版本并刷新</el-button
    >
    <div v-if="show" class="note-panel">
      <strong>安装方式</strong>
      <p>
        Android / Chrome：使用浏览器菜单中的“安装应用”或“添加到主屏幕”。iPhone /
        iPad：在 Safari 中打开HTTPS入口，选择“分享 →
        添加到主屏幕”。是否显示安装按钮由浏览器决定。
      </p>
      <p>
        断网只显示离线说明，不提供离线媒体播放；退出或更改密码仍撤销后台会话。
      </p>
    </div>
  </section>
</template>
