<script setup lang="ts">
import { onMounted, ref } from "vue";
import { request } from "../api/client";
interface Proxy {
  url: string;
  tmdb: boolean;
  sites: boolean;
  revision: number;
}
const record = ref<Proxy>({ url: "", tmdb: false, sites: false, revision: 1 }),
  loaded = ref(false),
  busy = ref(false),
  error = ref(""),
  message = ref("");
onMounted(async () => {
  try {
    record.value = await request("/settings/network-proxy");
    loaded.value = true;
  } catch (e) {
    error.value = String(e);
  }
});
async function save() {
  if (!record.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    record.value = await request("/settings/network-proxy", {
      method: "PUT",
      body: JSON.stringify(record.value),
    });
    message.value = "代理设置已保存，新的请求使用所选路由。";
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="panel section-gap">
    <div class="panel-title">
      <h2>网络代理</h2>
      <span class="muted">独立模块路由</span>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <form v-if="loaded" class="connection-form" @submit.prevent="save">
      <label for="outbound-proxy-url">代理地址</label
      ><el-input
        id="outbound-proxy-url"
        v-model="record.url"
        aria-label="代理地址"
        autocomplete="off"
        placeholder="http://proxy:7890 或 socks5://proxy:1080"
      />
      <p class="field-hint">
        支持HTTP、HTTPS、SOCKS5，可在地址中包含用户名密码。地址加密存储，仅管理员回显。
      </p>
      <div class="proxy-scopes">
        <el-checkbox v-model="record.tmdb">TMDB查询与海报使用代理</el-checkbox
        ><el-checkbox v-model="record.sites"
          >PT站点搜索、RSS与站点请求使用代理</el-checkbox
        >
      </div>
      <p class="field-hint">
        关闭的模块直连；不会影响115取链、Emby、CloudDrive2或下载器。所选代理不可达时明确报错，不静默回退直连。
      </p>
      <p v-if="message" role="status">{{ message }}</p>
      <el-button type="primary" native-type="submit" :loading="busy"
        >保存代理设置</el-button
      >
    </form>
  </section>
</template>
<style scoped>
.proxy-scopes {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.proxy-scopes :deep(.el-checkbox) {
  min-height: 44px;
  height: auto;
  white-space: normal;
}
.proxy-scopes :deep(.el-checkbox__label) {
  white-space: normal;
}
</style>
