<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
import StatusBadge from "../components/StatusBadge.vue";
import MetricTrend from "../components/MetricTrend.vue";
interface Policy {
  backupEnabled: boolean;
  backupCron: string;
  keepDays: number;
  keepCount: number;
  sampleDays: number;
  alertsEnabled: boolean;
  memoryPercent: number;
  diskFreeGB: number;
  loadPerCPU: number;
  failedJobs: number;
  consecutive: number;
  cooldownMinutes: number;
}
interface Backup {
  id: string;
  state: string;
  origin: string;
  pinned: boolean;
  bytes: number;
  digest: string;
  manifest: Record<string, unknown>;
  message: string;
  createdAt: string;
  completedAt: string | null;
}
interface Drill {
  id: string;
  backupId: string;
  state: string;
  message: string;
  report: unknown;
  createdAt: string;
}
interface Release {
  revision: string;
  image: string;
  schema: number;
  digest: string;
  verified: boolean;
  current: boolean;
}
interface Ticket {
  id: string;
  kind: string;
  state: string;
  payload: { backupId?: string; targetRevision?: string; schema: number };
  digest: string;
  expiresAt: string;
  message: string;
}
interface Snapshot {
  policy: { value: Policy; revision: number; nextBackupAt: string };
  backups: Backup[];
  drills: Drill[];
  releases: Release[];
  requests: Ticket[];
  alerts: {
    code: string;
    state: string;
    hits: number;
    value: number | null;
    acknowledged_at: string | null;
    message: string;
    last_seen_at: string;
  }[];
  mode: { paused: boolean; reason: string; revision: number };
  toolsReady: boolean;
  totalBytes: number;
  revision: string;
  version: string;
  schema: number;
}
const data = ref<Snapshot>(),
  policy = ref<Policy>(),
  policyRevision = ref(0),
  busy = ref(false),
  working = ref(false),
  error = ref(""),
  hours = ref(24),
  trends = ref<Record<string, unknown>[]>([]),
  tab = ref("backups"),
  preview = ref<{ request: Ticket; confirmText: string; command: string }>(),
  dialog = ref(false),
  confirmText = ref(""),
  detail = ref<unknown>(),
  detailOpen = ref(false),
  restoreID = ref(""),
  rollbackID = ref("");
const trendRows = computed(() =>
  trends.value.map((row) => ({
    ...row,
    processMiB:
      typeof row.processBytes === "number" ? row.processBytes / 1048576 : null,
    successRate:
      typeof row.succeededJobs10m === "number" &&
      typeof row.failedJobs10m === "number" &&
      row.succeededJobs10m + row.failedJobs10m > 0
        ? (row.succeededJobs10m / (row.succeededJobs10m + row.failedJobs10m)) *
          100
        : null,
  })),
);
const gapMS = computed(
  () => Math.max(1, Math.ceil((hours.value * 60) / 500)) * 60000,
);
let poll: ReturnType<typeof setInterval> | undefined,
  epoch = 0;
const date = (v: string | null | undefined) =>
  v ? new Date(v).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai" }) : "—";
const bytes = (n: number) =>
  n > 1073741824
    ? `${(n / 1073741824).toFixed(2)} GiB`
    : `${(n / 1048576).toFixed(2)} MiB`;
