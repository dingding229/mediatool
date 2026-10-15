<script setup lang="ts">
import { ref, onMounted, watch, computed } from "vue";
import { useRoute } from "vue-router";
import { ElDatePicker, ElMessageBox, ElMessage } from "element-plus";
import { request, post } from "../api/client";
import StatusBadge from "../components/StatusBadge.vue";
import { kindLabels, stepState } from "../files/workbench";
import type { FilePlan } from "../files/types";
const route = useRoute();
const originFilter = ref(""),
  methodFilter = ref("");
interface Entry {
  media?: { title: string; year: string; mediaType: string };
  poster?: string;
  destination?: string;
  category?: string;
  season?: number;
  episodes?: number[];
  origin?: string;
  method?: string;
  id: string;
  kind: string;
  state: string;
  source: string;
  connectionId: string;
  batchId: string;
  message: string;
  createdAt: string;
  updatedAt: string;
  details: {
    plan?: FilePlan;
    completedSteps?: number;
    items?: {
      ordinal: number;
      source: string;
      state: string;
      message: string;
      operationId: string;
    }[];
    phase?: string;
    bytesTransferred?: number;
    size?: number;
    taskId?: string;
    paths?: {
      path: string;
      visible: boolean;
      reason: string;
      itemId: string;
    }[];
    [key: string]: unknown;
  };
}
const items = ref<Entry[]>([]),
  page = ref(1),
  total = ref(0),
  state = ref(""),
  kind = ref(String(route.query.kind ?? "")),
  query = ref(""),
  batchId = ref(String(route.query.batchId ?? "")),
  busy = ref(false),
  error = ref(""),
  detail = ref<Entry | null>(null),
  dialog = ref(false),
  reconciling = ref(false),
  dates = ref<[string, string] | null>(null);
const selected = ref<Entry[]>([]),
  removing = ref(false);
const removable = (item: Entry) =>
  ["file", "batch", "scan", "transfer"].includes(item.kind) &&
  [
    "succeeded",
    "partial",
    "failed",
    "cancelled",
    "expired",
    "imported",
  ].includes(item.state) &&
  !(
    item.kind === "transfer" && !["imported", "cancelled"].includes(item.state)
  );
