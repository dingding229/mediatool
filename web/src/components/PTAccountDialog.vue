<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { request } from "../api/client";
import { ElMessage } from "element-plus";
interface Site {
  id: string;
  name: string;
  domain: string;
  loaded: boolean;
  fields: { key: string; label: string; required: boolean; secret: boolean }[];
}
interface Account {
  id: string;
  definitionId: string;
  name: string;
  settings: Record<string, string>;
  values: Record<string, string>;
  revision: number;
}
const props = defineProps<{ modelValue: boolean; editingId?: string }>(),
  emit = defineEmits(["update:modelValue", "saved"]);
const otherSaved = computed(() =>
  Object.fromEntries(
    Object.entries(values.value).filter(
      ([k]) => !["cookie", "userAgent", "rssUrl", "passkey"].includes(k),
    ),
  ),
);
const templates = ref<Site[]>([]),
  accounts = ref<Account[]>([]),
  siteID = ref(""),
  values = ref<Record<string, string>>({}),
  revision = ref(0),
  busy = ref(false),
  error = ref(""),
  rss = ref(""),
  rssState = ref(""),
  userID = ref(""),
  selection = computed(() =>
    templates.value.find((x) => x.id === siteID.value),
  );
let loadSequence = 0;
watch(
  () => props.modelValue,
  async (open) => {
    const sequence = ++loadSequence;
    values.value = {};
    rss.value = "";
    rssState.value = "";
    userID.value = "";
    error.value = "";
    if (!open) {
      busy.value = false;
      return;
    }
    busy.value = true;
    try {
      const [catalog, list, ua] = await Promise.all([
        request<{ templates: Site[] }>("/pt/templates"),
        request<Account[]>("/pt/accounts"),
        request<{ userAgent: string }>("/pt/ua"),
      ]);
      if (sequence !== loadSequence || !props.modelValue) return;
      templates.value = catalog.templates.filter(
        (x) =>
          x.loaded ||
          x.id === list.find((a) => a.id === props.editingId)?.definitionId,
      );
      accounts.value = list;
      const item = list.find((x) => x.id === props.editingId);
      if (item) {
        siteID.value = item.definitionId;
        values.value = {
          ...item.values,
          userAgent: item.settings.userAgent || ua.userAgent,
        };
        revision.value = item.revision;
        rss.value = item.values.rssUrl || "";
        rssState.value = item.settings.rssState || "";
        userID.value = item.settings.userId || "";
      } else {
        siteID.value = "";
        userID.value = "";
        values.value = { userAgent: ua.userAgent };
        revision.value = 0;
      }
    } catch (e) {
      if (sequence === loadSequence && props.modelValue)
        error.value = String(e);
    } finally {
      if (sequence === loadSequence) busy.value = false;
    }
  },
);
async function copyValue(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.warning("当前浏览器不支持复制，请选择字段文本手动复制");
  }
}
function savedLabel(key: string) {
  return (
    (
      {
        token: "Token",
        apiKey: "API Key",
        username: "用户名",
        password: "密码",
      } as Record<string, string>
    )[key] || key
  );
}
async function captureUA() {
  try {
    values.value.userAgent = (
      await request<{ userAgent: string }>("/pt/ua")
    ).userAgent;
  } catch (e) {
    error.value = String(e);
  }
}
async function save() {
  if (!selection.value) return;
  busy.value = true;
  error.value = "";
  try {
    const out = await request<{ rssState: string; rssMessage: string }>(
      "/pt/accounts" + (props.editingId ? "/" + props.editingId : ""),
      {
        method: props.editingId ? "PUT" : "POST",
        body: JSON.stringify({
          definitionId: siteID.value,
          values: {
            cookie: values.value.cookie,
            userAgent: values.value.userAgent,
          },
          revision: revision.value,
        }),
      },
    );
    ElMessage.success("站点已保存");
    if (out.rssState !== "ready") ElMessage.warning(out.rssMessage);
    emit("saved");
    emit("update:modelValue", false);
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <el-dialog
    :model-value="modelValue"
    :title="editingId ? '编辑站点' : '添加站点'"
    width="min(660px,94vw)"
    :close-on-click-modal="false"
    @update:model-value="emit('update:modelValue', $event)"
    ><el-alert v-if="error" :title="error" type="error" :closable="false" />
    <form class="connection-form" @submit.prevent="save">
      <label for="pt-definition">站点</label
      ><el-select
        id="pt-definition"
        v-model="siteID"
        :disabled="!!editingId || busy"
        placeholder="选择站点模板"
        ><el-option
          v-for="site in templates"
          :key="site.id"
          :label="site.name"
          :value="site.id"
          :disabled="
            accounts.some(
              (x) => x.definitionId === site.id && x.id !== editingId,
            )
          "
      /></el-select>
      <p v-if="!templates.length && !busy" class="field-hint">
        尚未加载站点模板，请先到站点模板页面选择加载。
      </p>
      <template v-if="selection"
        ><label
          >站点名称<el-input
            :model-value="selection.name"
            readonly
            aria-label="站点名称" /></label
        ><label
          >站点访问地址<el-input
            :model-value="selection.domain"
            readonly
            aria-label="站点访问地址" /></label
        ><template v-for="field in selection.fields" :key="field.key"
          ><label :for="'pt-' + field.key"
            >{{ field.label }}{{ field.required ? "（必填）" : "" }}</label
          ><el-input
            :id="'pt-' + field.key"
            v-model="values[field.key]"
            :type="field.key === 'cookie' ? 'textarea' : 'text'"
            :rows="3"
            :required="field.required"
            :maxlength="field.key === 'cookie' ? 16384 : 4096"
            autocomplete="off"
          /><el-button
            v-if="field.key === 'userAgent'"
            :disabled="busy"
            @click="captureUA"
            >获取当前UA</el-button
          ></template
        ><label v-if="userID"
          >用户ID<el-input
            :model-value="userID"
            readonly
            aria-label="用户ID" /></label
        ><label
          >RSS订阅URL<el-input
            :model-value="rss"
            readonly
            aria-label="RSS订阅URL"
            placeholder="保存时自动获取，无需手动填写"
        /></label>
        <p v-if="rssState === 'unavailable'" class="field-hint">
          上次未获取RSS；保存时会重新尝试，不会覆盖已保存的地址。
        </p>
        <section v-if="values.passkey" class="saved-field">
          <label for="pt-passkey">Passkey</label
          ><el-input
            id="pt-passkey"
            :model-value="values.passkey"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            readonly
            aria-label="Passkey"
          /><el-button @click="copyValue(values.passkey)"
            >复制 Passkey</el-button
          >
        </section>
        <section
          v-for="(value, key) in otherSaved"
          :key="key"
          class="saved-field"
        >
          <label :for="'pt-saved-' + key">{{ savedLabel(String(key)) }}</label
          ><el-input
            :id="'pt-saved-' + key"
            :model-value="value"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 6 }"
            readonly
            :aria-label="savedLabel(String(key))"
          /><el-button @click="copyValue(value)"
            >复制 {{ savedLabel(String(key)) }}</el-button
          >
        </section>
        <p class="field-hint">
          所有已保存信息原值回显，仅通过管理员会话访问。站点名称、地址由模板确定；保存前自动验证登录，自动获取RSS。
        </p></template
      ><el-button
        type="primary"
        native-type="submit"
        :loading="busy"
        :disabled="!selection"
        >保存站点</el-button
      >
    </form></el-dialog
  >
</template>

<style scoped>
.connection-form,
.saved-field {
  min-width: 0;
}
.saved-field {
  display: grid;
  gap: 8px;
}
.saved-field :deep(textarea) {
  overflow-wrap: anywhere;
  word-break: break-all;
  white-space: pre-wrap;
}
.saved-field :deep(.el-button) {
  justify-self: start;
  margin: 0;
}
.connection-form :deep(.el-input),
.connection-form :deep(.el-select) {
  min-width: 0;
  width: 100%;
}
.connection-form :deep(.el-button) {
  min-height: 44px;
}
</style>