const labels: Record<string, string> = {
  ready: "备份就绪",
  verified: "演练通过",
  pending: "排队中",
  running: "执行中",
  failed: "失败",
  interrupted: "已中断",
  deleted: "已删除",
  preview: "待审批",
  approved: "待宿主执行",
  executing: "宿主执行中",
  completed: "已完成",
  cancelled: "已取消",
  expired: "已过期",
  active: "告警中",
  normal: "正常",
  unknown: "指标未知",
  disabled: "已关闭",
};
async function load() {
  const serial = ++epoch;
  busy.value = true;
  try {
    const out = await request<Snapshot>("/ops");
    if (serial !== epoch) return;
    data.value = out;
    if (!policy.value) {
      policy.value = structuredClone(out.policy.value);
      policyRevision.value = out.policy.revision;
    }
    error.value = "";
  } catch (e) {
    error.value = String(e);
  } finally {
    if (serial === epoch) busy.value = false;
  }
}
async function trend() {
  try {
    trends.value = await request<Record<string, unknown>[]>(
      `/ops/trends?hours=${hours.value}`,
    );
  } catch (e) {
    error.value = String(e);
  }
}
async function reloadPolicy() {
  await load();
  if (data.value) {
    policy.value = structuredClone(data.value.policy.value);
    policyRevision.value = data.value.policy.revision;
  }
}
async function save() {
  if (!policy.value) return;
  await action(async () => {
    const out = await request<Snapshot["policy"]>("/ops/policy", {
      method: "PUT",
      body: JSON.stringify({
        value: policy.value,
        revision: policyRevision.value,
      }),
    });
    policy.value = structuredClone(out.value);
    policyRevision.value = out.revision;
    ElMessage.success("备份、采样保留和告警策略已保存");
  });
}
async function action(fn: () => Promise<unknown>) {
  working.value = true;
  error.value = "";
  try {
    await fn();
    await load();
  } catch (e) {
    if (e !== "cancel" && e !== "close") error.value = String(e);
  } finally {
    working.value = false;
  }
}
async function confirm(
  title: string,
  message: string,
  fn: () => Promise<unknown>,
) {
  await action(async () => {
    await ElMessageBox.confirm(message, title, {
      type: "warning",
      confirmButtonText: "确认继续",
      cancelButtonText: "取消",
    });
    await fn();
  });
}
async function backup() {
  await confirm(
    "创建系统备份",
    "包含数据库和托管STRM/NFO/字幕。备份不含主密钥、115原媒体或Emby/CD2/qB外部卷。",
    () => post("/ops/backups", { confirm: true }),
  );
}
async function drill(b: Backup) {
  await confirm(
    "恢复演练",
    "仅新建隔离克隆库验证，不恢复到正式数据库，不启动业务Worker。结束删除本次创建的克隆库。",
    () => post(`/ops/backups/${b.id}/drill`, { confirm: true }),
  );
}
async function remove(b: Backup) {
  await confirm(
    "删除本程序备份",
    "仅删除选中备份包，不删除媒体。固定、最近两份成功备份及正在使用的备份不可删除。",
    () => post(`/ops/backups/${b.id}/delete`, { confirm: true }),
  );
}
async function showPreview(kind: string, id: string) {
  await action(async () => {
    preview.value = await post("/ops/requests/preview", { kind, id });
    confirmText.value = "";
    dialog.value = true;
  });
}
async function approve() {
  if (!preview.value) return;
  await action(async () => {
    await post(`/ops/requests/${preview.value!.request.id}/approve`, {
      confirm: true,
      digest: preview.value!.request.digest,
      text: confirmText.value,
    });
    dialog.value = false;
    ElMessage.success("已审批；尚未执行恢复或回滚，请在宿主机运行受控命令");
  });
}
async function resume() {
  if (!data.value) return;
  await action(async () => {
    const out = await ElMessageBox.prompt(
      "核对文件、授权和恢复记录后输入：解除维护并恢复业务。已暂停的旧任务不会通用重放。",
      "解除维护",
      {
        inputValidator: (v) => v === "解除维护并恢复业务" || "确认短语不匹配",
        confirmButtonText: "明确解除",
        cancelButtonText: "取消",
      },
    );
    await post("/ops/resume", {
      confirm: true,
      text: out.value,
      revision: data.value!.mode.revision,
    });
  });
}
function inspect(value: unknown) {
  detail.value = value;
  detailOpen.value = true;
}
onMounted(() => {
  void load();
  void trend();
  poll = setInterval(() => {
    if (
      !document.hidden &&
      !working.value &&
      !dialog.value &&
      !detailOpen.value
    ) {
      void load();
      if (tab.value === "trends") void trend();
    }
  }, 15000);
});
onUnmounted(() => {
  epoch++;
  if (poll) clearInterval(poll);
});
</script>
<template>
 <div class="operations-stack">
  <section class="page-heading">
    <div>
      <span class="page-kicker">SYSTEM / 维护中心</span>
      <h1>备份、恢复与运维</h1>
      <p>
        备份与演练在主程序内完成。正式恢复与回滚采用短时审批及宿主机受控执行，不开放Docker
        Socket。
      </p>
    </div>
    <div class="heading-actions">
      <el-button
        :loading="busy"
        @click="
          load();
          trend();
        "
        >刷新</el-button
      ><el-button type="primary" :loading="working" @click="backup"
        >立即备份</el-button
      >
    </div>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    class="section-gap"
  />
  <el-alert
    v-if="data && !data.toolsReady"
    title="PostgreSQL客户端工具不可用，不能执行备份/演练。部署镜像内置所需工具；不要把未执行标记为成功。"
    type="warning"
    :closable="false"
    class="section-gap"
  />
  <section v-if="data?.mode.paused" class="panel maintenance-warning">
    <h2>业务当前暂停</h2>
    <p>{{ data.mode.reason }}</p>
    <el-button type="warning" @click="resume">核对后解除维护</el-button>
  </section>
  <nav class="maintenance-tabs" aria-label="维护功能">
    <button
      v-for="(name, key) in {
        backups: '备份管理',
        recovery: '恢复与回滚',
        alerts: '运维告警',
        trends: '运行趋势',
        policy: '维护策略',
      }"
      :key="key"
      :aria-pressed="tab === key"
      @click="tab = key"
    >
      {{ name }}
    </button>
  </nav>
  <template v-if="data">
    <section v-if="tab === 'backups'" class="panel">
      <div class="panel-title">
        <h2>系统备份</h2>
        <span class="muted"
          >总计 {{ bytes(data.totalBytes) }} ·
          {{
            data.policy.value.backupEnabled
              ? "定时 " + data.policy.value.backupCron
              : "定时备份已关闭"
          }}</span
        >
      </div>
      <p class="field-hint">
        成功备份至少保留最近两份，固定及维护单引用的备份不参与自动清理。最新200条。
      </p>
      <div class="maintenance-table">
        <table>
          <thead>
            <tr>
              <th>备份 / 创建时间</th>
              <th>状态 / 大小</th>
              <th>结果</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in data.backups" :key="b.id">
              <td>
                <code>{{ b.id }}</code
                ><small>{{ date(b.createdAt) }} · {{ b.origin }}</small
                ><el-tag v-if="b.pinned" size="small">已固定</el-tag>
              </td>
              <td>
                <StatusBadge
                  :state="b.state"
                  :label="labels[b.state] || b.state"
                /><small>{{ bytes(b.bytes) }}</small>
              </td>
              <td>
                {{ b.message || "等待执行"
                }}<small v-if="b.digest">SHA256 {{ b.digest }}</small>
              </td>
              <td>
                <div class="maintenance-actions">
                  <el-button @click="inspect(b)">详情</el-button
                  ><el-button v-if="b.state === 'ready'" @click="drill(b)"
                    >恢复演练</el-button
                  ><a
                    v-if="b.state === 'ready'"
                    :href="`/api/v1/ops/backups/${b.id}/download`"
                    class="inline-link"
                    >下载备份</a
                  ><el-button
                    v-if="b.state === 'ready'"
                    @click="
                      action(() =>
                        post(`/ops/backups/${b.id}/pin`, {
                          pinned: !b.pinned,
                          confirm: true,
                        }),
                      )
                    "
                    >{{ b.pinned ? "取消固定" : "固定" }}</el-button
                  ><el-button
                    v-if="['ready', 'failed', 'interrupted'].includes(b.state)"
                    type="danger"
                    plain
                    @click="remove(b)"
                    >删除</el-button
                  >
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <el-empty v-if="!data.backups.length" description="尚无系统备份" />
      <h2 class="section-gap">恢复演练记录</h2>
      <div v-for="d in data.drills" :key="d.id" class="maintenance-record">
        <div>
          <strong>{{ labels[d.state] || d.state }}</strong
          ><code>{{ d.backupId }}</code
          ><small>{{ date(d.createdAt) }}</small>
          <p>{{ d.message }}</p>
        </div>
        <div class="maintenance-actions">
          <el-button @click="inspect(d)">演练详情</el-button
          ><el-button
            v-if="['failed', 'interrupted'].includes(d.state)"
            @click="
              confirm(
                '清理演练临时库',
                '只清理有本次演练创建证明的临时库；正在使用或来源不明的数据库不会删除。',
                () => post(`/ops/drills/${d.id}/cleanup`, { confirm: true }),
              )
            "
            >清理临时库</el-button
          >
        </div>
      </div>
      <p v-if="!data.drills.length" class="muted">
        尚无恢复演练。备份就绪后可验证克隆库恢复与密文可读性。
      </p>
    </section>
    <section v-if="tab === 'recovery'" class="panel">
      <h2>受控恢复与版本回滚</h2>
      <p>
        先预览和审批，再在服务器运行维护单指定命令。批准有效期10分钟。不会在点击审批时直接修改数据库或重建容器。
      </p>
      <el-alert
        title="正式恢复要求24小时内演练通过、相同schema及账号/115授权/媒体归属一致；任何保护数据变化均阻止恢复。回滚只允许已核验的同schema Docker Hub AMD64版本。"
        type="warning"
        :closable="false"
      />
      <div class="recovery-grid">
        <div>
          <h3>恢复系统备份</h3>
          <el-select
            v-model="restoreID"
            aria-label="选择待恢复备份"
            placeholder="选择成功备份"
            ><el-option
              v-for="b in data.backups.filter((x) => x.state === 'ready')"
              :key="b.id"
              :label="`${date(b.createdAt)} · ${b.id}`"
              :value="b.id" /></el-select
          ><el-button
            :disabled="!restoreID"
            @click="showPreview('restore', restoreID)"
            >预览恢复</el-button
          >
        </div>
        <div>
          <h3>回滚程序版本</h3>
          <el-select
            v-model="rollbackID"
            aria-label="选择回滚版本"
            placeholder="选择已核验版本"
            ><el-option
              v-for="r in data.releases.filter((x) => !x.current && x.verified)"
              :key="r.revision"
              :label="`${r.revision} · schema ${r.schema}`"
              :value="r.revision" /></el-select
          ><el-button
            :disabled="!rollbackID"
            @click="showPreview('rollback', rollbackID)"
            >预览回滚</el-button
          >
        </div>
      </div>
      <p class="field-hint">
        在服务器登记已有镜像：python3 scripts/ops-owner.py --root
        /root/docker/mediatool register dingding229/mediatool:sha-完整SHA
      </p>
      <div
        v-for="r in data.releases"
        :key="r.revision"
        class="maintenance-record"
      >
        <div>
          <strong
            >{{ r.current ? "当前版本" : "历史版本" }} · schema
            {{ r.schema }}</strong
          ><code>{{ r.image }}</code
          ><small
            >{{ r.verified ? "宿主核验通过" : "待宿主核验" }} ·
            {{ r.digest || "未登记镜像摘要" }}</small
          >
        </div>
      </div>
      <h3>维护单</h3>
      <div v-for="r in data.requests" :key="r.id" class="maintenance-record">
        <div>
          <strong
            >{{ r.kind === "restore" ? "备份恢复" : "镜像回滚" }} ·
            {{ labels[r.state] || r.state }}</strong
          ><code>{{ r.id }}</code
          ><small>有效至 {{ date(r.expiresAt) }}</small>
          <p>{{ r.message }}</p>
          <code v-if="r.state === 'approved'"
            >python3 scripts/ops-owner.py --root /root/docker/mediatool execute
            {{ r.id }}</code
          >
        </div>
        <div class="maintenance-actions">
          <el-button @click="inspect(r)">详情</el-button
          ><el-button
            v-if="['preview', 'approved'].includes(r.state)"
            @click="
              action(() =>
                post(`/ops/requests/${r.id}/cancel`, { confirm: true }),
              )
            "
            >取消</el-button
          >
        </div>
      </div>
      <el-empty v-if="!data.requests.length" description="暂无维护单" />
    </section>
    <section v-if="tab === 'alerts'" class="panel">
      <div class="panel-title">
        <h2>阈值告警</h2>
        <RouterLink class="inline-link" to="/notifications"
          >通知投递记录</RouterLink
        >
      </div>
      <p class="field-hint">
        连续命中才触发，冷却时间内去重；确认后停止重复提醒，恢复时单独发事件。未知指标不当作正常。
      </p>
      <div v-for="a in data.alerts" :key="a.code" class="maintenance-record">
        <div>
          <strong>{{ a.code }} · {{ labels[a.state] || a.state }}</strong>
          <p>{{ a.message }}</p>
          <small
            >当前 {{ a.value ?? "未知" }} · 连续 {{ a.hits }} 次 ·
            {{ date(a.last_seen_at) }}</small
          >
        </div>
        <el-button
          v-if="a.state === 'active' && !a.acknowledged_at"
          @click="
            action(() => post(`/ops/alerts/${a.code}/ack`, { confirm: true }))
          "
          >确认告警</el-button
        ><el-tag v-if="a.acknowledged_at" type="info">已确认</el-tag>
      </div>
      <el-empty v-if="!data.alerts.length" description="等待首个分钟采样" />
    </section>
    <section v-if="tab === 'trends'" class="panel">
      <div class="panel-title">
        <h2>运行趋势</h2>
        <el-select v-model="hours" aria-label="趋势时间范围" @change="trend"
          ><el-option label="最近1小时" :value="1" /><el-option
            label="最近24小时"
            :value="24" /><el-option label="最近7天" :value="168" /><el-option
            label="最近30天"
            :value="720"
        /></el-select>
      </div>
      <p class="field-hint">
        每分钟采样，自动分桶最多500点。主机/proc与应用进程范围不同；空值和断点保留，不编造历史数据。失败/成功数为滚动十分钟窗口，不能相加当总量。
      </p>
      <div class="trend-grid">
        <MetricTrend
          title="内存使用率"
          unit="%"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="memoryPercent"
        /><MetricTrend
          title="剩余空间"
          unit="GiB"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="diskFreeGB"
        /><MetricTrend
          title="系统负载"
          unit="1min"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="load1"
        /><MetricTrend
          title="每CPU负载"
          unit=""
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="loadPerCPU"
        /><MetricTrend
          title="任务执行耗时"
          unit="秒"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="jobSeconds"
        /><MetricTrend
          title="待处理任务"
          unit="条"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="pendingJobs"
        />
        <MetricTrend
          title="应用堆内存"
          unit="MiB"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="processMiB"
        /><MetricTrend
          title="十分钟失败任务"
          unit="条"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="failedJobs10m"
        /><MetricTrend
          title="十分钟任务成功率"
          unit="%"
          :rows="trendRows"
          :gap-ms="gapMS"
          metric="successRate"
        />
      </div>
      <details class="section-gap">
        <summary>查看格式化采样数据</summary>
        <pre class="maintenance-json">{{
          JSON.stringify(trends, null, 2)
        }}</pre>
      </details>
    </section>
    <section v-if="tab === 'policy' && policy" class="panel">
      <h2>备份、采样与告警策略</h2>
      <el-form label-position="top" class="policy-form" @submit.prevent="save"
        ><el-form-item label="启用定时备份"
          ><el-switch
            v-model="policy.backupEnabled"
            aria-label="启用定时备份" /></el-form-item
        ><el-form-item label="备份Cron（Asia/Shanghai）"
          ><el-input
            v-model="policy.backupCron"
            aria-label="备份Cron" /></el-form-item
        ><el-form-item label="备份保留天数"
          ><el-input-number
            v-model="policy.keepDays"
            :min="1"
            :max="365" /></el-form-item
        ><el-form-item label="备份最大份数（至少保留最近两份）"
          ><el-input-number
            v-model="policy.keepCount"
            :min="2"
            :max="100" /></el-form-item
        ><el-form-item label="采样保留天数"
          ><el-input-number
            v-model="policy.sampleDays"
            :min="1"
            :max="90" /></el-form-item
        ><el-form-item label="启用阈值告警"
          ><el-switch
            v-model="policy.alertsEnabled"
            aria-label="启用阈值告警" /></el-form-item
        ><el-form-item label="内存使用率阈值 %"
          ><el-input-number
            v-model="policy.memoryPercent"
            :min="1"
            :max="100" /></el-form-item
        ><el-form-item label="磁盘剩余空间阈值 GiB"
          ><el-input-number
            v-model="policy.diskFreeGB"
            :min="0"
            :max="100000" /></el-form-item
        ><el-form-item label="每CPU负载阈值"
          ><el-input-number
            v-model="policy.loadPerCPU"
            :min="0.1"
            :max="100"
            :step="0.1" /></el-form-item
        ><el-form-item label="十分钟失败任务阈值"
          ><el-input-number
            v-model="policy.failedJobs"
            :min="1"
            :max="100000" /></el-form-item
        ><el-form-item label="连续命中次数"
          ><el-input-number
            v-model="policy.consecutive"
            :min="1"
            :max="60" /></el-form-item
        ><el-form-item label="重复提醒冷却（分钟）"
          ><el-input-number
            v-model="policy.cooldownMinutes"
            :min="5"
            :max="10080"
        /></el-form-item>
        <div class="policy-save">
          <p class="field-hint">
            当前修订 {{ policyRevision }} · 下次备份
            {{
              date(data.policy.nextBackupAt)
            }}。关闭备份不删除现有备份；固定备份不参与保留清理。
          </p>
          <el-button type="primary" native-type="submit" :loading="working"
            >保存维护策略</el-button
          ><el-button :disabled="working" @click="reloadPolicy"
            >重新读取策略</el-button
          >
        </div></el-form
      >
    </section>
  </template>
  <el-dialog v-model="dialog" title="确认维护审批" width="min(720px,94vw)"
    ><template v-if="preview"
      ><p>
        仅审批，不在浏览器中执行正式恢复/回滚。宿主执行前会创建新备份，并重新核验保护数据。
      </p>
      <p>有效至 {{ date(preview.request.expiresAt) }}</p>
      <el-input
        :model-value="preview.command"
        readonly
        aria-label="宿主受控执行命令"
      /><label class="confirm-label"
        >输入确认短语：{{ preview.confirmText
        }}<el-input v-model="confirmText" aria-label="维护确认短语"
      /></label>
      <pre class="maintenance-json">{{
        JSON.stringify(preview.request, null, 2)
      }}</pre></template
    ><template #footer
      ><el-button @click="dialog = false">取消</el-button
      ><el-button
        type="primary"
        :disabled="confirmText !== preview?.confirmText"
        :loading="working"
        @click="approve"
        >审批维护单（不立即执行）</el-button
      ></template
    ></el-dialog
  >
  <el-dialog v-model="detailOpen" title="维护详情" width="min(900px,94vw)">
    <pre class="maintenance-json">{{ JSON.stringify(detail, null, 2) }}</pre>
  </el-dialog>
 </div>