function select(item: Entry, checked: boolean) {
  selected.value = selected.value.filter(
    (row) => row.id !== item.id || row.kind !== item.kind,
  );
  if (checked) selected.value.push(item);
}
async function remove(entries: Entry[]) {
  if (!entries.length) return;
  try {
    await ElMessageBox.confirm(
      `移除 ${entries.length} 条历史？仅从历史列表移除，媒体、STRM、审计和恢复证据保留。进行中或结果未知的记录不能移除。`,
      "移除整理历史",
      {
        confirmButtonText: "明确移除历史",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
    removing.value = true;
    await post("/library/activity/remove", {
      confirm: true,
      items: entries.map((item) => ({
        id: item.id,
        kind: item.kind,
        updatedAt: item.updatedAt,
      })),
    });
    selected.value = [];
    await load();
    ElMessage.success("历史已从列表移除，媒体和恢复证据未删除");
  } catch (e) {
    if (e instanceof Error) error.value = e.message;
  } finally {
    removing.value = false;
  }
}
let epoch = 0;
async function load() {
  const serial = ++epoch;
  busy.value = true;
  error.value = "";
  try {
    const q = new URLSearchParams({
      page: String(page.value),
      pageSize: "25",
      state: state.value,
      kind: kind.value,
      q: query.value,
      batchId: batchId.value,
      origin: originFilter.value,
      method: methodFilter.value,
    });
    if (dates.value) {
      q.set("from", new Date(dates.value[0]).toISOString());
      q.set("to", new Date(dates.value[1]).toISOString());
    }
    const out = await request<{ items: Entry[]; total: number }>(
      `/library/activity?${q}`,
    );
    if (serial === epoch) {
      items.value = out.items;
      total.value = out.total;
      selected.value = [];
    }
  } catch (e) {
    if (serial === epoch)
      error.value = e instanceof Error ? e.message : "历史读取失败";
  } finally {
    if (serial === epoch) busy.value = false;
  }
}
const stateNames: Record<string, string> = {
  pending: "等待执行",
  running: "执行中",
  succeeded: "已完成",
  partial: "部分完成",
  needs_reconcile: "结果待核对",
  failed: "失败",
  paused: "已暂停",
  cancelled: "已取消",
  verified: "云端已核验",
  imported: "入库完成",
  visible: "Emby已可见",
  conflict: "文件冲突",
  applied: "输出已落盘",
  preview: "待批准",
};
const summary = computed(() => ({
  active: items.value.filter((i) =>
    ["pending", "running", "paused"].includes(i.state),
  ).length,
  attention: items.value.filter((i) =>
    ["needs_reconcile", "conflict", "failed", "partial"].includes(i.state),
  ).length,
  done: items.value.filter((i) =>
    ["succeeded", "imported", "visible", "applied", "verified"].includes(
      i.state,
    ),
  ).length,
}));
function stageFilter(value: string) {
  kind.value = value;
  apply();
}
function itemTitle(item: Entry) {
  if (item.media?.title)
    return `${item.media.title}${item.media.year ? ` (${item.media.year})` : ""}`;
  if (item.source === "." || item.source === "/")
    return kindLabels[item.kind] || "业务记录";
  return (
    item.details.plan?.items?.[0]?.source.path.split("/").pop() ||
    item.source?.split("/").pop() ||
    kindLabels[item.kind] ||
    "业务记录"
  );
}
function inspect(item: Entry) {
  detail.value = item;
  dialog.value = true;
}
async function reconcile() {
  if (!detail.value || !["batch", "file"].includes(detail.value.kind)) return;
  reconciling.value = true;
  error.value = "";
  try {
    await post(
      detail.value.kind === "batch"
        ? `/files/batches/${detail.value.id}/reconcile`
        : `/files/operations/${detail.value.id}/reconcile`,
      {},
    );
    dialog.value = false;
    await load();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "核对失败";
  } finally {
    reconciling.value = false;
  }
}
function apply() {
  page.value = 1;
  void load();
}
watch(
  () => [route.query.kind, route.query.batchId],
  () => {
    kind.value = String(route.query.kind ?? "");
    batchId.value = String(route.query.batchId ?? "");
    apply();
  },
);
onMounted(load);
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ORGANIZE / 历史与恢复</span>
      <h1>文件与入库历史</h1>
      <p>
        按批次追踪扫描、整理、上传、输出及 Emby
        确认。进度、冲突和未知结果独立展示。
      </p>
    </div>
    <el-button :loading="busy" @click="load">刷新历史</el-button>
  </section>
  <el-alert v-if="error" :title="error" type="error" :closable="false" />
  <nav class="history-stages" aria-label="历史阶段快捷筛选">
    <button type="button" :class="{ active: !kind }" @click="stageFilter('')">
      全部阶段</button
    ><button
      v-for="(label, value) in kindLabels"
      :key="value"
      type="button"
      :class="{ active: kind === value }"
      @click="stageFilter(value)"
    >
      {{ label }}
    </button>
  </nav>
  <section class="history-summary">
    <article>
      <span>当前筛选总数</span><strong>{{ total }}</strong>
    </article>
    <article>
      <span>本页进行中</span><strong>{{ summary.active }}</strong>
    </article>
    <article>
      <span>本页需处理</span><strong>{{ summary.attention }}</strong>
    </article>
    <article>
      <span>本页已完成阶段</span><strong>{{ summary.done }}</strong>
    </article>
  </section>
  <section class="panel">
    <el-form
      label-position="top"
      class="history-filters compact-history-filters"
      @submit.prevent="apply"
    >
      <el-form-item label="来源"
        ><el-select v-model="originFilter" aria-label="历史来源" @change="apply"
          ><el-option label="全部来源" value="" /><el-option
            label="下载源"
            value="download" /><el-option
            label="文件管理"
            value="file" /><el-option
            label="系统任务"
            value="system" /></el-select></el-form-item
      ><el-form-item label="方式"
        ><el-select v-model="methodFilter" aria-label="历史方式" @change="apply"
          ><el-option label="全部方式" value="" /><el-option
            label="上传入库"
            value="transfer" /><el-option
            label="文件整理"
            value="file" /><el-option
            label="STRM输出"
            value="output" /><el-option
            label="STRM修复"
            value="strmRepair" /><el-option
            label="本地清理"
            value="cleanup" /></el-select
      ></el-form-item>
      <el-form-item label="状态"
        ><el-select v-model="state" aria-label="历史状态" @change="apply"
          ><el-option label="全部状态" value="" /><el-option
            v-for="value in [
              'pending',
              'running',
              'succeeded',
              'partial',
              'needs_reconcile',
              'failed',
              'paused',
              'cancelled',
              'verified',
              'imported',
              'visible',
              'conflict',
              'applied',
            ]"
            :key="value"
            :value="value"
            :label="stateNames[value] || value" /></el-select
      ></el-form-item>
      <el-form-item label="媒体 / 来源关键词"
        ><el-input
          v-model="query"
          aria-label="历史关键词"
          clearable
          @keyup.enter="apply"
      /></el-form-item>
      <details class="history-advanced">
        <summary>更多筛选：批次与时间</summary>
        <div>
          <el-form-item label="批次ID"
            ><el-input
              v-model="batchId"
              aria-label="历史批次"
              clearable
              @keyup.enter="apply"
          /></el-form-item>
          <el-form-item label="起止时间"
            ><el-date-picker
              v-model="dates"
              type="datetimerange"
              start-placeholder="开始"
              end-placeholder="结束"
              @change="apply"
          /></el-form-item>
        </div>
      </details>
      <el-button :loading="busy" @click="apply">筛选历史</el-button>
    </el-form>
    <div class="history-selection">
      <span class="field-hint"
        >共 {{ total }} 条 · 当前页 {{ items.length }} 条</span
      ><el-button
        type="danger"
        plain
        :disabled="!selected.length || selected.length > 50"
        :loading="removing"
        @click="remove(selected)"
        >移除所选历史（{{ selected.length }}）</el-button
      >
    </div>
    <div v-loading="busy" class="history-list">
      <div class="history-table-heading">
        <span>媒体信息</span><span>文件信息</span><span>来源 / 方式 / 时间</span
        ><span>操作</span>
      </div>
      <article
        v-for="item in items"
        :key="`${item.kind}:${item.id}`"
        class="history-row"
      >
        <div class="history-media">
          <img
            v-if="item.poster"
            :src="item.poster"
            :alt="itemTitle(item)"
            loading="lazy"
          />
          <div v-else class="poster-placeholder" aria-hidden="true">
            {{ kindLabels[item.kind]?.slice(0, 2) }}
          </div>
          <div class="history-identity">
            <div class="history-head">
              <el-checkbox
                v-if="removable(item)"
                :model-value="
                  selected.some(
                    (row) => row.id === item.id && row.kind === item.kind,
                  )
                "
                :aria-label="`选择历史 ${item.id}`"
                @change="select(item, !!$event)"
              /><strong>{{ itemTitle(item) }}</strong>
            </div>
            <div class="history-badges">
              <el-tag size="small">{{ kindLabels[item.kind] }}</el-tag
              ><StatusBadge
                :state="item.state"
                :label="stateNames[item.state] || item.state"
              />
            </div>
            <small v-if="item.episodes?.length"
              >第{{ item.season }}季 · {{ item.episodes.join(",") }}集</small
            ><small v-if="item.category">{{ item.category }}</small>
          </div>
        </div>
        <div class="history-files">
          <div v-if="item.source">
            <small>来源</small><code>{{ item.source }}</code>
          </div>
          <div v-if="item.destination">
            <small>目标</small
            ><code class="target-path">{{ item.destination }}</code>
          </div>
          <template v-if="!item.destination"
            ><div
              v-for="(entry, index) in (item.details.plan?.items ?? []).slice(
                0,
                2,
              )"
              :key="index"
            >
              <small>目标</small><code>{{ entry.target }}</code>
            </div>
            <small v-if="(item.details.plan?.items.length ?? 0) > 2"
              >另有
              {{ (item.details.plan?.items.length ?? 0) - 2 }}
              项，详见步骤详情</small
            ></template
          >
          <p>{{ item.message || "查看步骤证据与恢复入口" }}</p>
          <small v-if="item.batchId" class="batch-id"
            >批次 {{ item.batchId }}</small
          >
        </div>
        <div class="history-origin">
          <span>{{ item.origin || "系统任务" }}</span
          ><small>方式：{{ item.method || kindLabels[item.kind] }}</small
          ><time>{{ new Date(item.updatedAt).toLocaleString("zh-CN") }}</time>
        </div>
        <details class="history-action-menu">
          <summary :aria-label="`历史操作 ${item.id}`">⋮</summary>
          <div class="history-actions">
            <el-button @click="inspect(item)">查看步骤详情</el-button
            ><el-button
              v-if="removable(item)"
              type="danger"
              plain
              :loading="removing"
              @click="remove([item])"
              >移除历史</el-button
            ><span v-else class="field-hint">执行 / 恢复证据受保护</span>
          </div>
        </details>
      </article>
      <el-empty
        v-if="!busy && !items.length"
        description="此范围暂无实际记录"
      />
    </div>
    <el-pagination
      v-model:current-page="page"
      :page-size="25"
      :total="total"
      layout="prev,pager,next,total"
      @current-change="load"
    />
  </section>
  <el-dialog
    v-model="dialog"
    title="步骤证据与恢复"
    width="min(850px,95vw)"
    top="5vh"
    class="history-detail"
  >
    <template v-if="detail"
      ><div class="history-head">
        <strong>{{ kindLabels[detail.kind] }}</strong
        ><StatusBadge :state="detail.state" />
      </div>
      <p class="field-hint">{{ detail.message }}</p>
      <ol v-if="detail.details.plan" class="step-list">
        <li v-for="(step, index) in detail.details.plan.steps" :key="index">
          <StatusBadge
            :state="
              stepState(detail.state, detail.details.completedSteps ?? 0, index)
            "
          /><strong>{{
            step.action === "mkdir" ? "创建目录" : "移动 / 重命名"
          }}</strong
          ><code>{{ step.source.path || "—" }} → {{ step.target }}</code>
        </li>
      </ol>
      <ol v-if="detail.details.items" class="step-list">
        <li v-for="row in detail.details.items" :key="row.ordinal">
          <StatusBadge :state="row.state" /><code>{{ row.source }}</code
          ><span>{{ row.message }}</span>
        </li>
      </ol>
      <div v-if="detail.kind === 'transfer'" class="phase-note">
        <strong>当前阶段：{{ detail.details.phase }}</strong>
        <p>
          {{ detail.details.bytesTransferred }} /
          {{ detail.details.size }} 字节已传输；源SHA1
          {{
            detail.details.sourceSha1 || "未知"
          }}。上传100%不代表云端校验、输出完成或Emby可见。
        </p>
        <RouterLink to="/downloads"
          >进入传输任务处理暂停 / 恢复 / 入库批准</RouterLink
        >
      </div>
      <div
        v-for="entry in detail.details.paths ?? []"
        :key="entry.path"
        class="history-path"
      >
        <code>{{ entry.path }}</code
        ><span>{{ entry.visible ? "可见" : "待确认" }}</span
        ><span>{{ entry.reason }}</span>
      </div>
      <details class="history-raw">
        <summary>查看完整原始证据（JSON）</summary>
        <pre class="evidence-json">{{
          JSON.stringify(detail.details, null, 2)
        }}</pre>
      </details>
      <RouterLink
        v-if="detail.kind === 'strmRepair'"
        :to="`/strm-tools?repairId=${detail.id}`"
        >查看STRM修复批次</RouterLink
      ><RouterLink v-if="detail.kind === 'output'" to="/cloudStorage"
        >进入输出修复与归档批准</RouterLink
      ><RouterLink v-if="detail.kind === 'scan'" to="/cloudStorage"
        >进入扫描控制</RouterLink
      >
    </template>
    <template #footer
      ><el-button @click="dialog = false">关闭</el-button
      ><el-button
        v-if="
          detail &&
          ['batch', 'file'].includes(detail.kind) &&
          ['running', 'needs_reconcile'].includes(detail.state)
        "
        :loading="reconciling"
        @click="reconcile"
        >核对结果（不重放）</el-button
      ></template
    >
  </el-dialog>
