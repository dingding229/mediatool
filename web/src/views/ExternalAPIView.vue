<script setup lang="ts">
import { ref, onMounted } from "vue";
import { ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
interface Key {
  id: string;
  name: string;
  token: string;
  scopes: string[];
  expiresAt: string;
  revokedAt?: string;
  lastUsedAt?: string;
}
const keys = ref<Key[]>([]),
  calls = ref<
    {
      name: string;
      method: string;
      path: string;
      status: number;
      createdAt: string;
    }[]
  >([]),
  busy = ref(false),
  error = ref(""),
  dialog = ref(false),
  name = ref(""),
  days = ref(90),
  scopes = ref(["status.read", "library.read", "layout.preview", "tasks.read"]);
const origin = window.location.origin;
const options = [
  { id: "status.read", label: "版本 / 状态只读" },
  { id: "library.read", label: "媒体 / STRM内容只读" },
  { id: "layout.preview", label: "分类与命名预览" },
  { id: "tasks.read", label: "下载 / 上传状态只读" },
  { id: "search.run", label: "创建站点搜索 / 读取结果" },
  { id: "downloads.run", label: "创建真实下载任务（写入）" },
];
async function load() {
  try {
    [keys.value, calls.value] = await Promise.all([
      request<Key[]>("/external-api/keys"),
      request<typeof calls.value>("/external-api/calls"),
    ]);
  } catch (e) {
    error.value = String(e);
  }
}
async function create() {
  busy.value = true;
  error.value = "";
  try {
    await post("/external-api/keys", {
      name: name.value,
      scopes: scopes.value,
      expiresAt: new Date(Date.now() + days.value * 86400000).toISOString(),
      confirm: true,
    });
    dialog.value = false;
    await load();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
async function revoke(k: Key) {
  try {
    await ElMessageBox.confirm(
      "立即停止此凭据后续调用；不删除媒体或任务。",
      "撤销API凭据",
      { type: "warning" },
    );
    await post(`/external-api/keys/${k.id}/revoke`, { confirm: true });
    await load();
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  }
}
onMounted(load);
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">SYSTEM / 外部接入</span>
      <h1>外部API</h1>
      <p>独立凭据、分级权限和有效期。默认只读，不复用管理员密码或播放Key。</p>
    </div>
    <div class="heading-actions">
      <el-button @click="load">刷新记录</el-button
      ><el-button type="primary" @click="dialog = true">创建API凭据</el-button>
    </div>
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <section class="panel">
    <h2>接口验证与调用</h2>
    <p>
      公网请通过HTTPS使用 Authorization:
      Bearer。每个凭据每分钟最多120次；不支持URL参数或后台Cookie替代鉴权。
    </p>
    <pre class="api-code">
curl -H 'Authorization: Bearer YOUR_API_KEY' '{{
        origin
      }}/api/external/v1/status'
GET  /api/external/v1/strm?kind=strm&amp;mediaType=movie
GET  /api/external/v1/strm/{assetId}/content?path=URL编码的相对STRM路径
POST /api/external/v1/layout/preview
GET  /api/external/v1/downloads
GET  /api/external/v1/transfers
POST /api/external/v1/search
GET  /api/external/v1/search/{id}
POST /api/external/v1/downloads</pre
    >
    <p class="field-hint">
      不开放管理员凭据、密码恢复、原始数据库或任意文件读写；允许下载的凭据能产生真实下载，请谨慎分配。
    </p>
  </section>
  <section class="panel section-gap">
    <h2>已保存凭据</h2>
    <article v-for="k in keys" :key="k.id" class="api-key">
      <div>
        <strong>{{ k.name }}</strong
        ><span>{{
          k.revokedAt
            ? "已撤销"
            : "有效期至 " + new Date(k.expiresAt).toLocaleString()
        }}</span
        ><el-button
          :disabled="!!k.revokedAt"
          type="danger"
          plain
          @click="revoke(k)"
          >撤销</el-button
        >
      </div>
      <el-input :model-value="k.token" readonly aria-label="完整API凭据" />
      <p>
        {{ k.scopes.join(" · ") }} · 最近使用
        {{
          k.lastUsedAt ? new Date(k.lastUsedAt).toLocaleString() : "尚未使用"
        }}
      </p>
    </article>
    <el-empty v-if="!keys.length" description="尚未创建独立API凭据" />
  </section>
  <section class="panel">
    <h2>最近调用</h2>
    <div class="table-scroll">
      <el-table :data="calls"
        ><el-table-column prop="name" label="凭据" /><el-table-column
          prop="method"
          label="方法"
          width="90"
        /><el-table-column
          prop="path"
          label="接口"
          min-width="260"
        /><el-table-column
          prop="status"
          label="响应"
          width="90"
        /><el-table-column label="时间" min-width="180"
          ><template #default="s">{{
            new Date(s.row.createdAt).toLocaleString()
          }}</template></el-table-column
        ></el-table
      >
    </div>
  </section>
  <el-dialog v-model="dialog" title="创建独立API凭据" width="min(580px,94vw)"
    ><el-form label-position="top"
      ><el-form-item label="凭据名称"><el-input v-model="name" /></el-form-item
      ><el-form-item label="有效天数"
        ><el-input-number v-model="days" :min="1" :max="366" /></el-form-item
      ><el-form-item label="明确授权的权限"
        ><el-checkbox-group v-model="scopes"
          ><el-checkbox v-for="o in options" :key="o.id" :value="o.id">{{
            o.label
          }}</el-checkbox></el-checkbox-group
        ></el-form-item
      ></el-form
    ><el-alert
      title="凭据代表这些权限；有下载权限将能产生实际业务写入。确认后创建，关闭窗口不会删除已创建凭据。"
      type="warning"
      :closable="false"
    /><template #footer
      ><el-button @click="dialog = false">取消</el-button
      ><el-button :loading="busy" type="primary" @click="create"
        >确认创建</el-button
      ></template
    ></el-dialog
  >
</template>
<style scoped>
.api-code {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 16px;
  border: 1px solid var(--border);
  line-height: 1.8;
}
.api-key {
  padding: 16px 0;
  border-bottom: 1px solid var(--border);
}
.api-key > div {
  display: flex;
  gap: 16px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 12px;
}
.api-key p {
  font-size: 13px;
  overflow-wrap: anywhere;
}
.table-scroll {
  overflow: auto;
}
.el-checkbox-group {
  display: flex;
  flex-direction: column;
}
.el-checkbox {
  margin: 0;
  min-height: 44px;
}
</style>
