<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage, ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
import { resourceSources } from "../utils/resource-source";
import StatusBadge from "../components/StatusBadge.vue";
interface Media {
  id: number;
  mediaType: string;
  title: string;
  year: string;
}
interface Rule {
  resolution?: string;
  source?: string;
  codec?: string;
  accountIds: string[];
  keyword: string;
  regexKeywords?: boolean;
  include: string[];
  exclude: string[];
  minSize: number;
  maxSize: number;
  minSeeders: number;
  freeOnly: boolean;
  rejectHR?: boolean;
  season: number;
  episodes: number[];
  includeHistory: boolean;
}
interface Subscription {
  id: string;
  media: Media;
  title: string;
  enabled: boolean;
  source: string;
  intervalMinutes: number;
  rules: Rule;
  revision: number;
  lastState: string;
  lastMessage: string;
  nextRunAt: string;
}
interface Account {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
  secretFields: string[];
}
interface Run {
  id: string;
  state: string;
  preview: boolean;
  decisions: {
    id: string;
    title: string;
    action: string;
    reasons: string[];
    episodes: number[];
    checks?: {
      key: string;
      label: string;
      state: string;
      actual: string;
      expected: string;
      message: string;
    }[];
  }[];
}
const route = useRoute(),
  router = useRouter();
const tmdbCache = ref<{ cached: boolean; seconds: number } | null>(null);
const list = ref<Subscription[]>([]),
  accounts = ref<Account[]>([]),
  loading = ref(false),
  error = ref(""),
  editing = ref(false),
  saving = ref(false),
  finding = ref(false),
  candidates = ref<Media[]>([]),
  run = ref<Run | null>(null),
  runDialog = ref(false);
const view=ref('current'),historyOpen=ref(false),historyRows=ref<Array<{id:string;state:string;preview:boolean;createdAt:string}>>([]);
async function historyFor(id:string){try{historyRows.value=await request(`/subscriptions/${id}/history`);historyOpen.value=true}catch(e){ElMessage.error(String(e))}}
async function historyResult(id:string){try{run.value=await request(`/subscription-runs/${id}`);runDialog.value=true}catch(e){ElMessage.error(String(e))}}

