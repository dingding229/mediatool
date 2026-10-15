<script setup lang="ts">
import { computed, ref, onMounted } from "vue";
import { ElMessageBox, ElMessage } from "element-plus";
import { request, post } from "../api/client";
import { useSystemStore } from "../stores/system";
interface Key {
  id: string;
  name: string;
  token: string;
  scopes: string[];
  expiresAt: null;
  createdAt?: string;
  revokedAt?: string;
  lastUsedAt?: string;
}
interface Call {
  name: string;
  method: string;
  path: string;
  status: number;
  createdAt: string;
}
const keys = ref<Key[]>([]),
  calls = ref<Call[]>([]),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  dialog = ref(false),
  name = ref(""),
  copyOpen = ref(false),
  copyValue = ref("");
const scopes = ref([
    "status.read",
    "library.read",
    "layout.preview",
    "tasks.read",
  ]),
  system = useSystemStore();
const origin = computed(
  () => system.status?.publicUrl || window.location.origin,
);
const options = [
  { id: "status.read", label: "版本 / 状态只读" },
  { id: "library.read", label: "媒体 / STRM内容只读" },
  { id: "layout.preview", label: "分类与命名预览" },
  { id: "tasks.read", label: "下载 / 上传状态只读" },
  { id: "search.run", label: "创建站点搜索 / 读取结果" },
  { id: "downloads.run", label: "创建真实下载任务（写入）" },
];
const scopeLabel = (value: string) =>
  options.find((option) => option.id === value)?.label || value;
const date = (value?: string) =>
  value
    ? new Date(value).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })
    : "尚未使用";
