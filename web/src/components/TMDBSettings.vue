<script setup lang="ts">
import { onMounted, ref } from "vue";
import { request, post } from "../api/client";
interface Connection {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
  revision: number;
  settings: Record<string, string>;
  secretFields: string[];
}
const record = ref<Connection>(),
  token = ref(""),
  busy = ref(false),
  loaded = ref(false),
  error = ref(""),
  message = ref("");
async function load() {
  busy.value = true;
  error.value = "";
  message.value = "";
  loaded.value = false;
  try {
    const list = await request<Connection[]>("/integrations");
    const items = list.filter((item) => item.kind === "tmdb");
    record.value = items.find((item) => item.enabled) || items[0];
    token.value = "";
    if (record.value?.secretFields.includes("token")) {
      const secret = await post<{
        values: { token?: string };
        revision: number;
      }>(`/integrations/${record.value.id}/secrets/reveal`, {
        revision: record.value.revision,
        fields: ["token"],
        confirm: true,
      });
      if (secret.revision !== record.value.revision)
        throw new Error("配置已变化，请重新读取");
      token.value = secret.values.token || "";
    }
    loaded.value = true;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "TMDB配置读取失败";
  } finally {
    busy.value = false;
  }
}
async function save() {
  if (!loaded.value || !token.value.trim()) return;
  busy.value = true;
  error.value = "";
  message.value = "";
  try {
    const value = await request<Connection>(
      record.value ? `/integrations/${record.value.id}` : "/integrations",
      {
        method: record.value ? "PUT" : "POST",
        body: JSON.stringify({
          kind: "tmdb",
          name: record.value?.name || "TMDB",
          enabled: true,
          revision: record.value?.revision || 0,
          settings: { language: record.value?.settings.language || "zh-CN" },
          secrets: { token: token.value.trim() },
        }),
      },
    );
    record.value = value;
    token.value = token.value.trim();
    message.value =
      "TMDB Token已保存；连接已在保存时验证。后续请求使用此配置。";
  } catch (e) {
    error.value =
      e instanceof Error ? e.message : "保存失败，请核对Token和网络代理";
  } finally {
    busy.value = false;
  }
}
onMounted(load);
</script>
<template>
  <section class="panel settings-panel tmdb-settings-panel">
    <div class="panel-title">
      <div>
        <h2>TMDB配置</h2>
        <p class="field-hint">
          仅配置一个 API Read Access Token，不需要连接名称或额外的API Key。
        </p>
      </div>
      <el-button :loading="busy" @click="load">重新读取Token</el-button>
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
    <form v-if="loaded" class="settings-form" @submit.prevent="save">
      <div class="settings-field">
        <label for="tmdb-single-token">TMDB API Token</label
        ><el-input
          id="tmdb-single-token"
          v-model="token"
          type="textarea"
          :rows="4"
          autocomplete="off"
          placeholder="填写TMDB的API Read Access Token"
        />
        <p class="field-hint">
          已保存Token完整回显。保存时自动验证；不会清理已有搜索缓存。查询语言由程序保持原配置，首次配置使用简体中文。
        </p>
      </div>
      <div class="settings-actions">
        <el-button
          native-type="submit"
          type="primary"
          :disabled="!token.trim()"
          :loading="busy"
          >保存TMDB Token</el-button
        >
      </div>
    </form>
    <el-skeleton v-else-if="busy" :rows="3" animated />
  </section>
</template>
