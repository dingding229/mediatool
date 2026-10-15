<script setup lang="ts">
import { useRoute } from "vue-router";
import {
  ref,
  onMounted,
  reactive,
  computed,
  watch,
  onBeforeUnmount,
} from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
import StatusBadge from "../components/StatusBadge.vue";
import Official115AuthDialog from "../components/Official115AuthDialog.vue";
import { accountHeadersError } from "../sites/account-headers";
import { changedSecrets } from "../files/secrets";
interface Connection {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
  settings: Record<string, string | boolean>;
  secretFields: string[];
  revision: number;
  lastCheckState: string;
  lastCheckMessage: string;
}
interface Field {
  key: string;
  label: string;
  hint?: string;
  secret?: boolean;
}
const props = defineProps<{ allowedKinds: string[]; title: string;configOnly?:boolean }>();
const kinds: Record<string, { label: string; fields: Field[] }> = {
  local: {
    label: "本地文件目录",
    fields: [
      {
        key: "rootPath",
        label: "允许管理的本地根目录",
        hint: "例如 /data/downloads；Docker需挂载此目录。不可填写系统根目录，做种源与托管输出受保护。",
      },
    ],
  },
  open115: {
    label: "115 官方直链（独立播放）",
    fields: [
      {
        key: "appId",
        label: "自己的115开放平台 AppID",
        hint: "先保存为未启用连接，再点击115扫码授权；PKCE无需填写AppSecret。",
      },
      {
        key: "cloudMountPrefix",
        label: "CloudDrive2中的115挂载前缀",
        hint: "例如 /115；/115/Mediatool/电影.mkv 对应115内 /Mediatool/电影.mkv",
      },
      {
        key: "accessToken",
        label: "Access Token（可选手动录入）",
        secret: true,
      },
      {
        key: "refreshToken",
        label: "Refresh Token（可选手动录入）",
        secret: true,
      },
    ],
  },
  clouddrive2: {
    label: "CloudDrive2（文件服务）",
    fields: [
      {
        key: "endpoint",
        label: "原生 gRPC 地址",
        hint: "http://NAS地址:19798 或支持 gRPC 的 HTTPS 地址",
      },
      { key: "cloudRoot", label: "允许的网盘根目录", hint: "/115/MS_Media" },
      {
        key: "token",
        label: "CloudDrive2 API Token",
        hint: "文件重命名/移动需要对应账号的写权限；只读凭据只能浏览。",
        secret: true,
      },
      {
        key: "username",
        label: "CloudDrive2 用户名（可选账号模式）",
        secret: true,
      },
      { key: "password", label: "CloudDrive2 密码", secret: true },
      { key: "caFile", label: "自定义 CA 文件路径（可选）" },
    ],
  },
  qbittorrent: {
    label: "qBittorrent",
    fields: [
      { key: "endpoint", label: "Web API 地址", hint: "http://NAS地址:8080" },
      { key: "username", label: "用户名", secret: true },
      { key: "password", label: "密码", secret: true },
      {
        key: "downloadPath",
        label: "下载器看到的保存目录",
        hint: "/downloads",
      },
      {
        key: "localPath",
        label: "本程序可读取的对应共享目录",
        hint: "必须挂载完成文件，可与下载器路径不同",
      },
      { key: "category", label: "任务分类", hint: "media-tool" },
    ],
  },
  emby: {
    label: "Emby",
    fields: [
      { key: "endpoint", label: "Emby 服务地址", hint: "http://NAS地址:8096" },
      {
        key: "apiKey",
        label: "媒体库刷新 API Key",
        hint: "仅后台刷新使用，不替代播放用户身份",
        secret: true,
      },
      { key: "strmPathPrefix", label: "Emby 看到的 STRM 目录", hint: "/strm" },
    ],
  },
  tmdb: {
    label: "TMDB",
    fields: [
      { key: "token", label: "API Read Access Token", secret: true },
      { key: "language", label: "展示语言", hint: "zh-CN" },
    ],
  },
  telegram: {
    label: "Telegram",
    fields: [
      { key: "botToken", label: "Bot Token", secret: true },
      {
        key: "chatId",
        label: "接收 Chat ID",
        hint: "群组 ID 可为负数；请输入完整字符串",
      },
    ],
  },
  ptsite: {
    label: "PT 站点账号",
    fields: [
      {
        key: "definitionId",
        label: "站点定义 ID",
        hint: "使用内置仓库中的 ID，例如 hdsky、ptchdbits、ssd、ourbits、ubits、ttg",
      },
      {
        key: "endpoint",
        label: "站点访问地址",
        hint: "使用已获授权的站点地址",
      },
      { key: "userId", label: "用户 ID（站点需要时）" },
      { key: "cookie", label: "Cookie", secret: true },
      { key: "apiKey", label: "API Key", secret: true },
      { key: "passkey", label: "Passkey", secret: true },
      {
        key: "rssUrl",
        label: "RSS订阅URL（可含Passkey，加密保存）",
        secret: true,
      },
      {
        key: "requestHeaders",
        label: "附加鉴权头（JSON，加密保存）",
        secret: true,
        hint: '例如 {"Authorization":"Bearer …","X-Csrf-Token":"…"}。只用于当前站点声明的必需头；不会自动取得或刷新。留空保留，填写 {} 清除。请通过HTTPS管理地址填写，数据库加密不替代传输加密。',
      },
    ],
  },
};
const route = useRoute();
const selectedKind = String(route.query.connectionKind ?? "");
const activeKind = ref(
  props.allowedKinds.includes(selectedKind)
    ? selectedKind
    : (props.allowedKinds[0] ?? "emby"),
);
const serviceTabs = computed(() =>
  props.allowedKinds.map((key) => ({
    key,
    label:
      (
        {
          open115: "115 授权",
          clouddrive2: "CloudDrive2",
          emby: "Emby",
          tmdb: "TMDB",
          qbittorrent: "下载器",
          ptsite: "PT 账号",
          telegram: "通知",
        } as Record<string, string>
      )[key] ?? key,
  })),
);
const list = ref<Connection[]>([]),
  loading = ref(false),
  error = ref(""),
  editing = ref(false),
  saving = ref(false),
  testing = ref("");
