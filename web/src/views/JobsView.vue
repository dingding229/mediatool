<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { ElMessage } from "element-plus";
import { Refresh, VideoPlay } from "@element-plus/icons-vue";
import { request, post } from "../api/client";
import StatusBadge from "../components/StatusBadge.vue";
interface Job {
  id: string;
  kind: string;
  state: string;
  queue: string;
  attempts: number;
  maxAttempts: number;
  createdAt: string;
  updatedAt: string;
  availableAt: string;
  leaseUntil: string | null;
  lockedBy: string | null;
  lastError?: string;
  payload: unknown;
}
interface Snapshot {
  sampledAt: string;
  postgres: { state: string; latencyMs: number; message?: string };
  redis: { state: string; latencyMs: number; message?: string };
  migration: number | null;
  databaseBytes: number | null;
  pendingNotifications: number | null;
  pool: Record<string, number> | null;
  process: Record<string, number>;
  workers: {
    owner: string;
    queue: string;
    lastSeenAt: string;
    online: boolean;
  }[];
  queues: {
    name: string;
    state: string;
    count: number;
    expiredLeases: number;
  }[];
  schedules: {
    id: string;
    name: string;
    cron: string;
    enabled: boolean;
    nextRunAt: string | null;
    lastRunAt?: string;
    error?: string;
    link: string;
    timezone: string;
  }[];
  unavailable: string[];
}
const jobs = ref<Job[]>([]),
  snapshot = ref<Snapshot | null>(null),
  error = ref(""),
  healthError = ref(""),
  loading = ref(false),
  creating = ref(false),
  page = ref(1),
  total = ref(0),
  state = ref(""),
  kind = ref(""),
  queue = ref(""),
  query = ref(""),
  detail = ref<Job | null>(null),
  dialog = ref(false);
let poll: ReturnType<typeof setInterval> | undefined,
  epoch = 0,
  stopped = false;
const date = (value?: string | null) =>
  value
    ? new Date(value).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" })
    : "—";
const bytes = (value?: number | null) =>
  value == null
    ? "—"
    : value >= 1073741824
      ? `${(value / 1073741824).toFixed(2)} GiB`
      : `${(value / 1048576).toFixed(1)} MiB`;
