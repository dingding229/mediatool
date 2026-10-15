<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from "vue";
import { request, post } from "../api/client";
import { ElMessage, ElMessageBox } from "element-plus";
import { Plus, DocumentAdd, Link } from "@element-plus/icons-vue";
interface Task {
  id: string;
  title: string;
  sourceKind: string;
  source: string;
  infoHash: string;
  state: string;
  submitAttempted?: boolean;
  progress: number | null;
  qbState: string;
  downloadSpeed: number;
  savePath: string;
  category: string;
  endpoint: string;
  message: string;
  createdAt: string;
  syncedAt?: string;
}
const tasks = ref<Task[]>([]),
  dialog = ref(false),
  kind = ref("file"),
  url = ref(""),
  file = ref<File>(),
  busy = ref(false),
  loading = ref(false),
  error = ref(""),
  listError = ref(""),
  input = ref<HTMLInputElement>();
let timer: ReturnType<typeof setInterval> | undefined;
const labels: Record<string, string> = {
  pending: "等待提交",
  failed: "提交失败，可重试",
  absent_confirmed: "已核对未找到，可确认后重试",
  blocked: "提交受阻，请查看原因",
  submitting: "提交中",
  uncertain: "结果待核对",
  downloading: "下载中",
  completed: "本地完成",
};
async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    tasks.value = await request("/downloads/manual");
    listError.value = "";
  } catch (e) {
    listError.value = String(e);
  } finally {
    loading.value = false;
  }
}
async function sync() {
  loading.value = true;
  try {
    const result = await request<{ message: string }>("/downloads/sync", {
      method: "POST",
      body: "{}",
    });
    if (result.message) ElMessage.warning(result.message);
  } catch (e) {
    listError.value = String(e);
  } finally {
    loading.value = false;
    await load();
  }
}
function choose() {
  file.value = input.value?.files?.[0];
  error.value = "";
}
async function submit() {
  error.value = "";
  if (
    kind.value === "file" &&
    (!file.value ||
      !file.value.name.toLowerCase().endsWith(".torrent") ||
      file.value.size > 10 * 1024 * 1024)
  ) {
    error.value = "请选择不超过10MiB的.torrent文件";
    return;
  }
  if (kind.value === "link" && !url.value.trim()) {
    error.value = "请填写种子链接或磁力链接";
    return;
  }
  busy.value = true;
  try {
    let body: BodyInit;
    if (kind.value === "file") {
      const form = new FormData();
      form.append("torrent", file.value!);
      body = form;
    } else {
      body = JSON.stringify({ url: url.value.trim() });
    }
    await request("/downloads/manual", { method: "POST", body });
    ElMessage.success("手动任务已登记；相同hash不会重复添加");
    dialog.value = false;
    url.value = "";
    file.value = undefined;
    if (input.value) input.value.value = "";
    await load();
  } catch (e) {
    error.value = String(e);
  } finally {
    busy.value = false;
  }
}
const actionBusy = ref("");
async function reconcile(task: Task) {
  actionBusy.value = task.id;
  try {
    const out = await post<{state:string}>(`/downloads/manual/${task.id}/inspect`, {});
    ElMessage.success(out.state === 'absent_confirmed' ? '已核对下载器未找到此种子；未重新提交，可确认风险后重试' : '已核对下载器中的现有任务');
    await load();
  } catch (e) { listError.value = e instanceof Error ? e.message : String(e); }
  finally { actionBusy.value = ""; }
}
async function retry(task: Task) {
  if (!['blocked','failed','absent_confirmed'].includes(task.state) || (task.submitAttempted && task.state !== 'absent_confirmed')) return;
  const uncertain = task.state === 'absent_confirmed';
  try {
    await ElMessageBox.confirm(uncertain ? '下载器已核对未找到此种子，但此前提交可能延迟接收。确认可能的重复风险并重新提交？' : '修复原因后，确认重新提交此手动下载任务？', '重试手动下载', {type:'warning',confirmButtonText:'确认重试',cancelButtonText:'取消'});
    actionBusy.value = task.id;
    await post(`/downloads/manual/${task.id}/retry`, {confirm:true,acceptDuplicateRisk:uncertain});
    ElMessage.success('手动下载任务已重新排队');
    await load();
  } catch (e) { if (e !== 'cancel' && e !== 'close') listError.value = e instanceof Error ? e.message : String(e); }
  finally { actionBusy.value = ""; }
}
onMounted(() => {
  void load();
  timer = setInterval(() => void load(), 8000);
});
onBeforeUnmount(() => clearInterval(timer));
</script>
<template>
  <section class="panel manual-downloads">
    <div class="panel-title">
      <div>
        <h2>手动种子下载</h2>
        <p class="field-hint">
          种子文件、HTTP(S)种子链接或磁力链接，使用已配置的qBittorrent。
        </p>
      </div>
      <div class="manual-actions">
        <el-button @click="sync" :loading="loading">同步手动下载</el-button
        ><el-button
          type="primary"
          @click="
            dialog = true;
            error = '';
          "
          ><Plus />添加种子</el-button
        >
      </div>
    </div>
    <el-alert
      v-if="listError"
      :title="listError"
      type="error"
      :closable="false"
    />
    <p class="field-hint">
      手动任务仅下载，不自动选择TMDB身份、上传115、生成STRM或删除本地文件。需要入库时请在文件管理中确认媒体身份并整理。
    </p>
    <el-empty
      v-if="!tasks.length && !listError && !loading"
      description="暂无手动下载任务"
      :image-size="60"
    />
    <div v-if="tasks.length" class="manual-list">
      <article v-for="task in tasks" :key="task.id" class="manual-task">
        <div class="manual-task-title">
          <strong>{{ task.title }}</strong
          ><el-tag
            :type="
              task.state === 'uncertain'
                ? 'warning'
                : task.state === 'completed'
                  ? 'success'
                  : 'info'
            "
            >{{ labels[task.state] || task.state }}</el-tag
          >
        </div>
        <el-progress
          v-if="task.progress !== null"
          :percentage="Math.round(task.progress * 1000) / 10"
        />
        <p v-else class="field-hint">进度待下载器同步，不以0%代替未知进度。</p>
        <p class="field-hint">
          {{ task.qbState || "状态待同步" }} ·
          {{ (task.downloadSpeed / 1024).toFixed(1) }} KiB/s ·
          {{ new Date(task.createdAt).toLocaleString() }}
        </p>
        <p class="field-hint">{{ task.message || "等待后台处理" }}</p>
        <p class="field-hint">
          最近成功同步：{{
            task.syncedAt
              ? new Date(task.syncedAt).toLocaleString()
              : "尚未同步"
          }}；同步失败时保留上次进度。
        </p>
        <div class="manual-actions">
          <el-button v-if="task.state === 'absent_confirmed' || (['blocked','failed'].includes(task.state) && !task.submitAttempted)" :loading="actionBusy === task.id" :disabled="!!actionBusy" @click="retry(task)">确认重试</el-button>
          <el-button v-if="['uncertain','submitting','absent_confirmed'].includes(task.state) || (['blocked','failed'].includes(task.state) && task.submitAttempted)" :loading="actionBusy === task.id" :disabled="!!actionBusy" @click="reconcile(task)">只读核对下载器</el-button>
        </div>
        <details>
          <summary>来源与下载信息</summary>
          <dl>
            <dt>来源</dt>
            <dd>{{ task.source }}</dd>
            <dt>Info hash</dt>
            <dd>
              <code>{{ task.infoHash }}</code>
            </dd>
            <dt>下载器</dt>
            <dd>{{ task.endpoint }}</dd>
            <dt>保存目录</dt>
            <dd>{{ task.savePath }}</dd>
            <dt>分类</dt>
            <dd>{{ task.category }}</dd>
          </dl>
        </details>
      </article>
    </div>
  </section>
  <el-dialog
    v-model="dialog"
    title="添加种子下载"
    width="min(600px,94vw)"
    :close-on-click-modal="!busy"
    :show-close="!busy"
    ><el-alert v-if="error" :title="error" type="error" :closable="false" />
    <form class="settings-form" @submit.prevent="submit">
      <el-radio-group v-model="kind" aria-label="种子来源"
        ><el-radio-button value="file"><DocumentAdd />种子文件</el-radio-button
        ><el-radio-button value="link"
          ><Link />种子链接</el-radio-button
        ></el-radio-group
      >
      <div v-show="kind === 'file'" class="settings-field">
        <label for="manual-torrent-file">种子文件（.torrent）</label
        ><input
          ref="input"
          id="manual-torrent-file"
          type="file"
          accept=".torrent,application/x-bittorrent"
          @change="choose"
        />
        <p class="field-hint">
          最大10MiB；服务端会解析文件内容并校验info hash。
        </p>
      </div>
      <div v-show="kind === 'link'" class="settings-field">
        <label for="manual-torrent-url">种子链接或磁力链接</label
        ><el-input
          id="manual-torrent-url"
          v-model="url"
          type="textarea"
          :rows="4"
          placeholder="https://example.com/file.torrent 或 magnet:?xt=urn:btih:..."
        />
        <p class="field-hint">
          HTTP(S)链接须返回种子原文，不是详情网页；拒绝本机与内网目标，链接中的认证参数完整保留。
        </p>
      </div>
      <div class="settings-notice">
        <p>
          使用下载器中已保存的目录与分类。相同hash去重，提交结果未知时只核对、不重复添加；不会自动入库或清理。
        </p>
      </div>
      <div class="settings-actions">
        <el-button :disabled="busy" @click="dialog = false">取消</el-button
        ><el-button type="primary" native-type="submit" :loading="busy"
          >确认添加下载</el-button
        >
      </div>
    </form></el-dialog
  >
