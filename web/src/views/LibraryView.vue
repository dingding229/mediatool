<script setup lang="ts">
import { nextTick, ref, onMounted, onUnmounted, watch } from "vue";
import { ElTabs, ElTabPane, ElMessage, ElMessageBox } from "element-plus";
import { request, post } from "../api/client";
import { useRoute, useRouter } from "vue-router";
const route = useRoute(),
  router = useRouter();
const recognitionEvidence = ref<string[]>([]),
  recognitionBusy = ref(false);
let recognitionEpoch = 0;
import FileBatchWorkbench, {
  type Row as OrganizationRow,
} from "../components/FileBatchWorkbench.vue";
import type { FileRoot, FileEntry } from "../files/types";
const organization = ref<{
    root: FileRoot;
    files: FileEntry[];
    rows: OrganizationRow[];
  }>(),
  organizing = ref(false),
  organizer = ref<InstanceType<typeof FileBatchWorkbench>>(),
  assetSelection = ref<Asset[]>([]);
async function organize(kind: "candidate" | "asset") {
  organizing.value = true;
  error.value = "";
  try {
    const result = await post<{
      root: FileRoot;
      files: FileEntry[];
      rows: OrganizationRow[];
    }>(
      "/library/organize-selection",
      kind === "candidate"
        ? { candidateIds: selection.value.map((x) => x.id) }
        : { assetIds: assetSelection.value.map((x) => x.id) },
    );
    organization.value = result;
    await nextTick();
    await organizer.value?.start();
  } catch (e) {
    error.value = e instanceof Error ? e.message : "整理准备失败";
  } finally {
    organizing.value = false;
  }
}
import ScanWorkbench from "../components/ScanWorkbench.vue";
import ArchiveManager from "../components/ArchiveManager.vue";
interface Candidate {
  assetId?: string;
  id: string;
  name: string;
  path: string;
  state: string;
  revision: number;
  cloudState: string;
}
interface Asset {
  mediaType?: string;
  id: string;
  title: string;
  path: string;
  state: string;
  revoked: boolean;
  fileScoped: boolean;
  revision: number;
  artifactsState: string;
}
interface EpisodeMetadata {
  id: number;
  seriesId: number;
  season: number;
  number: number;
  title: string;
  overview: string;
  airDate?: string;
  runtimeMinutes?: number;
}
interface Identity {
  id: number;
  mediaType: string;
  title: string;
  year: string;
}
interface Scan {
  completedDirectories?: number;
  currentDirectory?: string;
  generation?: number;
  revision?: number;
  updatedAt?: string;
  id: string;
  root: string;
  state: string;
  files: number;
  directories: number;
  message: string;
}
interface Schedule {
  id: string;
  root: string;
  enabled: boolean;
  cron: string;
  revision: number;
  lastError: string;
  nextRunAt: string;
  watchChanges: boolean;
  watchState: string;
  watchMessage: string;
  changeScanAfter?: string;
  lastChangeAt?: string;
}
interface Page<T> {
  items: T[];
  total: number;
}
interface BatchRow {
  fileDigest?: string;
  candidate: Candidate;
  season: number;
  episodes: string;
}
interface Repair {
  id: string;
  path: string;
  kind: string;
  state: string;
  operation?: string;
}
interface MigrationPreview {
  outputProjectionMode?: string;
  wholeFilePlayback?: boolean;
  declaredEpisodes?: number[];
  assetId: string;
  revision: number;
  planDigest: string;
  operations: { path: string; kind: string; operation: string }[];
  cloudFileUnchanged: boolean;
  playbackGrantUnchanged: boolean;
  sharedNfoRetained: boolean;
}
interface MigrationRequest {
  mediaType: string;
  tmdbId: number;
  season: number;
  episodes: number[];
  revision: number;
  migrateOutputs: boolean;
  planDigest?: string;
}
const repairs = ref<Repair[]>([]);
const workspaceTab = ref(
    typeof route.query.assetId === "string" ? "assets" : "candidates",
  ),
  searchError = ref("");
let searchEpoch = 0;
const migrateOutputs = ref(false),
  migrationDialog = ref(false),
  migrationApplying = ref(false),
  migrationPreview = ref<MigrationPreview | null>(null),
  migrationRequest = ref<MigrationRequest | null>(null);
const candidates = ref<Candidate[]>([]),
  assets = ref<Asset[]>([]),
  scans = ref<Scan[]>([]),
  schedules = ref<Schedule[]>([]);
const assetFilter = ref(
  typeof route.query.assetId === "string" ? route.query.assetId : "",
);
const page = ref(1),
  assetPage = ref(1),
  total = ref(0),
  assetTotal = ref(0),
  filter = ref("needs_confirmation"),
  selection = ref<Candidate[]>([]);
