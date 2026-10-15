<script setup lang="ts">
import { onMounted, ref } from "vue";
import { request } from "../api/client";
interface Origins {
  managementUrl: string;
  playbackUrl: string;
  memberUrl: string;
  embyUrl: string;
}
interface Record {
  value: Origins;
  revision: number;
  effective: Origins;
  listeners: { management: string; member: string; emby: string };
}
const record = ref<Record>(),
  busy = ref(false),
  error = ref(""),
  message = ref("");
const fields: [keyof Origins, string, string][] = [
  ["managementUrl", "后台访问域名", "https://admin.example.com"],
  ["playbackUrl", "STRM播放域名", "https://play.example.com"],
  ["memberUrl", "内置媒体入口域名", "https://media.example.com"],
  ["embyUrl", "Emby网关域名", "https://emby.example.com"],
];
async function load() {
  error.value = "";
  try {
    record.value = await request("/settings/access");
  } catch (e) {
    error.value = String(e);
  }
}
async function save() {
  if (!record.value) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    record.value = await request("/settings/access", {
      method: "PUT",
      body: JSON.stringify({
        value: record.value.value,
        revision: record.value.revision,
      }),
    });
    message.value =
      "域名已保存，新生成链接立即使用新地址；已有STRM请先预览全局修复。";
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="panel settings-panel">
    <div class="panel-title">
      <div>
        <h2>访问与域名</h2>
        <p class="field-hint">后台、固定播放链接和媒体用户入口分别配置。</p>
      </div>
      <el-button @click="load" :loading="busy">重新读取域名</el-button>
    </div>
    <el-alert
      v-if="error"
      :title="error"
      type="error"
      :closable="false"
    /><el-alert
      v-if="message"
      :title="message"
      type="success"
      :closable="false"
    />
    <form v-if="record" class="settings-form" @submit.prevent="save">
      <div
        class="settings-field"
        v-for="[key, label, placeholder] in fields"
        :key="key"
      >
        <label :for="`access-${key}`">{{ label }}</label
        ><el-input
          :id="`access-${key}`"
          v-model="record.value[key]"
          :placeholder="placeholder"
          autocomplete="url"
        />
        <p class="field-hint">
          当前生效：<a
            v-if="key !== 'playbackUrl' && record.effective[key]"
            :href="record.effective[key]"
            target="_blank"
            rel="noopener"
            >{{ record.effective[key] }}</a
          ><code v-else>{{ record.effective[key] || "未配置" }}</code>
        </p>
      </div>
      <div class="settings-notice">
        <strong>先配置域名解析、HTTPS与反向代理</strong>
        <p>
          输入完整来源，如 https://media.example.com，可带端口；不要添加
          /watch、其他路径、账号或查询参数。留空使用部署地址，不会修改DNS或自动签发证书。
        </p>
        <p>
          管理端：{{ record.listeners.management }} · 内置媒体：{{
            record.listeners.member || "未启用"
          }}
          · Emby：{{
            record.listeners.emby || "未启用"
          }}。内置媒体使用独立端口的根路径 /，不是管理端的子页面。
        </p>
        <p>
          修改地址不会自动覆盖已有STRM。请在
          <RouterLink to="/strm-tools">STRM工具</RouterLink>
          中预览并确认全局修复；原有部署入口保留，避免配置错误导致无法登录。
        </p>
      </div>
      <div class="settings-actions">
        <el-button type="primary" native-type="submit" :loading="busy"
          >保存域名设置</el-button
        >
      </div>
    </form>
    <el-skeleton v-else-if="!error" :rows="5" animated />
  </section>
</template>