const queueLabels: Record<string, string> = {
  default: "主任务",
  library: "媒体入库",
  transfer: "文件传输",
  maintenance: "系统维护",
};
const stateLabels: Record<string, string> = {
  pending: "等待中",
  running: "执行中",
  retry: "等待重试",
  succeeded: "已完成",
  failed: "失败",
  paused: "已暂停",
  cancelled: "已取消",
};
const queueLabel = (value: string) => queueLabels[value] || value;
const stateLabel = (value: string) => stateLabels[value] || value;
async function refreshJobs() {
  const serial = ++epoch;
  loading.value = true;
  try {
    const q = new URLSearchParams({
      page: String(page.value),
      pageSize: "25",
      state: state.value,
      kind: kind.value,
      queue: queue.value,
      q: query.value,
    });
    const out = await request<{ items: Job[]; total: number }>(
      `/jobs/browse?${q}`,
    );
    if (serial === epoch && !stopped) {
      jobs.value = out.items;
      total.value = out.total;
      error.value = "";
    }
  } catch (e) {
    if (serial === epoch) error.value = String(e);
  } finally {
    if (serial === epoch) loading.value = false;
  }
}
async function refreshHealth() {
  try {
    const out = await request<Snapshot>("/system/operations");
    if (!stopped) {
      snapshot.value = out;
      healthError.value = "";
    }
  } catch (e) {
    healthError.value = String(e);
  }
}
async function refresh() {
  await Promise.all([refreshJobs(), refreshHealth()]);
}
function filter() {
  page.value = 1;
  void refreshJobs();
}
async function create() {
  creating.value = true;
  try {
    await post("/jobs", { kind: "system.check" });
    ElMessage.success("系统检查已加入持久化队列");
    await refresh();
  } catch (e) {
    ElMessage.error(String(e));
  } finally {
    creating.value = false;
  }
}
async function inspect(id: string) {
  try {
    detail.value = await request<Job>(`/jobs/${id}`);
    dialog.value = true;
  } catch (e) {
    error.value = String(e);
  }
}
onMounted(() => {
  void refresh();
  let ticks = 0;
  poll = setInterval(() => {
    if (document.hidden) return;
    if (!loading.value) void refreshJobs();
    if (++ticks % 3 === 0) void refreshHealth();
  }, 5000);
});
onUnmounted(() => {
  stopped = true;
  epoch++;
  if (poll) clearInterval(poll);
});
</script>
<template>
  <div class="operations-stack">
    <section class="page-heading">
      <div>
        <span class="page-kicker">SYSTEM / 运行与调度</span>
        <h1>系统运维与后台任务</h1>
        <p>
          依赖探测、Worker 心跳、Cron 调度和持久化任务。任务每 5 秒、运行状态每
          15 秒刷新，不重放业务。
        </p>
      </div>
      <div class="heading-actions">
        <RouterLink class="el-button" to="/maintenance"
          >备份与维护中心</RouterLink
        >
        <el-button :loading="loading" @click="refresh"
          ><Refresh aria-hidden="true" />刷新</el-button
        ><el-button type="primary" :loading="creating" @click="create"
          ><VideoPlay aria-hidden="true" />运行系统检查</el-button
        >
      </div>
    </section>
    <el-alert
      v-if="healthError || error"
      :title="healthError || error"
      type="error"
      :closable="false"
      class="section-gap"
    />
    <el-alert
      v-if="snapshot?.unavailable.length"
      :title="`部分运维数据不可用：${snapshot.unavailable.join('、')}`"
      type="warning"
      :closable="false"
      class="section-gap"
    />
    <template v-if="snapshot"
      ><section class="ops-metrics" aria-label="系统运行状态">
        <article class="panel">
          <span>PostgreSQL</span
          ><strong>{{
            snapshot.postgres.state === "ready" ? "连接正常" : "不可用"
          }}</strong
          ><small>{{
            snapshot.postgres.state === "ready"
              ? `${snapshot.postgres.latencyMs} ms · 数据版本 ${snapshot.migration ?? "未知"}`
              : snapshot.postgres.message || "未配置"
          }}</small>
        </article>
        <article class="panel">
          <span>Redis</span
          ><strong>{{
            snapshot.redis.state === "ready" ? "连接正常" : "不可用"
          }}</strong
          ><small>{{
            snapshot.redis.state === "ready"
              ? `${snapshot.redis.latencyMs} ms`
              : snapshot.redis.message || "未配置"
          }}</small>
        </article>
        <article class="panel">
          <span>主库大小 / 连接池</span
          ><strong>{{ bytes(snapshot.databaseBytes) }}</strong
          ><small>{{
            snapshot.pool
              ? `占用 ${snapshot.pool.acquired} / 上限 ${snapshot.pool.max} · 空闲 ${snapshot.pool.idle}`
              : "连接池不可用"
          }}</small>
        </article>
        <article class="panel">
          <span>应用进程 / 待处理通知</span
          ><strong>{{ bytes(snapshot.process.heapBytes) }}</strong
          ><small
            >{{ snapshot.process.goroutines }} 个后台协程 · 通知
            {{ snapshot.pendingNotifications ?? "未知" }} 条</small
          >
        </article>
      </section>
      <section class="panel section-gap">
        <div class="panel-title">
          <h2>Worker 与队列</h2>
          <span class="muted">采样 {{ date(snapshot.sampledAt) }}</span>
        </div>
        <el-alert
          v-if="
            !snapshot.workers.some((w) => w.online && w.queue === 'default')
          "
          title="默认 Worker 未在线，调度与业务处理可能停止。请查看日志及容器状态。"
          type="warning"
          :closable="false"
        />
        <div class="workers">
          <article
            v-for="w in snapshot.workers"
            :key="w.owner"
            class="worker-row"
          >
            <div class="worker-heading">
              <el-tag :type="w.online ? 'success' : 'warning'">{{
                w.online ? "在线" : "历史心跳已过期"
              }}</el-tag
              ><strong>{{ queueLabel(w.queue) }}</strong
              ><time>{{ date(w.lastSeenAt) }}</time>
            </div>
            <code>{{ w.owner }}</code
            ><small class="muted">队列标识：{{ w.queue }}</small>
          </article>
          <p v-if="!snapshot.workers.length" class="muted">
            尚无 Worker 心跳记录，不能视为运行正常。
          </p>
        </div>
        <div class="queue-chips">
          <button
            v-for="q in snapshot.queues"
            :key="q.name + q.state"
            type="button"
            @click="
              queue = q.name;
              state = q.state;
              filter();
            "
          >
            <code>{{ q.name }}</code
            ><StatusBadge :state="q.state" /><strong>{{ q.count }}</strong
            ><span v-if="q.expiredLeases" class="danger"
              >{{ q.expiredLeases }} 条租约逾期</span
            >
          </button>
          <p v-if="!snapshot.queues.length" class="muted">
            队列暂无任务；可用性以依赖探测为准。
          </p>
        </div>
      </section>
      <section class="panel section-gap">
        <div class="panel-title">
          <h2>Cron 调度</h2>
          <span class="muted"
            >Asia/Shanghai · 业务最多300条 / 插件最多100个</span
          >
        </div>
        <p class="field-hint">
          业务表达式为五字段；核心协调为六字段（含秒）。下次时间为数据库到期时间，不代表已执行；停用项不派发。心跳不等于单次调度成功。
        </p>
        <div class="ops-table">
          <table>
            <thead>
              <tr>
                <th>调度</th>
                <th>表达式 / 状态</th>
                <th>下次到期 / 最近派发</th>
                <th>设置</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="s in snapshot.schedules" :key="s.id">
                <td>
                  {{ s.name
                  }}<small v-if="s.error" class="schedule-error">{{
                    s.error
                  }}</small>
                </td>
                <td>
                  <code>{{ s.cron || "未配置" }}</code
                  ><small>{{ s.enabled ? "已启用" : "已停用" }}</small>
                </td>
                <td>
                  {{ date(s.nextRunAt)
                  }}<small v-if="s.lastRunAt"
                    >最近派发 {{ date(s.lastRunAt) }}</small
                  >
                </td>
                <td>
                  <RouterLink :to="s.link" class="inline-link">查看</RouterLink>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section></template
    >
    <section class="panel section-gap">
      <div class="panel-title">
        <h2>后台任务</h2>
        <span class="muted">共 {{ total }} 条 · PostgreSQL 持久化</span>
      </div>
      <el-form class="job-filters" label-position="top" @submit.prevent="filter"
        ><el-form-item label="状态"
          ><el-select v-model="state" aria-label="任务状态" @change="filter"
            ><el-option label="全部状态" value="" /><el-option
              v-for="s in [
                'pending',
                'running',
                'retry',
                'succeeded',
                'failed',
                'paused',
                'cancelled',
              ]"
              :key="s"
              :label="stateLabel(s)"
              :value="s" /></el-select></el-form-item
        ><el-form-item label="队列"
          ><el-input
            v-model="queue"
            aria-label="任务队列"
            clearable
            @keyup.enter="filter" /></el-form-item
        ><el-form-item label="类型"
          ><el-input
            v-model="kind"
            aria-label="任务类型"
            clearable
            @keyup.enter="filter" /></el-form-item
        ><el-form-item label="任务ID / 类型 / 错误"
          ><el-input
            v-model="query"
            aria-label="任务关键词"
            clearable
            :maxlength="200"
            @keyup.enter="filter" /></el-form-item
        ><el-button native-type="submit">筛选任务</el-button></el-form
      >
      <div class="ops-table">
        <table v-loading="loading">
          <thead>
            <tr>
              <th>任务</th>
              <th>状态 / 尝试</th>
              <th>时间</th>
              <th>最近错误</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="j in jobs" :key="j.id">
              <td>
                <strong>{{
                  j.kind === "system.check" ? "系统检查" : j.kind
                }}</strong
                ><small>{{ queueLabel(j.queue) }}</small
                ><code>{{ j.id }}</code>
              </td>
              <td>
                <StatusBadge :state="j.state" /><small
                  >{{ j.attempts }} / {{ j.maxAttempts }} 次</small
                >
              </td>
              <td>
                {{ date(j.updatedAt)
                }}<small>创建 {{ date(j.createdAt) }}</small>
              </td>
              <td class="job-error">{{ j.lastError || "—" }}</td>
              <td>
                <el-button
                  :aria-label="`查看任务 ${j.id}`"
                  @click="inspect(j.id)"
                  >详情</el-button
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <el-empty
        v-if="!loading && !error && !jobs.length"
        description="当前筛选暂无任务"
      /><el-pagination
        v-model:current-page="page"
        :page-size="25"
        :total="total"
        layout="prev,pager,next,total"
        @current-change="refreshJobs"
      />
    </section>
    <el-dialog v-model="dialog" title="后台任务详情" width="min(760px,94vw)"
      ><template v-if="detail"
        ><p>
          状态：{{ detail.state }} · 队列：{{ detail.queue }} · 尝试
          {{ detail.attempts }} / {{ detail.maxAttempts }}
        </p>
        <p>
          可领取时间：{{ date(detail.availableAt) }} · 租约截至：{{
            date(detail.leaseUntil)
          }}
        </p>
        <p>Worker：{{ detail.lockedBy || "未被领取" }}</p>
        <p class="field-hint">
          完整持久化记录（仅查看）。上传、删除、结果未知任务请到对应业务页核对，不提供通用强制重放。
        </p>
        <pre class="job-json">{{ JSON.stringify(detail, null, 2) }}</pre>
      </template></el-dialog
    >
  </div>