</template>
<style scoped>
.history-stages {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 16px;
}
.history-stages button {
  border: 1px solid var(--border);
  background: var(--panel-bg);
  color: var(--muted);
  border-radius: 8px;
  min-height: 40px;
  padding: 6px 14px;
  cursor: pointer;
}
.history-stages .active {
  background: var(--el-color-primary-light-9);
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}
.history-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}
.history-summary article {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  background: var(--panel-bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 12px 16px;
}
.history-summary span {
  font-size: 12px;
  color: var(--muted);
}
.history-summary strong {
  font-size: 22px;
  font-variant-numeric: tabular-nums;
}
.history-filters {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0 12px;
  align-items: end;
}
.history-advanced {
  grid-column: 1/-1;
  margin-bottom: 12px;
}
.history-advanced summary {
  cursor: pointer;
  min-height: 36px;
  font-size: 13px;
}
.history-advanced > div {
  display: grid;
  grid-template-columns: 1fr 2fr;
  gap: 12px;
}
.history-filters :deep(.el-date-editor) {
  width: 100%;
  max-width: 100%;
}
.history-selection {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 16px;
}
.history-table-heading,
.history-row {
  display: grid;
  grid-template-columns:
    minmax(180px, 1.1fr) minmax(0, 2fr) minmax(140px, 0.7fr)
    44px;
  gap: 16px;
  align-items: center;
}
.history-table-heading {
  background: var(--el-fill-color-light);
  color: var(--muted);
  font-size: 12px;
  padding: 10px 12px;
  border-radius: 8px;
}
.history-row {
  padding: 16px 12px;
  border-bottom: 1px solid var(--border);
  position: relative;
}
.history-media {
  display: flex;
  gap: 12px;
  min-width: 0;
  align-items: flex-start;
}
.history-media img,
.poster-placeholder {
  width: 44px;
  height: 66px;
  border-radius: 4px;
  object-fit: cover;
  flex-shrink: 0;
}
.poster-placeholder {
  display: grid;
  place-items: center;
  background: var(--el-fill-color-light);
  color: var(--muted);
  font-size: 12px;
}
.history-identity {
  min-width: 0;
}
.history-head {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
}
.history-head strong {
  font-size: 14px;
  overflow-wrap: anywhere;
}
.history-head :deep(.el-checkbox) {
  margin: 0;
  height: 28px;
}
.history-badges {
  display: flex;
  gap: 6px;
  align-items: center;
  flex-wrap: wrap;
  margin: 6px 0;
}
.history-identity small,
.history-origin small {
  display: block;
  font-size: 12px;
  color: var(--muted);
  line-height: 1.7;
}
.history-files {
  min-width: 0;
  font-size: 12px;
}
.history-files > div {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr);
  gap: 6px;
  margin: 4px 0;
}
.history-files small {
  color: var(--muted);
}
.history-files code {
  overflow-wrap: anywhere;
  line-height: 1.6;
}
.target-path {
  color: var(--el-color-primary);
}
.history-files p {
  margin: 8px 0 0;
  line-height: 1.6;
  color: var(--muted);
  overflow-wrap: anywhere;
}
.batch-id {
  display: block;
  overflow-wrap: anywhere;
  margin-top: 6px;
}
.history-origin {
  font-size: 13px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.history-origin time {
  font-size: 11px;
  color: var(--muted);
}
.history-action-menu {
  position: relative;
  align-self: start;
}
.history-action-menu summary {
  display: grid;
  place-items: center;
  width: 40px;
  height: 40px;
  cursor: pointer;
  font-size: 23px;
  list-style: none;
  border-radius: 8px;
  border: 1px solid var(--border);
}
.history-action-menu summary::-webkit-details-marker {
  display: none;
}
.history-actions {
  position: absolute;
  right: 0;
  top: 44px;
  z-index: 5;
  display: grid;
  gap: 8px;
  min-width: 180px;
  padding: 12px;
  background: var(--panel-bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  box-shadow: 0 6px 20px #0002;
}
.history-actions .el-button {
  margin: 0;
  min-height: 40px;
}
.step-list {
  padding: 0;
  list-style: none;
}
.step-list li {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  padding: 10px 0;
  border-bottom: 1px solid var(--border);
}
.step-list code,
.history-path code {
  overflow-wrap: anywhere;
}
.history-path {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding: 8px 0;
}
.evidence-json {
  max-height: 50vh;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
.history-raw {
  margin: 16px 0;
}
.history-raw summary {
  cursor: pointer;
  min-height: 40px;
}
.phase-note {
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  font-size: 13px;
  overflow-wrap: anywhere;
}
.el-pagination {
  margin-top: 20px;
  max-width: 100%;
  flex-wrap: wrap;
}
@media (max-width: 1100px) {
  .history-table-heading,
  .history-row {
    grid-template-columns: minmax(170px, 0.9fr) minmax(0, 1.4fr) 44px;
  }
  .history-table-heading span:nth-child(3) {
    display: none;
  }
  .history-origin {
    grid-column: 2;
    grid-row: 2;
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
  }
  .history-action-menu {
    grid-column: 3;
    grid-row: 1;
  }
  .history-summary article {
    padding: 10px;
    flex-direction: column;
    align-items: flex-start;
  }
}
@media (max-width: 650px) {
  .history-summary {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .history-summary article {
    flex-direction: row;
    align-items: center;
  }
  .history-filters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .history-advanced > div {
    grid-template-columns: minmax(0, 1fr);
  }
  .history-table-heading {
    display: none;
  }
  .history-row {
    grid-template-columns: minmax(0, 1fr) 44px;
    gap: 10px;
    padding: 16px 0;
  }
  .history-media {
    grid-column: 1;
    grid-row: 1;
  }
  .history-action-menu {
    grid-column: 2;
    grid-row: 1;
  }
  .history-files {
    grid-column: 1/-1;
    grid-row: 2;
  }
  .history-origin {
    grid-column: 1/-1;
    grid-row: 3;
  }
  .history-action-menu summary,
  .history-actions .el-button,
  .history-stages button {
    min-height: 44px;
    min-width: 44px;
  }
  .history-head :deep(.el-checkbox) {
    min-height: 44px;
  }
  .history-summary strong {
    font-size: 20px;
  }
}
</style>