const filteredList = computed(() =>
  list.value.filter(
    (item) =>
      props.allowedKinds.includes(item.kind) && item.kind === activeKind.value,
  ),
);
const secretVisible = ref<Record<string, boolean>>({});
const secretLoading = ref(false),
  savedSecretFields = ref<string[]>([]),
  revealedSecrets = ref<Record<string, string>>({});
watch(editing, (open) => {
  if (!open) {
    form.secrets = {};
    revealedSecrets.value = {};
    savedSecretFields.value = [];
    secretVisible.value = {};
  }
});
const officialAuth = ref(false),
  officialId = ref(""),
  headersError = ref("");
function authorize115(item: Connection) {
  officialId.value = item.id;
  officialAuth.value = true;
}
const form = reactive({
  id: "",
  kind: "clouddrive2",
  name: "",
  enabled: false,
  settings: {} as Record<string, string | boolean>,
  secrets: {} as Record<string, string>,
  revision: 0,
});
async function refresh() {
  loading.value = true;
  error.value = "";
  try {
    list.value = await request<Connection[]>("/integrations");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "连接加载失败";
  } finally {
    loading.value = false;
  }
}
function create() {
  error.value = "";
  headersError.value = "";
  Object.assign(form, {
    id: "",
    kind: activeKind.value,
    name: "",
    enabled: false,
    settings: {},
    secrets: {},
    revision: 0,
  });
  changeKind();
  savedSecretFields.value = [];
  revealedSecrets.value = {};
  editing.value = true;
}
function changeKind() {
  headersError.value = "";
  form.settings = {};
  form.secrets = {};
  if (form.kind === "clouddrive2") form.settings.cloudRoot = "/115/Media";
  if (form.kind === "open115") form.settings.cloudMountPrefix = "/115";
  if (form.kind === "tmdb") form.settings.language = "zh-CN";
  if (form.kind === "qbittorrent") form.settings.category = "media-tool";
}
function edit(item: Connection) {
  error.value = "";
  headersError.value = "";
  savedSecretFields.value = [...item.secretFields];
  revealedSecrets.value = {};
  Object.assign(form, {
    id: item.id,
    kind: item.kind,
    name: item.name,
    enabled: item.enabled,
    settings: { ...item.settings },
    secrets: {},
    revision: item.revision,
  });
  editing.value = true;
  void loadSecrets();
}
async function loadSecrets() {
  const id = form.id,
    revision = form.revision,
    fields =
      kinds[form.kind]?.fields
        .filter(
          (field) =>
            field.secret && savedSecretFields.value.includes(field.key),
        )
        .map((field) => field.key) ?? [];
  if (!id || !fields.length) return;
  secretLoading.value = true;
  try {
    const out = await post<{
      revision: number;
      values: Record<string, string>;
    }>(`/integrations/${id}/secrets/reveal`, {
      revision,
      fields,
      confirm: true,
    });
    if (!editing.value || form.id !== id || form.revision !== revision) return;
    for (const [key, value] of Object.entries(out.values)) {
      if (!form.secrets[key]) {
        form.secrets[key] = value;
        revealedSecrets.value[key] = value;
      }
    }
    ElMessage.success("凭据已回显；未修改的值不会重复提交");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "凭据读取失败";
  } finally {
    secretLoading.value = false;
  }
}
async function save(startAuth = false) {
  headersError.value =
    form.kind === "ptsite"
      ? accountHeadersError(form.secrets.requestHeaders ?? "")
      : "";
  if (headersError.value) return;
  saving.value = true;
  try {
    const secrets = changedSecrets(form.secrets, revealedSecrets.value);
    const input = {
      kind: form.kind,
      name: form.name,
      enabled: savedSecretFields.value.length>0 || Object.values(form.secrets).some(v=>!!v),
      settings: form.settings,
      secrets,
      revision: form.revision,
    };
    const record = await request<Connection>(
      form.id ? `/integrations/${form.id}` : "/integrations",
      { method: form.id ? "PUT" : "POST", body: JSON.stringify(input) },
    );
    editing.value = false;
    ElMessage.success(
      record.kind === "open115" && !record.enabled
        ? "115连接已保存；无需手动Token，请扫码授权后再启用"
        : record.lastCheckState === "ready"
          ? "连接已自动验证并加密保存"
          : "配置草稿已保存；补全凭据后保存时自动验证",
    );
    await refresh();
    if (startAuth && record.kind === "open115") authorize115(record);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "保存失败";
  } finally {
    saving.value = false;
  }
}