const form = reactive({
  id: "",
  mediaType: "movie",
  tmdbId: 0,
  title: "",
  query: "",
  enabled: false,
  source: "search",
  accountIds: [] as string[],
  regexKeywords: true,
  keyword: "",
  include: "",
  exclude: "",
  resolution: "",
  releaseSource: "",
  codec: "",
  freeOnly: true,
  rejectHR: true,
  season: 1,
  episodes: "",
  includeHistory: false,
  revision: 0,
});
let timer: ReturnType<typeof setInterval> | undefined;
const actions: Record<string, string> = {
  skipped: "未匹配 / 已覆盖",
  needs_confirmation: "需人工确认",
  would_download: "预演：可下载",
  download_queued: "已创建下载",
  download_failed: "下载创建失败",
};
async function refresh() {
  loading.value = true;
  try {
    list.value = await request<Subscription[]>(`/subscriptions?view=${view.value}`);
    accounts.value = (await request<Account[]>("/integrations")).filter(
      (a) => a.kind === "ptsite" && a.enabled,
    );
    error.value = "";
  } catch (e) {
    error.value = e instanceof Error ? e.message : "订阅记录不可用";
  } finally {
    loading.value = false;
  }
}
function create() {
  Object.assign(form, {
    id: "",
    mediaType: "movie",
    tmdbId: 0,
    title: "",
    query: "",
    enabled: false,
    source: "search",
    accountIds: accounts.value.map((a) => a.id),
    regexKeywords: true,
    keyword: "",
    include: "",
    exclude: "",
    resolution: "",
    releaseSource: "",
    codec: "",
    freeOnly: true,
    rejectHR: true,
    season: 1,
    episodes: "",
    includeHistory: false,
    revision: 0,
  });
  candidates.value = [];
  tmdbCache.value = null;
  editing.value = true;
}
function edit(s: Subscription) {
  Object.assign(form, {
    id: s.id,
    mediaType: s.media.mediaType,
    tmdbId: s.media.id,
    title: s.title,
    query: s.title,
    enabled: s.enabled,
    source: s.source,
    accountIds: [...s.rules.accountIds],
    regexKeywords: s.rules.regexKeywords ?? false,
    keyword: s.rules.keyword,
    include: s.rules.include?.join("\n") ?? "",
    exclude: s.rules.exclude?.join("\n") ?? "",
    resolution: s.rules.resolution ?? "",
    releaseSource: s.rules.source ?? "",
    codec: s.rules.codec ?? "",
    freeOnly: s.rules.freeOnly,
    rejectHR: s.rules.rejectHR ?? true,
    season: s.rules.season,
    episodes: s.rules.episodes?.join(",") ?? "",
    includeHistory: s.rules.includeHistory,
    revision: s.revision,
  });
  candidates.value = [];
  tmdbCache.value = null;
  editing.value = true;
}
async function find() {
  if (!form.query.trim() || finding.value) return;
  tmdbCache.value = null;
  candidates.value = [];
  form.tmdbId = 0;
  finding.value = true;
  try {
    const out = await request<{
      items: Media[];
      cached: boolean;
      cacheSeconds: number;
    }>(
      `/search/media?mediaType=${form.mediaType}&q=${encodeURIComponent(form.query)}`,
    );
    candidates.value = out.items;
    tmdbCache.value = { cached: out.cached, seconds: out.cacheSeconds };
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "媒体查询失败");
  } finally {
    finding.value = false;
  }
}
function select(m: Media | null) {
  if (m) {
    form.tmdbId = m.id;
    form.title = m.title;
    if (!form.keyword) form.keyword = m.title;
  }
}
function body(s: Subscription) {
  return {
    mediaType: s.media.mediaType,
    tmdbId: s.media.id,
    enabled: s.enabled,
    source: s.source,
    rules: s.rules,
    revision: s.revision,
  };
}
async function save() {
  saving.value = true;
  try {
    const episodes = form.episodes.trim()
      ? form.episodes.split(",").map((v) => Number(v.trim()))
      : [];
    if (episodes.some((n) => !Number.isInteger(n) || n < 1))
      throw new Error("季集编号使用逗号分隔的正整数");
    const payload = {
      mediaType: form.mediaType,
      tmdbId: form.tmdbId,
      enabled: form.enabled,
      source: form.source,
      revision: form.revision,
      rules: {
        resolution: form.resolution,
        source: form.releaseSource,
        codec: form.codec,
        accountIds: form.accountIds,
        keyword: form.keyword,
        regexKeywords: form.regexKeywords,
        include: form.include
          .split(form.regexKeywords ? /\r?\n/ : /,|\r?\n/)
          .map((v) => v.trim())
          .filter(Boolean),
        exclude: form.exclude
          .split(form.regexKeywords ? /\r?\n/ : /,|\r?\n/)
          .map((v) => v.trim())
          .filter(Boolean),
        minSize: 0,
        maxSize: 0,
        minSeeders: 0,
        freeOnly: form.freeOnly,
        rejectHR: form.rejectHR,
        season: form.season,
        episodes,
        includeHistory: form.includeHistory,
      },
    };
    if (form.id)
      await request(`/subscriptions/${form.id}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      });
    else await post("/subscriptions", payload);
    editing.value = false;
    ElMessage.success("订阅已保存；停用状态不会自动下载");
    await refresh();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    saving.value = false;
  }
}
async function toggle(s: Subscription) {
  try {
    const input = body(s);
    input.enabled = !s.enabled;
    await request(`/subscriptions/${s.id}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
    await refresh();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "状态修改失败");
  }
}
async function evaluate(s: Subscription, preview: boolean) {
  try {
    const created = await post<{ id: string }>(
      `/subscriptions/${s.id}/evaluate`,
      { preview },
    );
    run.value = await request<Run>(`/subscription-runs/${created.id}`);
    runDialog.value = true;
    ElMessage.success(
      preview
        ? "规则预演已排队，不创建下载"
        : "评估已排队，只有启用订阅才会自动下载",
    );
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "评估创建失败");
  }
}
async function remove(s: Subscription) {
  try {
    await ElMessageBox.confirm(
      "归档后停止后续订阅，历史规则、评估和下载关联保留；不删除已有下载、云盘文件或做种源文件。",
      "归档订阅",
      {
        confirmButtonText: "归档订阅",
        cancelButtonText: "取消",
        type: "warning",
      },
    );
    await request(`/subscriptions/${s.id}`, { method: "DELETE" });
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
async function poll() {
  if (run.value && ["pending", "running"].includes(run.value.state)) {
    try {
      run.value = await request<Run>(`/subscription-runs/${run.value.id}`);
      if (!["pending", "running"].includes(run.value.state)) await refresh();
    } catch {}
  }
}
onMounted(async () => {
  await refresh();
  if (
    route.query.create === "1" &&
    (route.query.mediaType === "movie" || route.query.mediaType === "series") &&
    /^\d+$/.test(String(route.query.tmdbId))
  ) {
    try {
      const out = await request<{ item: Media }>(
        `/metadata/details?mediaType=${route.query.mediaType}&tmdbId=${route.query.tmdbId}`,
      );
      create();
      form.mediaType = out.item.mediaType;
      form.tmdbId = out.item.id;
      form.title = out.item.title;
      form.query = out.item.title;
      form.keyword = out.item.title;
      await router.replace({ path: "/subscriptions" });
    } catch (e) {
      error.value = String(e);
    }
  }

  timer = setInterval(poll, 3000);
});
onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 资源订阅</span>
      <h1>资源订阅</h1>
      <p>先选定媒体身份、预演规则，再启用自动下载。未知信息不默认满足条件。</p>
    </div>
    <div class="heading-actions">
      <el-button :loading="loading" @click="refresh">刷新</el-button
      ><el-button type="primary" @click="create">创建订阅</el-button>
    </div>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
  />
  <section class="panel"><el-radio-group v-model="view" aria-label="订阅记录分类" @change="refresh"><el-radio-button value="current">当前订阅</el-radio-button><el-radio-button value="history">历史订阅</el-radio-button></el-radio-group>
    <el-table
      :data="list"
      empty-text="暂无订阅。先配置TMDB、PT账号及下载器，再创建停用的订阅并预演。"
      ><el-table-column label="媒体" min-width="210"
        ><template #default="scope"
          ><strong>{{ scope.row.title }}</strong
          ><small class="table-id"
            >{{ scope.row.media.mediaType === "movie" ? "电影" : "剧集" }} ·
            TMDB {{ scope.row.media.id }}</small
          ></template
        ></el-table-column
      ><el-table-column label="调度" min-width="120"
        ><template #default="scope"
          >{{ scope.row.enabled ? "已启用" : "已停用"
          }}<small class="table-id"
            >{{scope.row.source}} · {{view==='history'?'历史记录':'系统Cron调度'}}</small
          ></template
        ></el-table-column
      ><el-table-column label="最近评估" min-width="220"
        ><template #default="scope"
          ><StatusBadge :state="scope.row.lastState" />
          <p class="field-hint">
            {{ scope.row.lastMessage || "尚未评估" }}
          </p></template
        ></el-table-column
      ><el-table-column label="评估历史" width="120"><template #default="{row}"><el-button @click="historyFor(row.id)">查看历史</el-button></template></el-table-column><el-table-column v-if="view==='current'" label="操作" min-width="320"
        ><template #default="scope"
          ><el-button size="small" @click="evaluate(scope.row, true)"
            >预演</el-button
          ><el-button size="small" @click="evaluate(scope.row, false)"
            >评估</el-button
          ><el-button size="small" @click="toggle(scope.row)">{{
            scope.row.enabled ? "停用" : "启用"
          }}</el-button
          ><el-button size="small" @click="edit(scope.row)">编辑</el-button
          ><el-button
            size="small"
            type="danger"
            plain
            @click="remove(scope.row)"
            >归档</el-button
          ></template
        ></el-table-column
      ></el-table
    >
  </section>
  <el-dialog
    v-model="editing"
    :title="form.id ? '编辑订阅' : '创建停用订阅'"
    width="min(780px,94vw)"
    :close-on-click-modal="false"
    ><form class="connection-form" @submit.prevent="save">
      <template v-if="!form.id"
        ><label>媒体身份</label>
        <div class="subscription-media-query">
          <el-select
            v-model="form.mediaType"
            aria-label="订阅媒体类型"
            @change="form.tmdbId = 0"
            ><el-option label="电影" value="movie" /><el-option
              label="剧集"
              value="series" /></el-select
          ><el-input
            v-model="form.query"
            @keydown.enter.prevent="find"
            aria-label="订阅TMDB关键词"
            placeholder="准确的媒体标题"
          /><el-button
            :loading="finding"
            :disabled="!form.query.trim()"
            @click="find"
            >查询TMDB</el-button
          >
        </div>
        <p v-if="tmdbCache" class="field-hint" role="status">
          {{ tmdbCache.cached ? "来自TMDB缓存" : "已查询TMDB" }} · 缓存有效期
          {{ tmdbCache.seconds }} 秒（系统设置可修改，0禁用）
        </p>
        <el-table
          :data="candidates"
          height="180"
          highlight-current-row
          @current-change="select"
          ><el-table-column prop="title" label="标题" /><el-table-column
            prop="year"
            label="年份"
            width="90" /><el-table-column
            prop="id"
            label="ID"
            width="100" /></el-table
      ></template>
      <p class="field-hint">
        已选择：{{
          form.tmdbId ? `${form.title} · TMDB ${form.tmdbId}` : "尚未选择"
        }}。编辑不允许变更媒体身份。
      </p>
      <label>PT账号</label
      ><el-checkbox-group v-model="form.accountIds"
        ><el-checkbox v-for="a in accounts" :key="a.id" :value="a.id"
          >{{ a.name
          }}{{
            a.secretFields.includes("rssUrl") ? " · 已配RSS" : ""
          }}</el-checkbox
        ></el-checkbox-group
      >
      <p class="field-hint">订阅刷新间隔在系统设置中统一配置，所有订阅共用。</p>
      <label>发现方式</label
      ><el-select v-model="form.source"
        ><el-option label="周期搜索" value="search" /><el-option
          label="RSS新条目"
          value="rss" /><el-option label="搜索 + RSS" value="both"
      /></el-select>
      <p class="field-hint">
        RSS
        URL在账号连接中加密保存。首次RSS默认只建立基线，不下载历史条目；预演不改变基线。
      </p>
      <label>站点搜索词（普通文字）</label><el-input v-model="form.keyword" />
      <p class="field-hint">
        普通搜索词发给PT站点，下面的正则只在本地匹配资源，不会发送给站点。
      </p>
      <el-checkbox v-model="form.regexKeywords"
        >使用正则匹配包含／排除关键词</el-checkbox
      >
      <div class="rule-grid">
        <div>
          <label>包含关键词（支持正则，一行一个）</label
          ><el-input
            v-model="form.include"
            type="textarea"
            :rows="3"
            aria-label="订阅包含关键词"
          />
        </div>
        <div>
          <label>排除关键词（支持正则，一行一个）</label
          ><el-input
            v-model="form.exclude"
            type="textarea"
            :rows="3"
            aria-label="订阅排除关键词"
          />
        </div>
      </div>
      <div v-if="form.mediaType === 'series'" class="rule-grid">
        <div>
          <label>目标季（0为特别篇）</label
          ><el-input-number v-model="form.season" :min="0" :max="999" />
        </div>
        <div>
          <label>目标集（空为TMDB已播集）</label
          ><el-input v-model="form.episodes" placeholder="1,2,3" />
        </div>
      </div>
      <div class="rule-grid subscription-attributes">
        <div>
          <label for="subscription-resolution">分辨率</label
          ><el-select
            id="subscription-resolution"
            v-model="form.resolution"
            aria-label="订阅分辨率"
            ><el-option label="不限" value="" /><el-option
              v-for="x in [
                '480p',
                '576p',
                '720p',
                '1080i',
                '1080p',
                '1440p',
                '2160p',
                '4320p',
              ]"
              :key="x"
              :label="x"
              :value="x"
          /></el-select>
        </div>
        <div>
          <label for="subscription-release-source">资源来源</label
          ><el-select
            id="subscription-release-source"
            v-model="form.releaseSource"
            aria-label="订阅资源来源"
            ><el-option label="不限" value="" /><el-option
              v-for="option in resourceSources"
              :key="option.value"
              :label="option.label"
              :value="option.value"
          /></el-select>
        </div>
        <div>
          <label for="subscription-codec">编码</label
          ><el-select
            id="subscription-codec"
            v-model="form.codec"
            aria-label="订阅编码"
            ><el-option label="不限" value="" /><el-option
              v-for="x in ['h264', 'h265', 'av1', 'mpeg2']"
              :key="x"
              :label="x"
              :value="x"
          /></el-select>
        </div>
      </div>
      <p class="field-hint">
        明确限制的属性未识别时拒绝匹配；未配置的属性不限。先规则预演，再明确启用，原做种文件不会被替换。
      </p>
      <el-checkbox v-model="form.freeOnly">只下载已确认免费资源</el-checkbox
      ><el-checkbox v-model="form.rejectHR"
        >拒绝下载H&amp;R资源</el-checkbox
      ><p class="field-hint">H&amp;R信息未知时始终拒绝自动下载；关闭此选项只允许已明确标记的H&amp;R资源，请承担相应做种义务。</p><el-checkbox v-model="form.includeHistory"
        >允许首次RSS历史补抓</el-checkbox
      ><el-checkbox v-model="form.enabled">保存后启用自动下载</el-checkbox
      ><el-alert
        title="RSS缺少促销数据时无法满足免费限制。季集使用种子文件清单预检，合集与已在下载中的集不会重复申请。"
        type="warning"
        :closable="false"
      /><el-button
        type="primary"
        native-type="submit"
        :disabled="!form.tmdbId || !form.accountIds.length"
        :loading="saving"
        >保存订阅</el-button
      >
    </form></el-dialog
  >
  <el-dialog v-model="historyOpen" title="历史订阅评估" width="min(780px,94vw)"><el-table :data="historyRows" empty-text="暂无评估历史"><el-table-column label="时间"><template #default="{row}">{{new Date(row.createdAt).toLocaleString('zh-CN')}}</template></el-table-column><el-table-column label="模式"><template #default="{row}">{{row.preview?'预演':'正式评估'}}</template></el-table-column><el-table-column label="状态"><template #default="{row}"><StatusBadge :state="row.state"/></template></el-table-column><el-table-column label="操作"><template #default="{row}"><el-button @click="historyResult(row.id)">查看结果</el-button></template></el-table-column></el-table></el-dialog>
  <el-dialog v-model="runDialog" title="订阅评估与决策" width="min(900px,96vw)"
    ><el-alert
      v-if="run?.state === 'needs_refresh'"
      title="订阅、站点或候选快照已变化。请关闭此窗口并重新预演或发起新的评估；已有下载不会重放。"
      type="warning"
      :closable="false"
      role="alert"
    />
    <div class="panel-title">
      <span>{{ run?.preview ? "仅规则预演" : "正常评估" }}</span
      ><StatusBadge :state="run?.state ?? 'pending'" />
    </div>
    <el-table
      :data="run?.decisions ?? []"
      empty-text="正在评估，或暂时没有候选资源"
      ><el-table-column
        prop="title"
        label="资源"
        min-width="260"
      /><el-table-column label="决策" width="170"
        ><template #default="scope">{{
          actions[scope.row.action] ?? scope.row.action
        }}</template></el-table-column
      ><el-table-column label="原因" min-width="250"
        ><template #default="scope"
          ><div
            v-for="check in scope.row.checks ?? []"
            :key="check.key"
            class="subscription-check"
          >
            <el-tag
              :type="
                check.state === 'rejected'
                  ? 'danger'
                  : check.state === 'matched'
                    ? 'success'
                    : 'info'
              "
              size="small"
              >{{ check.label }} ·
              {{
                check.state === "rejected"
                  ? "拒绝"
                  : check.state === "matched"
                    ? "通过"
                    : "不限"
              }}</el-tag
            ><span>{{ check.message }}</span>
          </div>
          {{ scope.row.reasons.join("；")
          }}<small v-if="scope.row.episodes.length" class="table-id"
            >覆盖集：{{ scope.row.episodes.join(",") }}</small
          ></template
        ></el-table-column
      ></el-table
    ></el-dialog
  >
  <section class="note-panel">
    <strong>去重与缺集</strong>
    <p>
      媒体身份绑定到TMDB。已入库或正在下载的电影/季集计入覆盖；资源变化、种子文件不明确或规则缺信息时不会猜测下载。
    </p>
  </section>
</template>

<style scoped>
.subscription-media-query {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
}
.subscription-media-query > * {
  min-width: 0;
}
.subscription-media-query :deep(.el-button) {
  margin: 0;
  min-height: 44px;
}
.subscription-media-query :deep(.el-select),
.subscription-media-query :deep(.el-input) {
  width: 100%;
}
@media (max-width: 600px) {
  .subscription-media-query {
    grid-template-columns: 1fr auto;
  }
  .subscription-media-query > .el-select {
    grid-column: 1/-1;
  }
}
</style>