const loading = ref(false),
  error = ref(""),
  root = ref(""),
  scanning = ref(false),
  dialog = ref(false),
  saving = ref(false),
  lookup = ref(false);
const batch = ref<BatchRow[]>([]),
  editingAsset = ref<Asset | null>(null),
  query = ref(""),
  mediaType = ref("movie"),
  results = ref<Identity[]>([]),
  selected = ref<Identity | null>(null),
  season = ref(1),
  episodes = ref("");
const episodePreview = ref<EpisodeMetadata[]>([]),
  episodePreviewLoading = ref(false),
  episodePreviewError = ref("");
let episodePreviewEpoch = 0;
const scheduleDialog = ref(false),
  scheduleSaving = ref(false),
  scheduleForm = ref({
    id: "",
    root: "",
    enabled: false,
    watchChanges: false,
    cron: "0 * * * *",
    revision: 0,
  });
let timer: ReturnType<typeof setInterval> | undefined;
async function refresh(background = false) {
  if (
    loading.value ||
    dialog.value ||
    scheduleDialog.value ||
    migrationDialog.value
  )
    return;
  if (!background) {selection.value=[];assetSelection.value=[];}
  loading.value = true;
  const failures: string[] = [];
  try {
    await Promise.all([
      request<Page<Candidate>>(
        `/library/candidates?page=${page.value}&pageSize=50&state=${filter.value}`,
      )
        .then((c) => {
          if (selection.value.length || assetSelection.value.length || organizing.value) return;
          candidates.value = c.items;
          total.value = c.total;
          selection.value = [];
        })
        .catch((e) =>
          failures.push(
            `候选文件：${e instanceof Error ? e.message : "不可用"}`,
          ),
        ),
      request<Page<Asset>>(
        `/media/assets?page=${assetPage.value}&pageSize=50${assetFilter.value ? "&assetId=" + encodeURIComponent(assetFilter.value) : ""}`,
      )
        .then((a) => {
          if (selection.value.length || assetSelection.value.length || organizing.value) return;
          assets.value = a.items;
          assetTotal.value = a.total;
        })
        .catch((e) =>
          failures.push(
            `已确认资产：${e instanceof Error ? e.message : "不可用"}`,
          ),
        ),
      request<Scan[]>("/library/scans")
        .then((s) => {
          scans.value = s;
        })
        .catch((e) =>
          failures.push(
            `扫描证据：${e instanceof Error ? e.message : "不可用"}`,
          ),
        ),
      request<Schedule[]>("/library/scan-schedules")
        .then((s) => {
          schedules.value = s;
        })
        .catch((e) =>
          failures.push(
            `扫描计划：${e instanceof Error ? e.message : "不可用"}`,
          ),
        ),
      request<Repair[]>("/library/artifact-repairs")
        .then((j) => {
          repairs.value = j;
        })
        .catch((e) =>
          failures.push(
            `输出恢复：${e instanceof Error ? e.message : "不可用"}`,
          ),
        ),
    ]);
    error.value = failures.length
      ? `部分读取失败，失败区域保留上次成功数据，不代表空目录：${failures.join("；")}`
      : "";
  } finally {
    loading.value = false;
  }
}
async function scan() {
  scanning.value = true;
  try {
    await post("/library/scans", { root: root.value.trim() });
    ElMessage.success("扫描已加入独立队列");
    await refresh();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "扫描创建失败");
  } finally {
    scanning.value = false;
  }
}
async function open(rows: Candidate[], asset: Asset | null = null) {
  if (rows.length > 20) {
    ElMessage.warning("每批最多20个文件");
    return;
  }
  const epoch = ++recognitionEpoch;
  batch.value = rows.map((candidate) => ({
    candidate,
    season: 0,
    episodes: "",
  }));
  editingAsset.value = asset;
  migrateOutputs.value = false;
  if (asset?.mediaType) mediaType.value = asset.mediaType;
  searchError.value = "";
  query.value = asset?.title ?? rows[0]?.name.replace(/\.[^.]+$/, "") ?? "";
  results.value = [];
  selected.value = null;
  season.value = 1;
  episodes.value = "";
  recognitionEvidence.value = [];
  dialog.value = true;
  if (!asset && rows.length) {
    recognitionBusy.value = true;
    try {
      for (const entry of batch.value) {
        const result = await post<{
          hint: {
            title: string;
            mediaType: string;
            season: number;
            episodes: number[];
          };
          matches: Identity[];
          selected?: Identity;
          evidence: string[];
          fileDigest: string;
        }>(`/library/candidates/${entry.candidate.id}/recognize`, {});
        if (epoch !== recognitionEpoch || !dialog.value) return;
        entry.season = result.hint.season;
        entry.episodes = result.hint.episodes.join(",");
        entry.fileDigest = result.fileDigest;
        recognitionEvidence.value.push(
          `${entry.candidate.name}：${result.evidence.join("；")}`,
        );
        if (rows.length === 1) {
          query.value = result.hint.title;
          if (["movie", "series"].includes(result.hint.mediaType))
            mediaType.value = result.hint.mediaType;
          results.value = result.matches;
          selected.value = result.selected ?? null;
        }
      }
    } catch (e) {
      if (epoch === recognitionEpoch)
        recognitionEvidence.value.push(
          e instanceof Error ? e.message : "文件身份读取失败；请先完成扫描",
        );
    } finally {
      if (epoch === recognitionEpoch) recognitionBusy.value = false;
    }
  }
}
async function search() {
  const q = query.value.trim();
  if (!q) {
    searchError.value = "请输入媒体标题或关键词";
    return;
  }
  const epoch = ++searchEpoch,
    type = mediaType.value;
  lookup.value = true;
  searchError.value = "";
  selected.value = null;
  try {
    const out = await request<Identity[]>(
      `/metadata/search?mediaType=${type}&q=${encodeURIComponent(q)}`,
    );
    if (epoch !== searchEpoch || !dialog.value) return;
    results.value = out;
    if (!out.length) searchError.value = "TMDB无匹配结果，请调整关键词";
  } catch (e) {
    if (epoch === searchEpoch)
      searchError.value = e instanceof Error ? e.message : "媒体查询失败";
  } finally {
    if (epoch === searchEpoch) lookup.value = false;
  }
}
function episodeList(value: string) {
  const out = value.trim() ? value.split(",").map((v) => Number(v.trim())) : [];
  if (
    out.some((v) => !Number.isInteger(v) || v < 1 || v > 9999) ||
    new Set(out).size !== out.length
  )
    throw new Error("集号必须是逗号分隔、不重复的正整数");
  if (selected.value?.mediaType === "series" && !out.length)
    throw new Error("请逐文件填写实际覆盖集");
  return out;
}
watch(
  () => route.query.assetId,
  (value) => {
    assetFilter.value = typeof value === "string" ? value : "";
    assetPage.value = 1;
    if (assetFilter.value) workspaceTab.value = "assets";
    refresh();
  },
);
watch(
  () => [
    dialog.value,
    selected.value?.id,
    selected.value?.mediaType,
    season.value,
    episodes.value,
    JSON.stringify(batch.value.map((r) => [r.season, r.episodes])),
  ],
  () => {
    episodePreviewEpoch++;
    episodePreview.value = [];
    episodePreviewError.value = "";
    episodePreviewLoading.value = false;
  },
);
async function previewEpisodes() {
  if (selected.value?.mediaType !== "series") return;
  const epoch = ++episodePreviewEpoch;
  episodePreviewLoading.value = true;
  episodePreviewError.value = "";
  try {
    const currentSeason = editingAsset.value
      ? season.value
      : (batch.value[0]?.season ?? 1);
    const coverage = episodeList(
      editingAsset.value ? episodes.value : (batch.value[0]?.episodes ?? ""),
    );
    const result = await request<EpisodeMetadata[]>(
      `/metadata/episodes?tmdbId=${selected.value.id}&season=${currentSeason}&episodes=${coverage.join(",")}`,
    );
    if (epoch === episodePreviewEpoch && dialog.value)
      episodePreview.value = result;
  } catch (e) {
    if (epoch === episodePreviewEpoch)
      episodePreviewError.value =
        e instanceof Error ? e.message : "单集信息未验证";
  } finally {
    if (epoch === episodePreviewEpoch) episodePreviewLoading.value = false;
  }
}
async function confirm() {
  if (!selected.value) return;
  saving.value = true;
  try {
    const identity = {
      mediaType: selected.value.mediaType,
      tmdbId: selected.value.id,
    };
    if (editingAsset.value) {
      const payload = {
        ...identity,
        season: season.value,
        episodes: episodeList(episodes.value),
        revision: editingAsset.value.revision,
        migrateOutputs: migrateOutputs.value,
      };
      if (migrateOutputs.value) {
        migrationPreview.value = await post<MigrationPreview>(
          `/media/assets/${editingAsset.value.id}/identity-preview`,
          payload,
        );
        migrationRequest.value = {
          ...payload,
          planDigest: migrationPreview.value.planDigest,
        };
        dialog.value = false;
        migrationDialog.value = true;
        return;
      }
      await post(`/media/assets/${editingAsset.value.id}/identity`, payload);
      ElMessage.success("身份已修正，文件链接和输出路径保持不变");
    } else {
      const items = batch.value.map((row) => ({
        id: row.candidate.id,
        confirmation: {
          ...identity,
          season: row.season,
          episodes: episodeList(row.episodes),
          revision: row.candidate.revision,
          fileDigest: row.fileDigest,
        },
      }));
      await ElMessageBox.confirm(
        `将${items.length}个文件绑定到「${selected.value.title}」。请确认每个文件的媒体身份和季集；失败条目不会冒充成功。`,
        "确认入库",
        { confirmButtonText: "确认生成", cancelButtonText: "返回检查" },
      );
      const outcome = await post<
        { id: string; succeeded: boolean; message: string }[]
      >("/library/candidates/batch-confirm", { items });
      const failures = outcome.filter((v) => !v.succeeded);
      if (failures.length) {
        ElMessage.warning(
          `${outcome.length - failures.length}项成功，${failures.length}项失败；刷新后逐条检查`,
        );
      } else ElMessage.success("STRM/NFO已生成，播放仍需真实客户端验收");
    }
    dialog.value = false;
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  } finally {
    saving.value = false;
  }
}
async function applyMigration() {
  if (!migrationPreview.value || !migrationRequest.value) return;
  migrationApplying.value = true;
  try {
    await post(
      `/media/assets/${migrationPreview.value.assetId}/identity`,
      migrationRequest.value,
    );
    migrationDialog.value = false;
    ElMessage.success("新输出已就绪，旧托管输出已归档；文件播放链接保持不变");
    await refresh();
  } catch (e) {
    ElMessage.error(
      e instanceof Error ? e.message : "迁移未完成，请核对输出恢复状态",
    );
  } finally {
    migrationApplying.value = false;
  }
}
async function grant(asset: Asset, action: "revoke" | "rotate") {
  try {
    await ElMessageBox.confirm(
      action === "revoke"
        ? "固定文件链接将失效，已签发的临时CDN链接可能短期仍有效。"
        : "旧凭据将失效。STRM重写通过恢复日志执行，不覆盖手工修改的文件。",
      action === "revoke" ? "撤销文件播放" : "轮换播放凭据",
      { type: "warning" },
    );
    await post(`/media/assets/${asset.id}/grant/${action}`, {});
    ElMessage.success("操作已完成");
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
async function repair(row: Repair) {
  try {
    await ElMessageBox.confirm(
      "先核对权限，并回退或移开手工修改的冲突文件。恢复只接受缺失、原版本或目标版本，不覆盖其他内容。",
      "重试输出恢复",
      { confirmButtonText: "重试恢复", cancelButtonText: "取消" },
    );
    await post(`/library/artifact-repairs/${row.id}/retry`, {});
    ElMessage.success("文件已恢复");
    await refresh();
  } catch (e) {
    if (e instanceof Error) ElMessage.error(e.message);
  }
}
function watchLabel(state: string) {
  return (
    (
      {
        connecting: "正在连接",
        connected: "已连接",
        waiting: "等待重连",
        unavailable: "推送暂不可用",
        unsupported: "当前版本不支持",
        disabled: "未启用",
      } as Record<string, string>
    )[state] ?? "等待连接"
  );
}
function editSchedule(s?: Schedule) {
  scheduleForm.value = s
    ? {
        id: s.id,
        root: s.root,
        enabled: s.enabled,
        watchChanges: s.watchChanges ?? false,
        cron: s.cron,
        revision: s.revision,
      }
    : {
        id: "",
        root: root.value,
        enabled: false,
        watchChanges: false,
        cron: "0 * * * *",
        revision: 0,
      };
  scheduleDialog.value = true;
}
async function saveSchedule() {
  scheduleSaving.value = true;
  try {
    await post("/library/scan-schedules", scheduleForm.value);
    scheduleDialog.value = false;
    ElMessage.success("定时扫描已保存");
    await refresh();
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "保存失败");
  } finally {
    scheduleSaving.value = false;
  }
}
onMounted(() => {
  refresh();
  timer = setInterval(() => {
    // Keep an operator's selection stable until they explicitly refresh or
    // finish the batch; polling must never silently cancel a bulk action.
    if (!selection.value.length && !assetSelection.value.length && !organizing.value) void refresh(true);
  }, 10000);
});
onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ORGANIZE / 云盘索引</span>
      <h1>云盘索引</h1>
      <p>
        扫描、确认与入库分开处理。保留源文件，固定播放链接跟随索引中的当前路径。
      </p>
    </div>
    <el-button :loading="loading" @click="refresh()">刷新记录</el-button>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
  />

  <el-tabs v-model="workspaceTab" class="library-workspace"
    ><el-tab-pane label="待识别与确认" name="candidates"
      ><section class="panel section-gap">
        <div class="panel-title">
          <h2>
            待确认文件 <small>{{ total }}项</small>
          </h2>
          <div class="library-actions">
            <el-select
              v-model="filter"
              aria-label="候选状态"
              @change="
                page = 1;
                refresh();
              "
              ><el-option label="待确认" value="needs_confirmation" /><el-option
                label="输出待恢复"
                value="artifacts_pending" /><el-option
                label="需明确迁移"
                value="needs_output_reconciliation" /><el-option
                label="已确认"
                value="confirmed" /><el-option
                label="全部"
                value="" /></el-select
            ><el-button
              :disabled="selection.length === 0"
              @click="open(selection)"
              >确认所选 {{ selection.length || "" }}</el-button
            ><el-button
              :loading="organizing"
              :disabled="!selection.length || selection.length > 20"
              @click="organize('candidate')"
              >批量整理115 · {{ selection.length }}</el-button
            >
          </div>
        </div>
        <el-table
          :data="candidates"
          row-key="id"
          @selection-change="selection = $event"
          empty-text="没有此状态的文件。先配置CloudDrive2并扫描目录。"
          ><el-table-column
            type="selection"
            width="45"
            :selectable="
              (row: Candidate) =>
                row.state !== 'confirmed' &&
                row.state !== 'artifacts_pending' &&
                row.state !== 'needs_output_reconciliation' &&
                row.cloudState === 'present'
            "
          /><el-table-column
            prop="path"
            label="云盘文件"
            min-width="300"
          /><el-table-column label="入库阶段" min-width="220"
            ><template #default="s"
              ><span>{{ s.row.state }}</span>
              <p
                v-if="s.row.state === 'needs_output_reconciliation'"
                class="field-hint"
              >
                已有资产{{
                  s.row.assetId
                }}；请使用下方资产的修正身份/季集，明确预览迁移，不再新确认。
              </p></template
            ></el-table-column
          ><el-table-column
            prop="cloudState"
            label="云端状态"
            width="130"
          /><el-table-column label="操作" width="100"
            ><template #default="s"
              ><el-button
                :disabled="
                  s.row.cloudState !== 'present' ||
                  s.row.state === 'confirmed' ||
                  s.row.state === 'artifacts_pending' ||
                  s.row.state === 'needs_output_reconciliation'
                "
                @click="open([s.row])"
                >确认</el-button
              ></template
            ></el-table-column
          ></el-table
        >
        <el-pagination
          v-model:current-page="page"
          :page-size="50"
          :total="total"
          layout="prev,pager,next,total"
          @current-change="refresh()"
        />
      </section> </el-tab-pane
    ><el-tab-pane label="已入库资产" name="assets"
      ><section class="panel section-gap">
        <p v-if="assetFilter" class="field-hint">
          当前只显示资产 {{ assetFilter }}
          <el-button
            size="small"
            @click="
              assetFilter = '';
              router.replace({ path: '/cloudStorage' });
              refresh();
            "
            >显示全部资产</el-button
          >
        </p>
        <div class="panel-title">
          <h2>
            已入库文件 <small>{{ assetTotal }}项</small>
          </h2>
          <el-button
            :loading="organizing"
            :disabled="!assetSelection.length || assetSelection.length > 20"
            @click="organize('asset')"
            >批量整理115 · {{ assetSelection.length }}</el-button
          >
        </div>
        <el-table
          :data="assets"
          empty-text="尚未确认入库文件。"
          @selection-change="assetSelection = $event"
          ><el-table-column type="selection" width="48" /><el-table-column
            label="媒体 / 路径"
            min-width="300"
            ><template #default="s"
              ><strong>{{ s.row.title }}</strong>
              <p class="field-hint">{{ s.row.path }}</p></template
            ></el-table-column
          ><el-table-column label="输出 / 云端" width="150"
            ><template #default="s"
              >{{ s.row.artifactsState }} / {{ s.row.state }}</template
            ></el-table-column
          ><el-table-column label="身份与播放凭据" min-width="280"
            ><template #default="s"
              ><el-button @click="open([], s.row)">修正身份 / 季集</el-button
              ><el-button
                :disabled="!s.row.fileScoped"
                @click="grant(s.row, 'revoke')"
                >撤销</el-button
              ><el-button @click="grant(s.row, 'rotate')"
                >轮换</el-button
              ></template
            ></el-table-column
          ></el-table
        ><el-pagination
          v-model:current-page="assetPage"
          :page-size="50"
          :total="assetTotal"
          layout="prev,pager,next,total"
          @current-change="refresh()"
        />
      </section> </el-tab-pane
    ><el-tab-pane label="扫描与计划" name="scans"
      ><section class="panel">
        <div class="panel-title">
          <h2>扫描目录</h2>
          <el-button @click="editSchedule()">新增定时扫描</el-button>
        </div>
        <div class="scan-toolbar">
          <el-input
            v-model="root"
            aria-label="115扫描根目录"
            placeholder="留空使用CloudDrive2配置的媒体根目录"
          /><el-button type="primary" :loading="scanning" @click="scan"
            >开始扫描</el-button
          >
        </div>
        <el-table
          :data="schedules"
          empty-text="没有定时扫描。新增配置默认停用，确认根目录后再启用。"
          ><el-table-column
            prop="root"
            label="定时范围"
            min-width="220"
          /><el-table-column label="Cron / 状态" min-width="200"
            ><template #default="s"
              >{{ s.row.cron }} ·
              {{ s.row.enabled ? "已启用" : "停用" }}</template
            ></el-table-column
          ><el-table-column label="变化推送" min-width="190"
            ><template #default="s"
              ><span>{{
                s.row.enabled && s.row.watchChanges
                  ? watchLabel(s.row.watchState)
                  : "未启用"
              }}</span>
              <p v-if="s.row.changeScanAfter" class="field-hint">
                已有变化待对账
              </p>
              <p class="field-hint">{{ s.row.watchMessage }}</p></template
            ></el-table-column
          ><el-table-column
            prop="lastError"
            label="最近提示"
            min-width="180"
          /><el-table-column label="操作" width="95"
            ><template #default="s"
              ><el-button @click="editSchedule(s.row)"
                >编辑</el-button
              ></template
            ></el-table-column
          ></el-table
        >
      </section>
      <ScanWorkbench :scans="scans" @changed="refresh" /></el-tab-pane
    ><el-tab-pane label="输出恢复与归档" name="recovery"
      ><el-empty
        v-if="!repairs.length"
        description="暂无待恢复输出；归档策略在下方独立批准" />
      <section v-if="repairs.length" class="panel section-gap">
        <div class="panel-title"><h2>输出待恢复</h2></div>
        <el-table :data="repairs"
          ><el-table-column
            prop="path"
            label="输出文件"
            min-width="250"
          /><el-table-column label="动作" width="100"
            ><template #default="s">{{
              s.row.operation === "archive" ? "归档" : "写入"
            }}</template></el-table-column
          ><el-table-column
            prop="kind"
            label="类型"
            width="100"
          /><el-table-column
            prop="state"
            label="恢复状态"
            width="120"
          /><el-table-column label="操作" width="120"
            ><template #default="s"
              ><el-button @click="repair(s.row)">重试恢复</el-button></template
            ></el-table-column
          ></el-table
        >
      </section>

      <ArchiveManager /></el-tab-pane
  ></el-tabs>
  <el-dialog
    v-model="dialog"
    :title="editingAsset ? '修正媒体身份与季集' : '确认媒体身份'"
    width="min(900px,95vw)"
    ><p v-if="editingAsset" class="field-hint">{{ editingAsset.path }}</p>
    <el-alert
      v-if="editingAsset"
      title="默认保持assetId、播放凭据和输出路径不变，按系统刮削选项更新绑定及托管输出。需要改变Movies/TV目录、剧集或季集时，勾选输出迁移并先预览；共享NFO冲突会被拒绝，不修改其他集。"
      type="info"
      :closable="false"
    />
    <el-alert
      v-if="recognitionBusy"
      title="正在核对当前文件与TMDB证据"
      type="info"
      :closable="false"
    />
    <ul v-if="recognitionEvidence.length" class="recognition-evidence">
      <li v-for="(evidence, index) in recognitionEvidence" :key="index">
        {{ evidence }}
      </li>
    </ul>
    <el-alert
      v-if="searchError"
      :title="searchError"
      type="warning"
      :closable="false"
    />
    <div class="metadata-search">
      <div>
        <label>媒体类型</label
        ><el-select v-model="mediaType" aria-label="媒体类型"
          ><el-option label="电影" value="movie" /><el-option
            label="剧集"
            value="series"
        /></el-select>
      </div>
      <div>
        <label>标题 / 关键词</label
        ><el-input
          v-model="query"
          aria-label="TMDB查询关键词"
          placeholder="输入电影或剧集名称"
          clearable
          @keyup.enter="search"
        />
      </div>
      <el-button
        :loading="lookup"
        :disabled="recognitionBusy || !query.trim()"
        @click="search"
        >查询TMDB</el-button
      >
    </div>
    <el-table
      :data="results"
      highlight-current-row
      @current-change="selected = $event"
      empty-text="输入关键词后查询，选择正确媒体。"
      ><el-table-column prop="title" label="标题" /><el-table-column
        prop="year"
        label="年份"
        width="90" /><el-table-column prop="id" label="ID" width="90"
    /></el-table>
    <p v-if="selected">
      将绑定：<strong>{{ selected.title }} ({{ selected.year }})</strong>
    </p>
    <el-table v-if="!editingAsset" :data="batch"
      ><el-table-column label="待确认文件" min-width="230"
        ><template #default="s">{{
          s.row.candidate.name
        }}</template></el-table-column
      ><el-table-column
        v-if="selected?.mediaType === 'series'"
        label="季"
        width="150"
        ><template #default="s"
          ><el-input-number
            v-model="s.row.season"
            :min="0"
            :max="999"
            aria-label="文件季号" /></template></el-table-column
      ><el-table-column
        v-if="selected?.mediaType === 'series'"
        label="实际覆盖集"
        min-width="160"
        ><template #default="s"
          ><el-input
            v-model="s.row.episodes"
            placeholder="例如1,2"
            aria-label="文件实际覆盖集" /></template></el-table-column
    ></el-table>
    <div
      v-if="editingAsset && selected?.mediaType === 'series'"
      class="rule-grid"
    >
      <div>
        <label>季</label
        ><el-input-number
          v-model="season"
          :min="0"
          :max="999"
          aria-label="修正季号"
        />
      </div>
      <div>
        <label>实际覆盖集</label
        ><el-input
          v-model="episodes"
          placeholder="1,2"
          aria-label="修正实际覆盖集"
        />
      </div>
    </div>
    <section v-if="selected?.mediaType === 'series'" class="episode-preview">
      <el-button :loading="episodePreviewLoading" @click="previewEpisodes">{{
        editingAsset ? "预览单集信息" : "预览首个文件的单集信息"
      }}</el-button>
      <p class="field-hint">
        按已选剧集和明确季集向TMDB核对；仅预览，不写文件。保存时仍重新校验。连续合集保留范围；非连续合集生成各集索引，共用同一个固定链接。播放仍读取完整合集，不切片、不按单集跳转，不自动补中间集。
      </p>
      <el-alert
        v-if="episodePreviewError"
        :title="episodePreviewError"
        type="error"
        :closable="false"
      />
      <ul class="migration-plan" aria-label="单集元数据预览">
        <li v-for="ep in episodePreview" :key="ep.id">
          <strong
            >S{{ String(ep.season).padStart(2, "0") }}E{{
              String(ep.number).padStart(2, "0")
            }}
            · {{ ep.title }}</strong
          >
          <p class="field-hint">
            {{ ep.airDate || "播出日期未知" }} ·
            {{ ep.runtimeMinutes ? ep.runtimeMinutes + "分钟" : "时长未知" }}
          </p>
          <p>{{ ep.overview || "单集简介暂无数据" }}</p>
        </li>
      </ul>
    </section>
    <el-checkbox v-if="editingAsset" v-model="migrateOutputs"
      >同时迁移Movies/TV输出目录（先预览）</el-checkbox
    >
    <p v-if="editingAsset && migrateOutputs" class="field-hint">
      改变媒体类型、剧集或季集时生成新输出；旧托管文件归档，不移动网盘媒体、不轮换播放Key、不清理共享tvshow.nfo。非连续合集的新输出将包含多个索引，仍播放同一个完整源文件。
    </p>
    <template #footer
      ><el-button @click="dialog = false">取消</el-button
      ><el-button
        type="primary"
        :disabled="!selected || recognitionBusy"
        :loading="saving"
        @click="confirm"
        >{{
          editingAsset
            ? migrateOutputs
              ? "预览输出迁移"
              : "保存身份修正"
            : "确认并生成STRM/NFO"
        }}</el-button
      ></template
    ></el-dialog
  >
  <el-dialog
    v-model="migrationDialog"
    append-to-body
    top="6vh"
    title="确认输出迁移"
    width="min(740px,95vw)"
    :close-on-click-modal="false"
    ><el-alert
      title="仅处理托管STRM/NFO/字幕：新文件完成后归档旧输出。网盘/做种源文件及播放Key不变；手工修改或归属冲突会阻止迁移。"
      type="warning"
      :closable="false"
    /><el-alert
      class="projection-warning"
      v-if="
        migrationPreview?.outputProjectionMode === 'explicit_episode_aliases'
      "
      title="非连续合集将生成所标集号的多个STRM索引，共用原固定链接；不会补中间集。每个条目都播放同一个完整源文件，不切片、不按集号跳转。"
      type="info"
      :closable="false"
    />
    <ul class="migration-plan" aria-label="输出迁移计划">
      <li
        v-for="op in migrationPreview?.operations ?? []"
        :key="op.operation + op.path"
      >
        <div class="migration-plan-action">
          <strong>{{
            op.operation === "archive" ? "归档旧输出" : "生成新输出"
          }}</strong
          ><span class="muted">{{ op.kind }}</span>
        </div>
        <code>{{ op.path }}</code>
      </li>
    </ul>
    <p class="field-hint">
      旧输出保留在.media-tool-retired目录，使用不可播放的.retired扩展名；目录和共享tvshow.nfo不自动删除。预览后文件或身份变化将要求重新预览。
    </p>
    <template #footer
      ><el-button :disabled="migrationApplying" @click="migrationDialog = false"
        >返回检查</el-button
      ><el-button
        type="primary"
        :loading="migrationApplying"
        :disabled="!migrationPreview"
        @click="applyMigration"
        >确认生成并归档</el-button
      ></template
    ></el-dialog
  >
  <el-dialog v-model="scheduleDialog" title="定时扫描" width="min(560px,95vw)"
    ><el-form label-position="top"
      ><el-form-item label="扫描根目录"
        ><el-input
          v-model="scheduleForm.root"
          :disabled="!!scheduleForm.id"
          placeholder="留空使用配置根目录" /></el-form-item
      ><el-form-item label="Cron 表达式"
        ><el-input v-model="scheduleForm.cron" placeholder="0 * * * *" />
        <p class="field-hint">
          分 时 日 月 周；Asia/Shanghai，最少5分钟
        </p></el-form-item
      ><el-form-item label="启用"
        ><el-switch
          v-model="scheduleForm.enabled"
          aria-label="启用定时扫描" /></el-form-item
      ><el-form-item label="文件变化推送"
        ><el-switch
          v-model="scheduleForm.watchChanges"
          aria-label="启用CloudDrive2文件变化推送"
        />
        <p class="field-hint">
          需同时启用此扫描计划。变化合并后对账，删除事件不直接移除资产或STRM；推送不支持/断线时仍保留周期扫描。
        </p></el-form-item
      ></el-form
    ><el-alert
      title="按指定子目录进行周期对账；失败扫描不会标记缺失，连续两次完整扫描未见才确认缺失。默认不删除STRM。"
      type="info"
      :closable="false"
    /><template #footer
      ><el-button @click="scheduleDialog = false">取消</el-button
      ><el-button type="primary" :loading="scheduleSaving" @click="saveSchedule"
        >保存定时扫描</el-button
      ></template
    ></el-dialog
  >
  <FileBatchWorkbench
    v-if="organization"
    ref="organizer"
    :root="organization.root"
    :files="organization.files"
    :initial-rows="organization.rows"
    @completed="refresh()"
  />
  <section class="note-panel">
    <strong>安全边界</strong>
    <p>
      手工修改或其他程序创建的输出文件不会被覆盖。暂停、缺失和待恢复不是播放成功；最终播放需通过真实Emby客户端验证。
    </p>
  </section>