</template>
<style scoped>
.ops-metrics {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}
.ops-metrics article {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin: 0;
  padding: 16px;
}
.ops-metrics span,
.ops-metrics small {
  font-size: 12px;
  color: var(--muted);
  overflow-wrap: anywhere;
}
.ops-metrics strong {
  font-size: 20px;
}
.workers {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
  margin: 16px 0;
}
.workers article {
  display: grid;
  gap: 8px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: 8px;
  min-width: 0;
  font-size: 12px;
}
.worker-heading {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}
.worker-heading time {
  margin-left: auto;
}
.worker-heading strong {
  font-size: 13px;
}

.workers code {
  overflow-wrap: anywhere;
}
.workers time {
  color: var(--muted);
}
.queue-chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.queue-chips button {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--border);
  background: var(--panel-bg);
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  min-height: 44px;
}
.danger,
.schedule-error {
  color: var(--el-color-danger);
}
.ops-table {
  width: 100%;
  overflow: auto;
}
.ops-table table {
  border-collapse: collapse;
  width: 100%;
  font-size: 13px;
  text-align: left;
}
.ops-table th {
  background: var(--el-fill-color-light);
  font-size: 12px;
  color: var(--muted);
}
.ops-table td,
.ops-table th {
  padding: 12px;
  vertical-align: top;
  border-bottom: 1px solid var(--border);
  min-width: 115px;
}
.ops-table td:first-child {
  min-width: 180px;
  max-width: 360px;
  overflow-wrap: anywhere;
}
.ops-table small {
  display: block;
  font-size: 11px;
  margin-top: 6px;
  color: var(--muted);
  overflow-wrap: anywhere;
}
.ops-table code {
  font-size: 11px;
  overflow-wrap: anywhere;
}
.ops-table .schedule-error {
  color: var(--el-color-danger);
}
.job-filters {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr 2fr auto;
  gap: 12px;
  align-items: center;
}
.job-error {
  max-width: 300px;
  overflow-wrap: anywhere;
}
.el-pagination {
  margin-top: 16px;
  flex-wrap: wrap;
  max-width: 100%;
}
.ops-links {
  display: flex;
  gap: 12px 20px;
  flex-wrap: wrap;
  align-items: center;
}
.ops-links-note {
  margin: 16px 0 0;
  color: var(--muted);
  font-size: 13px;
}
.ops-links a {
  font-size: 13px;
  color: var(--el-color-primary);
  min-height: 44px;
  display: flex;
  align-items: center;
}
.job-json {
  max-height: 55vh;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
@media (max-width: 1100px) {
  .ops-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .job-filters {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 760px) {
  .workers {
    grid-template-columns: minmax(0, 1fr);
  }
  .worker-heading time {
    margin-left: 0;
  }
  .ops-metrics {
    gap: 8px;
  }
  .ops-metrics article {
    padding: 12px;
  }
  .ops-metrics strong {
    font-size: 17px;
  }
  .ops-table .el-button {
    min-height: 44px;
  }
}
</style>