</template>
<style scoped>
.manual-downloads {
  margin-bottom: 24px;
  min-width: 0;
}
.manual-downloads .panel-title {
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
}
.manual-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.manual-actions .el-button {
  margin: 0;
}
.manual-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-top: 16px;
}
.manual-task {
  padding: 16px;
  border: 1px solid var(--border);
  border-radius: 8px;
  min-width: 0;
}
.manual-task-title {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  justify-content: space-between;
  margin-bottom: 12px;
}
.manual-task strong,
.manual-task dd {
  overflow-wrap: anywhere;
}
.manual-task .el-tag {
  flex-shrink: 0;
}
.manual-task summary {
  cursor: pointer;
  font-size: 13px;
  min-height: 32px;
  padding: 6px 0;
  color: var(--el-color-primary);
}
.manual-task dl {
  font-size: 13px;
  margin: 8px 0;
  display: grid;
  grid-template-columns: 70px minmax(0, 1fr);
  gap: 8px;
}
.manual-task dt {
  color: var(--muted);
}
.manual-task dd {
  margin: 0;
}
.manual-actions svg,
.el-radio-button svg {
  width: 15px;
  height: 15px;
  vertical-align: middle;
  margin-right: 4px;
}
#manual-torrent-file {
  max-width: 100%;
  min-height: 44px;
  font-size: 14px;
}
@media (max-width: 760px) {
  .manual-list {
    grid-template-columns: 1fr;
  }
  .manual-actions .el-button {
    min-height: 44px;
  }
}
</style>