</template>
<style scoped>
.maintenance-tabs {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin: 16px 0;
}
.maintenance-tabs button {
  background: var(--panel-bg);
  border: 1px solid var(--border);
  padding: 10px 16px;
  min-height: 44px;
  border-radius: 8px;
  color: var(--text);
  cursor: pointer;
}
.maintenance-tabs button[aria-pressed="true"] {
  border-color: var(--el-color-primary);
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
}
.maintenance-table {
  overflow: auto;
  max-width: 100%;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
  text-align: left;
}
th,
td {
  padding: 12px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
  min-width: 130px;
  max-width: 400px;
  overflow-wrap: anywhere;
}
th {
  background: var(--el-fill-color-light);
  color: var(--muted);
  font-size: 12px;
}
small {
  display: block;
  color: var(--muted);
  font-size: 12px;
  margin: 6px 0;
}
code {
  display: block;
  font-size: 12px;
  overflow-wrap: anywhere;
}
.maintenance-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  min-width: 180px;
}
.maintenance-actions .el-button {
  margin: 0;
  min-height: 36px;
}
.maintenance-record {
  display: flex;
  gap: 16px;
  align-items: center;
  justify-content: space-between;
  padding: 16px 0;
  border-bottom: 1px solid var(--border);
  min-width: 0;
}
.maintenance-record > div {
  min-width: 0;
}
.maintenance-record p {
  font-size: 13px;
  overflow-wrap: anywhere;
}
.recovery-grid,
.trend-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin: 20px 0;
}
.recovery-grid .el-select {
  width: 100%;
  margin-bottom: 12px;
}
.policy-form {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.policy-form .el-input-number {
  max-width: 100%;
}
.policy-save {
  grid-column: 1/-1;
}
.maintenance-json {
  max-height: 50vh;
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 12px;
}
.confirm-label {
  display: block;
  margin: 16px 0;
}
.maintenance-warning {
  border-color: var(--el-color-warning);
}
.panel-title .el-select {
  width: 180px;
}
.field-hint {
  overflow-wrap: anywhere;
}
@media (max-width: 900px) {
  .policy-form {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
@media (max-width: 600px) {
  .trend-grid,
  .recovery-grid,
  .policy-form {
    grid-template-columns: minmax(0, 1fr);
  }
  .maintenance-record {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .maintenance-actions .el-button {
    min-height: 44px;
  }
  .panel-title {
    flex-wrap: wrap;
  }
}
</style>
