<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { request, post } from "../api/client";
import {
  Upload,
  Download,
  Collection,
  Refresh,
  Search,
} from "@element-plus/icons-vue";
import SiteMetricSchedule from "./SiteMetricSchedule.vue";
import StatusBadge from "./StatusBadge.vue";
interface Metric {
  uploaded: number | null;
  downloaded: number | null;
  ratio: number | null;
  bonus: number | null;
  level?: string;
  username?: string;
  seeding: number | null;
  seedingBytes: number | null;
  notes?: string[];
}
interface Row {
  iconUrl?: string;
  accountId: string;
  name: string;
  enabled: boolean;
  metrics: Metric | null;
  collectedAt: string | null;
  attemptedAt: string | null;
  state: string;
  message: string;
  stale: boolean;
}
const iconErrors = ref(new Set<string>());
const emit = defineEmits<{ edit: [id: string] }>();
const rows = ref<Row[]>([]),
  q = ref(""),
  busy = ref(false),
  error = ref(""),
  note = ref("");
const visible = computed(() =>
  rows.value.filter((x) =>
    x.name.toLowerCase().includes(q.value.toLowerCase()),
  ),
);
const enabled = computed(() => rows.value.filter((x) => x.enabled));
const totals = computed(() => [
  ...["uploaded", "downloaded"].map((key) => {
    const valid = enabled.value.filter(
      (x) => typeof x.metrics?.[key as keyof Metric] === "number",
    );
    return {
      key,
      count: valid.length,
      value: valid.length
        ? valid.reduce(
            (sum, x) => sum + Number(x.metrics?.[key as keyof Metric]),
            0,
          )
        : null,
    };
  }),
  { key: "sites", count: rows.value.length, value: rows.value.length },
]);
function bytes(n: number | null | undefined) {
  if (n === null || n === undefined) return "—";
  if (n === 0) return "0 B";
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), 5);
  return (
    (n / 1024 ** i).toFixed(2) +
    " " +
    ["B", "KiB", "MiB", "GiB", "TiB", "PiB"][i]
  );
}
async function load() {
  try {
    rows.value = await request("/sites/metrics");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "指标读取失败";
  }
}
async function sync(id = "") {
  busy.value = true;
  error.value = "";
  note.value = "";
  try {
    const result = await post<{
      attempted: number;
      failed?: number;
      remainingMayExist: boolean;
    }>("/sites/metrics/sync", { accountId: id });
    note.value = `本次尝试${result.attempted}个账号${result.failed ? `，${result.failed}个采集失败` : ""}；请核对每站结果。${result.remainingMayExist ? "最多16站，请单站同步其余账号。" : ""}`;
    await load();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "同步失败";
    await load();
  } finally {
    busy.value = false;
  }
}
let refreshTimer: ReturnType<typeof setInterval> | undefined;
onMounted(() => {
  load();
  refreshTimer = setInterval(() => {
    if (!document.hidden && !busy.value) load();
  }, 30000);
});
onUnmounted(() => clearInterval(refreshTimer));
</script>
<template>
  <section class="metrics-overview">
    <article v-for="(t, index) in totals" :key="t.key" class="panel">
      <component
        :is="[Upload, Download, Collection][index]"
        aria-hidden="true"
      />
      <div>
        <span>{{ ["总上传", "总下载", "已添加站点数"][index] }}</span
        ><strong>{{ index === 2 ? (t.value ?? "—") : bytes(t.value) }}</strong
        ><small v-if="index === 2">全部已添加的 PT 站点</small
        ><small v-else
          >{{ t.count }} / {{ enabled.length }} 个启用账号有可用字段</small
        >
      </div>
    </article>
  </section>
  <section class="panel site-metrics-panel">
    <SiteMetricSchedule />
    <div class="panel-title">
      <div>
        <h2>站点最新数据</h2>
        <p class="field-hint">
          {{ rows.length }} 个账号 ·
          展示最近成功采集快照；失败保留旧数据，未知字段显示“—”。汇总可能包含旧快照，不代表本次同步均成功。
        </p>
      </div>
      <el-button :icon="Refresh" :loading="busy" @click="sync()"
        >同步站点数据</el-button
      >
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <p v-if="note" class="field-hint" role="status">{{ note }}</p>
    <el-input
      v-model="q"
      aria-label="搜索站点名称"
      :prefix-icon="Search"
      placeholder="搜索站点名称"
      clearable
      class="metrics-search"
    />
    <article
      v-for="row in visible"
      :key="row.accountId"
      class="site-metric-row"
    >
      <div class="metric-site-name">
        <span class="site-avatar"
          ><img
            v-if="row.iconUrl && !iconErrors.has(row.accountId)"
            :src="row.iconUrl"
            alt=""
            aria-hidden="true"
            width="32"
            height="32"
            loading="lazy"
            @error="iconErrors.add(row.accountId)"
          /><template v-else>{{ row.name.slice(0, 2) }}</template></span
        >
        <div>
          <h3>{{ row.name }}</h3>
          <p>
            {{ row.metrics?.level || "等级未知"
            }}<span v-if="row.metrics?.username">
              · {{ row.metrics.username }}</span
            >
          </p>
          <small
            >{{
              row.collectedAt
                ? "成功采集 " +
                  new Date(row.collectedAt).toLocaleString("zh-CN")
                : "尚未采集成功"
            }}{{
              !row.enabled ? " · 已停用" : row.stale ? " · 旧快照" : ""
            }}</small
          >
        </div>
      </div>
      <div class="metric-facts">
        <span
          >分享率 <strong>{{ row.metrics?.ratio ?? "—" }}</strong></span
        ><span
          >魔力值 <strong>{{ row.metrics?.bonus ?? "—" }}</strong></span
        ><span
          >上传 <strong>{{ bytes(row.metrics?.uploaded) }}</strong></span
        ><span
          >下载 <strong>{{ bytes(row.metrics?.downloaded) }}</strong></span
        ><span
          >做种数 <strong>{{ row.metrics?.seeding ?? "—" }}</strong></span
        ><span
          >做种体积
          <strong>{{ bytes(row.metrics?.seedingBytes) }}</strong></span
        >
      </div>
      <div class="metric-state">
        <StatusBadge
          :state="row.state"
          :label="
            row.state === 'partial'
              ? '部分字段'
              : row.state === 'succeeded'
                ? '已采集'
                : row.state === 'not_synced'
                  ? '未采集'
                  : undefined
          "
        /><el-button @click="emit('edit', row.accountId)">编辑站点</el-button
        ><el-button
          :disabled="!row.enabled || busy"
          @click="sync(row.accountId)"
          >同步此站</el-button
        >
      </div>
      <p v-if="row.message || row.metrics?.notes?.length" class="metrics-note">
        {{ row.message || row.metrics?.notes?.join("；") }}
      </p>
    </article>
    <el-empty
      v-if="!visible.length"
      description="尚未添加站点，请使用顶部添加站点。"
    />
  </section>