async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [keyList, callList] = await Promise.all([
      request<Key[]>("/external-api/keys"),
      request<Call[]>("/external-api/calls"),
    ]);
    keys.value = keyList;
    calls.value = callList;
  } catch (e) {
    error.value = String(e);
  } finally {
    loading.value = false;
  }
}
async function create() {
  if (!name.value.trim() || !scopes.value.length) return;
  busy.value = true;
  error.value = "";
  try {
    await post("/external-api/keys", {
      name: name.value.trim(),
      scopes: scopes.value,
      confirm: true,
    });
    dialog.value = false;
    name.value = "";
    await load();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
async function revoke(key: Key) {
  try {
    await ElMessageBox.confirm(
      "立即停止此凭据后续调用；不删除媒体或任务。",
      "撤销API凭据",
      { type: "warning" },
    );
    await post(`/external-api/keys/${key.id}/revoke`, { confirm: true });
    await load();
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  }
}
async function copyToken(value: string) {
  try {
    if (!navigator.clipboard?.writeText) throw new Error();
    await navigator.clipboard.writeText(value);
    ElMessage.success("完整API凭据已复制");
  } catch {
    copyValue.value = value;
    copyOpen.value = true;
  }
}
onMounted(load);
</script>
<template>
  <div class="operations-stack external-api-page">
    <section class="page-heading">
      <div>
        <span class="page-kicker">SYSTEM / 外部接入</span>
        <h1>外部API</h1>
        <p>
          独立凭据永久有效，按权限调用并可随时撤销，不复用管理员密码或播放Key。
        </p>
      </div>
      <div class="heading-actions">
        <el-button :loading="loading" @click="load">刷新记录</el-button
        ><el-button type="primary" @click="dialog = true"
          >创建API凭据</el-button
        >
      </div>
    </section>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <section class="panel api-keys">
      <div class="panel-title">
        <h2>已保存凭据</h2>
        <span class="muted">{{ keys.length }} 个凭据 · 默认只读权限</span>
      </div>
      <el-skeleton v-if="loading && !keys.length" :rows="3" animated />
      <article v-for="key in keys" :key="key.id" class="api-key">
        <div class="api-key-heading">
          <div>
            <strong>{{ key.name }}</strong
            ><el-tag :type="key.revokedAt ? 'info' : 'success'">{{
              key.revokedAt ? "已撤销" : "永久有效"
            }}</el-tag>
          </div>
          <div class="api-key-actions">
            <el-button @click="copyToken(key.token)">复制凭据</el-button
            ><el-button
              :disabled="!!key.revokedAt"
              type="danger"
              plain
              @click="revoke(key)"
              >撤销</el-button
            >
          </div>
        </div>
        <label :for="`api-key-${key.id}`" class="field-hint">完整API凭据</label
        ><el-input
          :id="`api-key-${key.id}`"
          :model-value="key.token"
          readonly
          aria-label="完整API凭据"
        />
        <div class="api-scopes">
          <el-tag v-for="scope in key.scopes" :key="scope" type="info">{{
            scopeLabel(scope)
          }}</el-tag>
        </div>
        <p class="field-hint">
          最近使用：{{ date(key.lastUsedAt)
          }}<span v-if="key.createdAt"> · 创建：{{ date(key.createdAt) }}</span
          ><span v-if="key.revokedAt"> · 撤销：{{ date(key.revokedAt) }}</span>
        </p>
      </article>
      <el-empty
        v-if="!loading && !error && !keys.length"
        description="尚未创建独立API凭据"
        :image-size="72"
      />
    </section>
    <section class="panel api-guide">
      <details>
        <summary>接口验证与调用</summary>
        <p class="field-hint">
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
      </details>
    </section>
    <section class="panel api-calls">
      <div class="panel-title">
        <h2>最近调用</h2>
        <span class="muted">真实接口调用记录</span>
      </div>
      <div class="table-scroll">
        <el-table :data="calls" empty-text="暂无外部API调用"
          ><el-table-column
            prop="name"
            label="凭据"
            min-width="150"
          /><el-table-column
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
            ><template #default="scope">{{
              date(scope.row.createdAt)
            }}</template></el-table-column
          ></el-table
        >
      </div>
    </section>
  </div>
  <el-dialog v-model="dialog" title="创建独立API凭据" width="min(580px,94vw)"
    ><el-form label-position="top"
      ><el-form-item label="凭据名称" for="external-key-name"
        ><el-input
          id="external-key-name"
          v-model="name"
          maxlength="120" /></el-form-item
      ><el-form-item label="明确授权的权限"
        ><el-checkbox-group v-model="scopes"
          ><el-checkbox
            v-for="option in options"
            :key="option.id"
            :value="option.id"
            >{{ option.label }}</el-checkbox
          ></el-checkbox-group
        ></el-form-item
      ></el-form
    ><el-alert
      title="下载权限可产生真实业务写入。凭据永久有效，直到主动撤销。"
      type="warning"
      :closable="false"
    /><template #footer
      ><el-button @click="dialog = false">取消</el-button
      ><el-button
        :loading="busy"
        :disabled="!name.trim() || !scopes.length"
        type="primary"
        @click="create"
        >确认创建</el-button
      ></template
    ></el-dialog
  >
  <el-dialog v-model="copyOpen" title="复制API凭据" width="min(640px,94vw)"
    ><p class="field-hint">
      当前浏览器不允许自动复制，请选中完整凭据手动复制。
    </p>
    <el-input
      :model-value="copyValue"
      type="textarea"
      :rows="4"
      readonly
      aria-label="待复制API凭据"
    /><template #footer
      ><el-button @click="copyOpen = false">关闭</el-button></template
    ></el-dialog
  >
</template>
<style scoped>
.api-key {
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 8px;
  margin: 0 0 16px;
  min-width: 0;
}
.api-key:last-child {
  margin-bottom: 0;
}
.api-key-heading,
.api-key-heading > div {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.api-key-heading {
  justify-content: space-between;
  margin-bottom: 14px;
}
.api-key-heading strong {
  font-size: 15px;
  overflow-wrap: anywhere;
}
.api-key-actions .el-button {
  margin: 0;
}
.api-scopes {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 12px;
}
.api-scopes .el-tag {
  height: auto;
  min-height: 24px;
  white-space: normal;
  line-height: 1.5;
}
.api-code {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 16px;
  background: var(--el-fill-color-light);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.8;
}
.api-guide summary {
  font-size: 16px;
  font-weight: 600;
  cursor: pointer;
  min-height: 44px;
  display: flex;
  align-items: center;
}
.api-guide summary:before {
  content: "›";
  margin-right: 10px;
}
.api-guide details[open] > summary:before {
  content: "⌄";
}
.table-scroll {
  max-width: 100%;
  overflow: auto;
}
.el-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.el-checkbox {
  margin: 0;
  height: auto;
  min-height: 44px;
  white-space: normal;
}
.el-checkbox :deep(.el-checkbox__label) {
  white-space: normal;
}
@media (max-width: 680px) {
  .api-key {
    padding: 12px;
  }
  .api-key-actions .el-button {
    min-height: 44px;
  }
}
</style>