async function remove(item: Connection) {
  try {
    await ElMessageBox.confirm(
      "删除该连接会使依赖它的业务不可用。继续删除？",
      "删除连接",
      {
        type: "warning",
        confirmButtonText: "删除连接",
        cancelButtonText: "取消",
      },
    );
    await request(`/integrations/${item.id}`, { method: "DELETE" });
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
onBeforeUnmount(() => {
  form.secrets = {};
  revealedSecrets.value = {};
  savedSecretFields.value = [];
  secretVisible.value = {};
});
onMounted(refresh);
</script>
<template>
  <section class="page-heading">
    <div>
      <h2>{{ title }}</h2>
      <p>
        新增或修改时自动完成连接验证，验证通过后保存；115 可先保存授权草稿。
      </p>
    </div>
    <div class="heading-actions">
      <el-button :loading="loading" @click="refresh">刷新</el-button
      ><el-button type="primary" @click="create">添加连接</el-button>
    </div>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
  />
  <nav class="service-tabs" aria-label="独立服务配置">
    <button
      v-for="tab in serviceTabs"
      :key="tab.key"
      :class="{ active: activeKind === tab.key }"
      :aria-pressed="activeKind === tab.key"
      @click="activeKind = tab.key"
    >
      {{ tab.label }}
    </button>
  </nav>
  <el-alert
    v-if="activeKind !== 'all'"
    :title="`${serviceTabs.find((tab) => tab.key === activeKind)?.label} 独立配置：仅显示本服务连接，不影响其他服务。`"
    type="info"
    :closable="false"
  />
  <section class="panel">
    <div class="panel-title">
      <h2>已保存连接</h2>
      <span class="muted">单实例服务自动保持唯一启用连接，PT账号可多个</span>
    </div>
    <el-table
      :data="filteredList"
      empty-text="尚未配置服务。添加连接并验证后再启用。"
      ><el-table-column label="名称" min-width="180"
        ><template #default="scope"
          ><strong>{{ scope.row.name }}</strong
          ><small class="table-id">{{
            kinds[scope.row.kind]?.label
          }}</small></template
        ></el-table-column
      ><el-table-column v-if="!configOnly" label="启用" width="90"
        ><template #default="scope">{{
          scope.row.enabled ? "已启用" : "未启用"
        }}</template></el-table-column
      ><el-table-column v-if="!configOnly" label="连接验证" min-width="230"
        ><template #default="scope"
          ><StatusBadge
            :state="
              scope.row.lastCheckState === 'unchecked'
                ? 'not_configured'
                : scope.row.lastCheckState
            "
          />
          <p class="field-hint">
            {{ scope.row.lastCheckMessage || "尚未验证" }}
          </p></template
        ></el-table-column
      ><el-table-column label="操作" min-width="330"
        ><template #default="scope"
          ><el-button size="small" @click="edit(scope.row)">编辑</el-button
          ><el-button
            v-if="scope.row.kind === 'open115'"
            size="small"
            @click="authorize115(scope.row)"
            >115扫码授权</el-button
          ><el-button
            size="small"
            type="danger"
            plain
            @click="remove(scope.row)"
            >删除</el-button
          ></template
        ></el-table-column
      ></el-table
    >
  </section>
  <el-dialog
    v-model="editing"
    class="connection-dialog"
    top="5vh"
    :title="form.id ? '编辑连接' : '添加连接'"
    width="min(620px, 92vw)"
    :close-on-click-modal="false"
    ><el-alert v-if="error" :title="error" type="error" :closable="false" />
    <form @submit.prevent="save()" class="connection-form">
      <label>服务类型</label
      ><el-tag v-if="activeKind !== 'all'">{{ kinds[form.kind]?.label }}</el-tag
      ><el-select
        v-else
        v-model="form.kind"
        :disabled="!!form.id"
        @change="changeKind"
        aria-label="服务类型"
        ><el-option
          v-for="key in allowedKinds"
          :key="key"
          :label="kinds[key]?.label"
          :value="key" /></el-select
      ><el-button
        v-if="form.id && savedSecretFields.length"
        :loading="secretLoading"
        :disabled="saving"
        @click="loadSecrets"
        >读取已保存密钥</el-button
      ><label for="connection-name">连接名称</label
      ><el-input
        id="connection-name"
        v-model="form.name"
        placeholder="例如：家庭NAS"
      />
      <template v-for="field in kinds[form.kind]?.fields" :key="field.key"
        ><label :for="`connection-${field.key}`">{{ field.label }}</label>
        <div v-if="field.secret" class="secret-input-row">
          <el-input
            :id="`connection-${field.key}`"
            v-model="form.secrets[field.key]"
            :type="secretVisible[field.key] === false ? 'password' : 'text'"
            :placeholder="form.id ? '留空保留原凭据' : '请输入凭据'"
            autocomplete="off"
            :aria-invalid="field.key === 'requestHeaders' && !!headersError"
            :aria-describedby="
              field.key === 'requestHeaders'
                ? 'account-headers-hint account-headers-error'
                : undefined
            "
            @input="field.key === 'requestHeaders' && (headersError = '')"
          /><el-button
            :aria-label="`${secretVisible[field.key] === false ? '显示' : '隐藏'}${field.label}`"
            @click="
              secretVisible[field.key] = secretVisible[field.key] === false
            "
            >{{
              secretVisible[field.key] === false ? "显示" : "隐藏"
            }}</el-button
          >
        </div>
        <el-input
          v-else
          :id="`connection-${field.key}`"
          v-model="form.settings[field.key]"
          :placeholder="field.hint ?? ''"
        />
        <p
          v-if="field.hint"
          :id="
            field.key === 'requestHeaders' ? 'account-headers-hint' : undefined
          "
          class="field-hint"
        >
          {{ field.hint }}
        </p>
        <p
          v-if="field.key === 'requestHeaders'"
          id="account-headers-error"
          role="alert"
          class="headers-error"
        >
          {{ headersError }}
        </p></template
      >
      <div v-if="form.kind === 'emby'">
        <el-checkbox v-model="form.settings.gatewayEnabled"
          >启用静态流302拦截</el-checkbox
        ><el-checkbox v-model="form.settings.directPlayOnly"
          >仅直链播放：禁止转码和媒体中转</el-checkbox
        >
        <p class="field-hint">
          开启后必须启用302拦截。HLS、重新封装和代理回退均被拒绝；不支持原始TS/MKV或音频格式的客户端将提示无法播放，请改用支持原格式的原生播放器。
        </p>
      </div>
      <el-alert
        v-if="form.kind === 'open115'"
        title="启用后：扫描和上传仍走CloudDrive2，播放取链走115官方API。官方取链失败会明确报错，不会悄悄中转整部影片。保存并扫码后自动验证授权，再启用。"
        type="warning"
        :closable="false"
      /><p class="field-hint">保存并验证通过后自动启用；未完成115授权时保持草稿。</p><el-alert
        title="编辑时自动回显已保存凭据，可按需隐藏。留空或未修改的回显值保留服务端最新凭据；公网请使用HTTPS，关闭窗口后清空内存。"
        type="info"
        :closable="false"
      />
      <div class="save-actions">
        <el-button native-type="submit" type="primary" :loading="saving"
          >加密保存</el-button
        ><el-button
          v-if="form.kind === 'open115'"
          :disabled="saving"
          @click="save(true)"
          >保存并扫码授权</el-button
        >
      </div>
    </form></el-dialog
  >
  <Official115AuthDialog
    v-model="officialAuth"
    :integration-id="officialId"
    @authorized="refresh"
  />
  <section v-if="!configOnly" class="note-panel">
    <strong>连接不等于业务验收</strong>
    <p>
      验证仅检查认证和只读接口。PT真实搜索、上传、Emby客户端播放应在相应业务页面逐项验收。需要公网接入时使用HTTPS。
    </p>
  </section>
</template>

<style scoped>
.secret-input-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.secret-input-row .el-input {
  min-width: 0;
  flex: 1;
}
.service-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin: 0 0 20px;
}
.service-tabs button {
  min-height: 44px;
  padding: 8px 14px;
  background: var(--panel-bg, #fff);
  color: var(--text);
  border: 1px solid var(--border);
  border-radius: 6px;
  cursor: pointer;
}
.service-tabs button.active {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.service-tabs button:focus-visible {
  outline: 2px solid var(--el-color-primary);
  outline-offset: 2px;
}
.save-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.connection-form {
  padding-bottom: 12px;
}
.headers-error {
  margin: 0;
  color: var(--el-color-danger);
  font-size: 13px;
  line-height: 1.6;
}
.headers-error:empty {
  display: none;
}
:global(.connection-dialog) {
  display: flex;
  flex-direction: column;
  max-height: 90dvh;
}
:global(.connection-dialog .el-dialog__body) {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}
</style>
