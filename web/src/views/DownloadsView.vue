<script setup lang="ts">
import ConnectionPanel from "../components/ConnectionPanel.vue";
import { computed, ref, onMounted, onUnmounted } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
import OutputRecoveryDialog from "../components/OutputRecoveryDialog.vue";
import DownloadSourceDialog from "../components/DownloadSourceDialog.vue";
const sourceRecovery = ref<InstanceType<typeof DownloadSourceDialog> | null>(
  null,
);
const outputRecovery = ref<InstanceType<typeof OutputRecoveryDialog>>();
interface Task {
  sourceRecoveryRequired: boolean;
  sourceControlRevision: number;
  id: string;
  title: string;
  state: string;
  seedingState: string;
  trackers?: {
    url: string;
    status: number;
    message: string;
    peers: number;
    seeds: number;
    leeches: number;
  }[];
  trackerError?: string;
  progress: number | null;
  qbState: string;
  downloadSpeed: number;
  uploadSpeed: number;
  seeds: number;
  peers: number;
  syncedAt?: string;
  syncAttemptAt?: string;
  syncError: string;
  infoHash: string;
  message: string;
  media: { title: string; year: string };
  createdAt: string;
}
const transfers = ref<
  {
    id: string;
    destination: string;
    state: string;
    phase: string;
    bytes: number;
    hashBytes: number;
    total: number;
    message: string;
  }[]
>([]);
const tasks = ref<Task[]>([]),
  loading = ref(false),
  error = ref("");
let timer: ReturnType<typeof setInterval> | undefined;
const transferLabels: Record<string, string> = {
  needs_output_reconciliation: "需核对输出布局",
  output_resume_approved: "已批准恢复入库",
  artifacts_pending: "输出待恢复",
  verified: "云端已校验",
  imported: "文件已入库",
  paused: "上传暂停",
  cancelled: "已取消",
  uploading: "上传中",
  uncertain: "上传结果待核对",
};
const qbLabel = (s: string) =>
  ({
    stalledDL: "下载停滞（等待有效连接）",
    downloading: "正在下载",
    metaDL: "获取元数据",
    checkingDL: "校验中",
    pausedDL: "下载已暂停",
    stoppedDL: "下载已停止",
    error: "下载器错误",
    missingFiles: "源文件缺失",
    queuedDL: "下载排队",
    allocating: "分配空间",
  })[s] ||
  s ||
  "状态未同步";
const rate = (n: number) =>
  n >= 1024 ** 2
    ? `${(n / 1024 ** 2).toFixed(2)} MiB/s`
    : `${(n / 1024).toFixed(1)} KiB/s`;