</template>
<style scoped>
.library-workspace {
  margin-top: 20px;
}
.library-workspace :deep(.el-tabs__content) {
  overflow: visible;
}
.metadata-search {
  display: grid;
  grid-template-columns: 140px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: end;
  margin: 20px 0;
}
.metadata-search > div {
  min-width: 0;
}
.metadata-search label {
  display: block;
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 8px;
}
.metadata-search .el-select {
  width: 100%;
}
.metadata-search .el-button {
  min-height: 44px;
  margin: 0;
  white-space: nowrap;
}
.metadata-search :deep(.el-input__wrapper),
.metadata-search :deep(.el-select__wrapper) {
  min-height: 44px;
}
.library-workspace .panel {
  margin-bottom: 24px;
}
@media (max-width: 680px) {
  .metadata-search {
    grid-template-columns: 1fr;
  }
  .library-workspace :deep(.el-tabs__nav-wrap) {
    overflow: auto;
  }
}
.recognition-evidence {
  padding: 16px 16px 16px 32px;
  border: 1px solid var(--border);
  background: var(--surface);
  font-size: 13px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.projection-warning {
  background: #eaf3f4;
}
:deep(.projection-warning .el-alert__title) {
  color: #263b45;
  font-weight: 500;
}
.migration-plan {
  list-style: none;
  padding: 0;
  margin: 16px 0;
  max-height: 45vh;
  overflow: auto;
}
.migration-plan li {
  padding: 12px 0;
  border-bottom: 1px solid var(--border);
}
.migration-plan-action {
  display: flex;
  gap: 12px;
  margin-bottom: 6px;
}
.migration-plan code {
  display: block;
  white-space: normal;
  overflow-wrap: anywhere;
  font-size: 13px;
  line-height: 1.6;
}
.panel.section-gap {
  margin-bottom: 0;
}
.panel + .panel {
  margin-top: 24px;
}
:deep(.el-table__empty-text) {
  width: 90%;
  max-width: 520px;
  line-height: 1.7;
  padding: 16px 0;
}
.scan-toolbar,
.library-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}
.scan-toolbar {
  margin-bottom: 16px;
}
.library-actions .el-select {
  width: 150px;
}
.el-pagination {
  margin-top: 18px;
  overflow: auto;
}
.panel-title small {
  font-size: 13px;
  font-weight: 400;
  color: var(--muted);
}
@media (max-width: 680px) {
  .scan-toolbar,
  .library-actions {
    flex-wrap: wrap;
  }
  .panel-title {
    align-items: flex-start;
    gap: 12px;
  }
  .scan-toolbar .el-input {
    width: 100%;
  }
}
</style>