</template>
<style scoped>
.metrics-overview {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.metrics-overview article {
  display: flex;
  align-items: center;
  gap: 18px;
}
.metrics-overview svg {
  width: 32px;
  height: 32px;
  color: var(--accent);
}
.metrics-overview span {
  font-size: 13px;
  color: var(--muted);
}
.metrics-overview strong {
  display: block;
  font-size: 24px;
}
.metrics-overview small {
  font-size: 11px;
  color: var(--muted);
}
.site-metrics-panel {
  margin-bottom: 24px;
}
.metrics-search {
  max-width: 350px;
  margin-bottom: 16px;
}
.site-metric-row {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) minmax(340px, 1.7fr) auto;
  gap: 18px;
  align-items: center;
  padding: 20px 0;
  border-top: 1px solid var(--border);
}
.metric-site-name {
  display: flex;
  align-items: center;
  gap: 14px;
}
.site-avatar img {
  object-fit: contain;
}
.site-avatar {
  display: grid;
  place-items: center;
  flex-shrink: 0;
  width: 44px;
  height: 44px;
  border-radius: 10px;
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 13px;
}
.metric-site-name h3 {
  font-size: 15px;
}
.metric-site-name p,
.metric-site-name small {
  font-size: 12px;
  color: var(--muted);
}
.metric-facts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px 16px;
  font-size: 12px;
}
.metric-facts strong {
  color: var(--accent);
  margin-left: 8px;
  white-space: nowrap;
}
.metric-state {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.metrics-note {
  grid-column: 1/-1;
  font-size: 12px;
  color: var(--muted);
}
@media (max-width: 1100px) {
  .site-metric-row {
    grid-template-columns: 1fr;
  }
  .metric-state {
    flex-direction: row;
    align-items: center;
  }
}
@media (max-width: 680px) {
  .metrics-overview {
    grid-template-columns: 1fr;
  }
  .metric-facts {
    grid-template-columns: 1fr 1fr;
  }
  .metrics-overview article {
    padding: 16px;
  }
}
</style>