const labels: Record<string, string> = {
  pending: "等待下载器提交",
  downloading: "下载中",
  local_complete: "本地完成，上传入库中",
  strm_ready: "STRM已生成，等待Emby刷新",
  uploading: "上传中",
  cloud_verified: "云端校验完成",
  imported: "Emby库中已确认",
  refresh_requested: "已请求刷新，等待Emby扫描确认",
  failed: "失败",
};
const currentTasks = computed(
  () =>
    tasks.value.filter(
      (x) => !["imported", "failed", "cancelled"].includes(x.state),
    ).length,
);
const currentTransfers = computed(
  () =>
    transfers.value.filter((x) => !["imported", "cancelled"].includes(x.state))
      .length,
);
async function refresh(sync = false) {
  if (loading.value) return;
  loading.value = true;
  try {
    if (sync) {
      const result = await post<{ message: string }>("/downloads/sync", {});
      if (result.message) ElMessage.warning(result.message);
    }
    const [downloadList, transferList] = await Promise.all([
      request<Task[]>("/downloads"),
      request<typeof transfers.value>("/transfers"),
    ]);
    tasks.value = downloadList;
    transfers.value = transferList;
    error.value = "";
  } catch (e) {
    error.value = e instanceof Error ? e.message : "下载任务不可用";
  } finally {
    loading.value = false;
  }
}
async function retry(kind: string, id: string) {
  try {
    await post(`/pipeline/${kind}/${id}/retry`, {});
    ElMessage.success("重试已加入持久化队列");
    await refresh();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "不能重试");
  }
}
async function control(id: string, action: string) {
  try {
    let allowRestartIfAbsent = false;
    if (action === "reconcile") {
      try {
        await post(`/transfers/${id}/control`, {
          action: "reconcile",
          allowRestartIfAbsent: false,
        });
        ElMessage.success("远端任务/文件已核对");
        await refresh();
        return;
      } catch (e) {
        const message = e instanceof Error ? e.message : "";
        if (!message.includes("explicitly confirm a restart")) throw e;
        await ElMessageBox.confirm(
          "远端当前未见匹配任务或已完成文件。明确确认后才允许新会话；请先在CloudDrive2界面再次核对。",
          "核对后重建",
          {
            confirmButtonText: "确认允许重建",
            cancelButtonText: "取消",
            type: "warning",
          },
        );
        allowRestartIfAbsent = true;
      }
    } else {
      await ElMessageBox.confirm(
        action === "cancel"
          ? "取消上传不会删除本地做种文件，也不自动删除云端已有数据。"
          : "将控制已持久化的CloudDrive2上传会话。",
        "上传控制",
        {
          confirmButtonText: "确认操作",
          cancelButtonText: "取消",
          type: "warning",
        },
      );
    }
    await post(`/transfers/${id}/control`, { action, allowRestartIfAbsent });
    ElMessage.success("控制请求已接受，请查看后续状态");
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
async function bind(id: string) {
  try {
    const list = await request<{ id: string; size: number; state: string }[]>(
      `/transfers/${id}/sessions`,
    );
    if (!list.length)
      throw new Error("没有匹配目标路径的远端会话，先在CloudDrive2核对");
    const { value } = await ElMessageBox.prompt(
      "匹配会话：" +
        list.map((v) => `${v.id} (${v.size}字节，${v.state})`).join("；") +
        "。输入确认的会话ID，绑定会先暂停该会话。",
      "绑定既有上传会话",
      {
        inputValidator: (v) =>
          list.some((s) => s.id === v) || "请输入列表中的准确ID",
        confirmButtonText: "确认绑定并暂停",
        cancelButtonText: "取消",
      },
    );
    await post(`/transfers/${id}/control`, {
      action: "bind",
      sessionId: value,
      confirmSessionBinding: true,
    });
    ElMessage.success("会话已绑定并暂停，核对后可恢复");
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
onMounted(() => {
  refresh();
  timer = setInterval(refresh, 5000);
});
onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 下载与传输</span>
      <h1>下载与传输</h1>
      <p>下载、做种、上传和入库分开记录，原文件不会因入库而被移动。</p>
    </div>
    <el-button :loading="loading" @click="refresh(true)">刷新进度</el-button>
  </section>
  <section class="download-overview">
    <article class="panel">
      <span>本次加载下载任务</span><strong>{{ tasks.length }}</strong>
    </article>
    <article class="panel">
      <span>仍在流水线内</span><strong>{{ currentTasks }}</strong>
    </article>
    <article class="panel">
      <span>上传与核对任务</span><strong>{{ currentTransfers }}</strong>
    </article>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
  />
  <section class="panel download-section">
    <div class="panel-title">
      <div>
        <span class="section-kicker">01 / 下载与做种</span>
        <h2>下载任务</h2>
        <p class="field-hint">
          下载进度与做种状态独立展示，原文件保留在下载器中。
        </p>
      </div>
      <RouterLink to="/search" class="inline-link">搜索并创建下载</RouterLink>
    </div>
    <el-table
      :data="tasks"
      empty-text="尚无下载任务。先配置站点、TMDB与qBittorrent，再搜索资源。"
      ><el-table-column type="expand"
        ><template #default="scope">
          <pre class="download-evidence">{{
            JSON.stringify(scope.row, null, 2)
          }}</pre>
        </template></el-table-column
      ><el-table-column label="资源 / 媒体" min-width="300"
        ><template #default="scope"
          ><strong>{{ scope.row.title }}</strong>
          <p class="field-hint">
            {{ scope.row.media?.title }} ({{ scope.row.media?.year }})
          </p></template
        ></el-table-column
      ><el-table-column label="进度 / 下载器状态" min-width="320"
        ><template #default="scope"
          ><el-progress
            v-if="scope.row.progress !== null"
            :percentage="Math.round(scope.row.progress * 100)" /><span
            v-if="scope.row.progress === null"
            >尚未同步</span
          ><small
            >{{ labels[scope.row.state] ?? scope.row.state }} ·
            {{ qbLabel(scope.row.qbState) }}</small
          ><small class="download-telemetry"
            >↓ {{scope.row.syncedAt ? rate(scope.row.downloadSpeed) : "未知"}} / ↑
            {{scope.row.syncedAt ? rate(scope.row.uploadSpeed) : "未知"}} · 做种连接
            {{ scope.row.seeds ?? "未知" }} / 下载连接
            {{ scope.row.peers ?? "未知" }}<br />最近同步：{{
              scope.row.syncedAt
                ? new Date(scope.row.syncedAt).toLocaleString("zh-CN")
                : "未同步"
            }}<br /><code>{{ scope.row.infoHash }}</code></small
          ><small
            v-for="tracker in scope.row.trackers?.filter(
              (t: any) => t.status >= 4 && t.message,
            ) ?? []"
            :key="tracker.url"
            class="tracker-error"
            >Tracker：{{ tracker.message }}</small
          ><small v-if="scope.row.trackerError" class="tracker-error"
            >Tracker状态未同步：{{ scope.row.trackerError }}</small
          >
          <el-alert
            v-if="scope.row.syncError"
            :title="'同步失败（保留上次进度）：' + scope.row.syncError"
            type="warning"
            :closable="false" /></template></el-table-column
      ><el-table-column label="做种" width="100"
        ><template #default="scope">{{
          scope.row.seedingState === "seeding" ? "做种中" : "待开始"
        }}</template></el-table-column
      ><el-table-column label="操作" width="100"
        ><template #default="scope"
          ><el-button
            v-if="scope.row.sourceRecoveryRequired"
            @click="sourceRecovery?.open(scope.row.id)"
            >核对来源</el-button
          ><el-button v-else @click="retry('downloads', scope.row.id)"
            >重试</el-button
          ></template
        ></el-table-column
      ><el-table-column label="最近提示" min-width="180"
        ><template #default="scope">{{
          scope.row.message || "—"
        }}</template></el-table-column
      ></el-table
    >
  </section>
  <section class="panel transfer-section">
    <div class="panel-title">
      <div>
        <span class="section-kicker">02 / 上传与入库</span>
        <h2>115上传与入库</h2>
        <p class="field-hint">上传、文件校验和媒体入库按阶段记录。</p>
      </div>
      <RouterLink to="/organize-history?kind=transfer" class="inline-link"
        >查看上传与入库阶段证据</RouterLink
      >
    </div>
    <el-table
      :data="transfers"
      empty-text="下载完成且CloudDrive2已启用后会准备上传任务。"
      ><el-table-column
        prop="destination"
        label="目标文件"
        min-min-width="280" /><el-table-column label="状态" min-width="150"
        ><template #default="scope"
          ><strong>{{
            transferLabels[scope.row.state] ?? scope.row.state
          }}</strong
          ><small class="table-id">{{ scope.row.phase }}</small></template
        ></el-table-column
      ><el-table-column label="处理进度" min-width="150"
        ><template #default="scope"
          ><el-progress
            :percentage="
              scope.row.total
                ? Math.round((scope.row.bytes / scope.row.total) * 100)
                : 0
            " /></template></el-table-column
      ><el-table-column label="操作" min-width="290"
        ><template #default="scope"
          ><el-button
            v-if="
              scope.row.state === 'needs_output_reconciliation' ||
              scope.row.state === 'output_resume_approved'
            "
            size="small"
            @click="outputRecovery?.open(scope.row.id)"
            >核对入库恢复</el-button
          ><template v-else
            ><el-button size="small" @click="retry('transfers', scope.row.id)"
              >重试</el-button
            ><el-button size="small" @click="control(scope.row.id, 'pause')"
              >暂停</el-button
            ><el-button size="small" @click="control(scope.row.id, 'resume')"
              >恢复</el-button
            ><el-button size="small" @click="control(scope.row.id, 'reconcile')"
              >核对</el-button
            ><el-button
              v-if="
                scope.row.state === 'uncertain' ||
                scope.row.state === 'starting'
              "
              size="small"
              @click="bind(scope.row.id)"
              >绑定会话</el-button
            ><el-button
              size="small"
              type="danger"
              plain
              @click="control(scope.row.id, 'cancel')"
              >取消</el-button
            ></template
          ></template
        ></el-table-column
      ><el-table-column prop="message" label="最近提示" min-width="180"
    /></el-table>
  </section>
  <section class="note-panel">
    <strong>进度与完成条件</strong>
    <p>
      上传字节按已确认区间去重统计；哈希计算与传输分开。100%传输不代表云端校验或Emby播放完成。
    </p>
    <p>
      CloudDrive2上传状态、远端文件大小与可用哈希验证后才生成STRM。Emby扫描后确认所有对应STRM可见才标记入库完成；下载原文件继续做种。真实外部服务的最终验收需用你的本地配置完成。
    </p>
  </section>
  <OutputRecoveryDialog
    ref="outputRecovery"
    @completed="refresh"
  /><DownloadSourceDialog ref="sourceRecovery" @completed="refresh" />
  <section class="downloader-section">
    <ConnectionPanel :allowed-kinds="['qbittorrent']" title="下载器配置" />
  </section>
</template>

<style scoped>
.tracker-error {
  display: block;
  color: var(--caution);
  overflow-wrap: anywhere;
  margin-top: 8px;
}
.download-evidence {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 16px;
  max-height: 400px;
  overflow: auto;
}
.download-telemetry {
  display: block;
  color: var(--muted);
  line-height: 1.8;
  margin-top: 8px;
  overflow-wrap: anywhere;
}

.download-section,
.transfer-section,
.note-panel,
.downloader-section {
  margin-bottom: 28px;
}
.downloader-section {
  padding-top: 24px;
  border-top: 1px solid var(--border);
}
.download-overview {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.download-overview span {
  display: block;
  color: var(--muted);
  font-size: 12px;
}
.download-overview strong {
  display: block;
  font-size: 26px;
  margin-top: 8px;
}
@media (max-width: 680px) {
  .download-overview {
    grid-template-columns: 1fr;
  }
  .download-overview article {
    padding: 16px;
  }
}
</style>
