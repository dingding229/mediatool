<script setup lang="ts">
import { useRoute } from "vue-router";
import PromotionBadge from "../components/PromotionBadge.vue";
import RecentSearches from "../components/RecentSearches.vue";
import { computed, ref, onMounted, onUnmounted } from "vue";
import { Search, Film } from "@element-plus/icons-vue";
import { ElMessage, type TableInstance } from "element-plus";
import { request, post } from "../api/client";
import { resourceSources, sourceLabel } from "../utils/resource-source";
import SearchMediaContext from "../components/SearchMediaContext.vue";
import { tmdbImage } from "../utils/tmdb-image";
import StatusBadge from "../components/StatusBadge.vue";
interface Account {
  id: string;
  kind: string;
  name: string;
  enabled: boolean;
}
interface Resource {
  id: string;
  title: string;
  siteName: string;
  size: number;
  seeders: number;
  leechers: number;
  downloadFactor: number | null;
  metadata: {
    subtitle?: string;
    attributes?: { resolution: string; source: string; codec: string };
  };
}
interface Run {
  id: string;
  keyword: string;
  state: string;
  sites: {
    accountId: string;
    name: string;
    state: string;
    message: string;
    count: number;
  }[];
  resources: Resource[];
  total: number;
  media?: Media;
}
interface Media {
  posterPath?: string;
  overview?: string;
  id: number;
  mediaType: string;
  title: string;
  year: string;
  originalTitle?: string;
}
const codecLabel=(value?:string)=>value && value!=='unknown' ? value.toUpperCase() : '编码未知';
const persistedMedia=new Set<string>();
async function persistMedia(item:Media){if(run.value){const key=`${run.value.id}:${item.mediaType}:${item.id}`;if(persistedMedia.has(key))return;persistedMedia.add(key);try{await post(`/search/${run.value.id}/media`,{id:item.id,mediaType:item.mediaType});await load()}catch{persistedMedia.delete(key)}}}
const activeMedia = ref<Media | null>(null),
  contextRunID = ref("");
async function useMedia(item: Media) {
  activeMedia.value = item;
  keyword.value = item.title;
  await search(false);
}
const tmdbSearched = ref(false),
  posterErrors = ref(new Set<string>());
const resourceFilters = ref({
  resolution: [] as string[],
  source: [] as string[],
  codec: [] as string[],
  accountId: [] as string[],
  keyword: "",
  sort: "seeders_desc",
});
const resourceTable = ref<TableInstance>();
const tableSort = computed(() => ({
  prop: resourceFilters.value.sort.startsWith("size") ? "size" : "seeders",
  order: resourceFilters.value.sort.endsWith("asc")
    ? ("ascending" as const)
    : ("descending" as const),
}));
function changeSort({
  prop,
  order,
}: {
  prop: string;
  order: "ascending" | "descending" | null;
}) {
  if ((prop !== "size" && prop !== "seeders") || !order) return;
  const sort = `${prop}_${order === "ascending" ? "asc" : "desc"}`;
  if (sort === resourceFilters.value.sort) return;
  resourceFilters.value.sort = sort;
  applyFilter();
}
function toggleSort(prop: "size" | "seeders") {
  const order =
    tableSort.value.prop === prop && tableSort.value.order === "descending"
      ? "ascending"
      : "descending";
  resourceTable.value?.sort(prop, order);
}
const mode = ref("media"),
  foundMedia = ref<Media[]>([]),
  foundCached = ref(false),
  tmdbSearching = ref(false),
  tmdbQuery = ref(""),
  tmdbKind = ref("auto"),
  tmdbYear = ref(""),
  searchCached = ref(false);
const accounts = ref<Account[]>([]),
  keyword = ref(""),
  run = ref<Run | null>(null),
  history = ref<Run[]>([]),
  busy = ref(false),
  error = ref(""),
  page = ref(1);
const selectedRunId = ref(""),
  downloadError = ref("");
const dialog = ref(false),
  resource = ref<Resource | null>(null),
  mediaType = ref("auto"),
  mediaQuery = ref(""),
  media = ref<Media[]>([]),
  chosen = ref<Media | null>(null),
  metadataBusy = ref(false),
  downloading = ref(false);
let filterTimer: ReturnType<typeof setTimeout> | undefined;
function applyFilter() {
  clearTimeout(filterTimer);
  page.value = 1;
  refresh();
}
function queueFilter() {
  clearTimeout(filterTimer);
  filterTimer = setTimeout(applyFilter, 350);
}
let timer: ReturnType<typeof setInterval> | undefined;
let resultEpoch = 0,
  resultController: AbortController | undefined,
  resultLoading = false;
async function load() {
  try {
    accounts.value = (await request<Account[]>("/integrations")).filter(
      (a) => a.kind === "ptsite" && a.enabled,
    );
    history.value = await request<Run[]>("/search");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "初始化失败";
  }
}
async function search(force = false) {
  if (
    activeMedia.value &&
    ![activeMedia.value.title, activeMedia.value.originalTitle].includes(
      keyword.value.trim(),
    )
  )
    activeMedia.value = null;
  busy.value = true;
  error.value = "";
  run.value = null;
  page.value = 1;
  try {
    const created = await post<{ id: string; cached: boolean }>("/search", {
      keyword: keyword.value,
      refresh: force,
    });
    searchCached.value = created.cached;
    contextRunID.value = created.id;
    if(activeMedia.value) {try{await post(`/search/${created.id}/media`,{id:activeMedia.value.id,mediaType:activeMedia.value.mediaType})}catch{ElMessage.warning("搜索已启动，媒体海报暂未保存，可稍后重新匹配")}}
    await refresh(created.id);
    history.value = await request<Run[]>("/search");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "搜索启动失败";
  } finally {
    busy.value = false;
  }
}
function filterParams() {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(resourceFilters.value)) {
    if (Array.isArray(value)) {
      for (const item of value) params.append(key, item);
    } else params.set(key, value);
  }
  return params.toString();
}
async function refresh(id = run.value?.id) {
  if (!id) return;
  if (id !== contextRunID.value) {const media=history.value.find(x=>x.id===id)?.media;activeMedia.value=media?.id && ['movie','series'].includes(media.mediaType) ? media : null;}
  const epoch = ++resultEpoch;
  resultController?.abort();
  resultController = new AbortController();
  resultLoading = true;
  error.value = "";
  try {
    const result = await request<Run>(
      `/search/${id}?page=${page.value}&limit=50&${filterParams()}`,
      { signal: resultController.signal },
    );
    if (epoch === resultEpoch) run.value = result;
  } catch (e) {
    if (
      epoch !== resultEpoch ||
      (e instanceof DOMException && e.name === "AbortError")
    )
      return;
    error.value = e instanceof Error ? e.message : "搜索状态查询失败";
  } finally {
    if (epoch === resultEpoch) resultLoading = false;
  }
}
function openDownload(item: Resource) {
  selectedRunId.value = run.value?.id ?? "";
  downloadError.value = "";
  resource.value = item;
  mediaQuery.value = keyword.value;
  media.value = [];
  chosen.value = null;
  dialog.value = true;
}
async function findMedia(force: boolean) {
  if (tmdbSearching.value) return;
  tmdbSearching.value = true;
  error.value = "";
  foundMedia.value = [];
  foundCached.value = false;
  tmdbSearched.value = false;
  posterErrors.value.clear();
  try {
    const out = await request<{ items: Media[]; cached: boolean }>(
      `/search/media?mediaType=${tmdbKind.value}&q=${encodeURIComponent(tmdbQuery.value)}&year=${encodeURIComponent(tmdbYear.value)}&refresh=${force}`,
    );
    foundMedia.value = out.items;
    foundCached.value = out.cached;
    tmdbSearched.value = true;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "TMDB搜索失败";
  } finally {
    tmdbSearching.value = false;
  }
}
async function lookup() {
  metadataBusy.value = true;
  chosen.value = null;
  try {
    media.value = await request<Media[]>(
      `/metadata/search?mediaType=${mediaType.value}&q=${encodeURIComponent(mediaQuery.value)}`,
    );
  } catch (e) {
    ElMessage.error(e instanceof Error ? e.message : "TMDB 查询失败");
  } finally {
    metadataBusy.value = false;
  }
}
async function download() {
  if (!resource.value || !chosen.value) return;
  downloading.value = true;
  try {
    await post("/downloads", {
      searchRunId: selectedRunId.value,
      resourceId: resource.value.id,
      mediaType: chosen.value.mediaType,
      tmdbId: chosen.value.id,
    });
    dialog.value = false;
    ElMessage.success("下载任务已持久化。可在下载与传输查看进度。");
  } catch (e) {
    downloadError.value = e instanceof Error ? e.message : "下载任务创建失败";
    ElMessage.error(downloadError.value);
  } finally {
    downloading.value = false;
  }
}
function size(n: number) {
  return n >= 1024 ** 3
    ? (n / 1024 ** 3).toFixed(2) + " GiB"
    : (n / 1024 ** 2).toFixed(1) + " MiB";
}
const route=useRoute();
onMounted(() => {
  load().then(()=>{if(typeof route.query.run==='string')refresh(route.query.run)});
  timer = setInterval(() => {
    if (
      !resultLoading &&
      run.value &&
      ["pending", "running"].includes(run.value.state)
    )
      refresh();
  }, 2000);
});
onUnmounted(() => {
  clearTimeout(filterTimer);
  ++resultEpoch;
  resultController?.abort();
  if (timer) clearInterval(timer);
});
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 资源搜索</span>
      <h1>资源搜索</h1>
      <p>按站点显示结果与失败原因。选择下载前确认 TMDB 媒体身份。</p>
    </div>
    <RouterLink to="/sites" class="inline-link">管理PT站点账号</RouterLink>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
    show-icon
  />
  <section class="search-mode-tabs">
    <el-radio-group v-model="mode" aria-label="搜索方式"
      ><el-radio-button value="media">TMDB 媒体搜索</el-radio-button
      ><el-radio-button value="site">站点直搜</el-radio-button></el-radio-group
    >
  </section>
  <section v-if="mode === 'media'" class="panel section-gap">
    <div class="panel-title">
      <div>
        <span class="section-kicker">DISCOVER</span>
        <h2>先找到你想看的作品</h2>
      </div>
      <span class="muted">TMDB 元数据</span>
    </div>
    <form class="tmdb-search" @submit.prevent="findMedia(false)">
      <label class="search-field" for="tmdb-media-query"
        >媒体名称<el-input
          id="tmdb-media-query"
          v-model="tmdbQuery"
          aria-label="TMDB媒体名称"
          placeholder="电影或剧集名称"
          :prefix-icon="Search"
          clearable
      /></label>
      <label class="search-field" for="tmdb-media-kind"
        >类型<el-select
          id="tmdb-media-kind"
          v-model="tmdbKind"
          aria-label="TMDB媒体类型"
          ><el-option label="自动识别" value="auto" /><el-option
            label="电影"
            value="movie" /><el-option label="剧集" value="series" /></el-select
      ></label>
      <label class="search-field" for="tmdb-media-year"
        >年份（可选）<el-input
          id="tmdb-media-year"
          v-model="tmdbYear"
          aria-label="TMDB年份"
          placeholder="例如 2024"
          :maxlength="4"
          inputmode="numeric"
      /></label>
      <div class="search-submit-row">
        <el-button
          type="primary"
          native-type="submit"
          :loading="tmdbSearching"
          :disabled="!tmdbQuery.trim()"
          >搜索媒体</el-button
        ><el-button
          :disabled="tmdbSearching || !tmdbQuery.trim()"
          @click="findMedia(true)"
          >忽略TMDB缓存</el-button
        >
      </div>
    </form>
    <p class="field-hint">
      {{
        foundCached
          ? "本次使用TMDB缓存"
          : "TMDB查询与PT资源搜索独立，不会自动下载"
      }}。缓存有效期可在系统设置修改。
    </p>
    <el-skeleton
      v-if="tmdbSearching"
      :rows="4"
      animated
      aria-label="正在搜索媒体"
    />
    <template v-else-if="tmdbSearched"
      ><div class="search-results-heading">
        <h2>匹配作品</h2>
        <span class="muted"
          >{{ foundMedia.length }} 项 ·
          {{ foundCached ? "缓存结果" : "本次查询" }}</span
        >
      </div>
      <div class="tmdb-results">
        <article
          v-for="item in foundMedia"
          :key="item.mediaType + item.id"
          class="tmdb-media"
        >
          <div class="tmdb-poster">
            <img
              v-if="
                tmdbImage(item.posterPath, 'w342') &&
                !posterErrors.has(item.mediaType + item.id)
              "
              :src="tmdbImage(item.posterPath, 'w342')"
              :alt="item.title"
              loading="lazy"
              @error="posterErrors.add(item.mediaType + item.id)"
            /><Film v-else aria-label="暂无海报" />
          </div>
          <div>
            <h2>
<button type="button" class="media-title" :disabled="busy || !accounts.length" @click="useMedia(item)">{{ item.title }}</button><small
                >{{ item.year || "年份未知" }} ·
                {{ item.mediaType === "movie" ? "电影" : "剧集" }}</small              >
            </h2>
            <p>{{ item.originalTitle }}</p>
            <p class="field-hint">{{ item.overview || "暂无简介" }}</p>

          </div>
        </article>
      </div>
      <el-empty
        v-if="!foundMedia.length"
        description="没有找到匹配作品，请换个名称或调整年份。"
      />
    </template>
    <RecentSearches v-else-if="!error" :items="history" @select="page=1;refresh($event)" />
  </section>
  <template v-if="mode === 'site' || run">
    <SearchMediaContext
      v-if="run"
      :key="run.id"
      :keyword="run.keyword"
      :initial="activeMedia"
      @selected="persistMedia"
    />
    <section v-if="mode==='site'" class="panel section-gap">
      <form @submit.prevent="search(false)" class="search-form">
        <el-input
          v-model="keyword"
          placeholder="电影名称、剧集名称或发布关键词"
          aria-label="资源关键词"
          size="large"
        /><el-button
          type="primary"
          native-type="submit"
          :loading="busy"
          :disabled="!keyword.trim() || accounts.length === 0"
          >开始搜索</el-button
        >
      </form>
      <p class="field-hint">
        {{
          searchCached
            ? "本次复用有效的搜索快照"
            : "按全部启用站点直搜，错误与挑战会如实显示"
        }}
      </p>
      <el-button
        :disabled="busy || !keyword.trim() || !accounts.length"
        @click="search(true)"
        >忽略站点缓存重新搜索</el-button
      >
      <p class="field-hint">默认搜索全部 {{accounts.length}} 个已启用 PT 站点。</p>
      <div v-if="run" class="site-progress">
        <div v-for="site in run.sites" :key="site.accountId">
          <strong>{{ site.name }}</strong
          ><StatusBadge
            :state="site.state"
            :label="site.state === 'partial' ? '部分结果' : undefined"
          /><span class="muted">{{
            site.message || `${site.count} 个结果`
          }}</span>
        </div>
      </div>
    </section>

    <section v-if="run" class="panel">
      <div class="resource-filter-grid">
        <div>
          <label>分辨率</label
          ><el-select
            v-model="resourceFilters.resolution"
            multiple
            clearable
            collapse-tags
            collapse-tags-tooltip
            aria-label="资源分辨率"
            placeholder="全部分辨率"
            @change="
              page = 1;
              refresh();
            "
            ><el-option
              v-for="x in [
                '480p',
                '720p',
                '1080p',
                '2160p',
                '4320p',
                'unknown',
              ]"
              :key="x"
              :label="x === 'unknown' ? '未知' : x"
              :value="x"
          /></el-select>
        </div>
        <div>
          <label>来源</label
          ><el-select
            v-model="resourceFilters.source"
            multiple
            clearable
            collapse-tags
            collapse-tags-tooltip
            aria-label="资源来源"
            placeholder="全部来源"
            @change="
              page = 1;
              refresh();
            "
            ><el-option
              v-for="option in resourceSources"
              :key="option.value"
              :label="option.label"
              :value="option.value"
          /></el-select>
        </div>
        <div>
          <label>编码</label
          ><el-select
            v-model="resourceFilters.codec"
            multiple
            clearable
            collapse-tags
            collapse-tags-tooltip
            aria-label="资源编码"
            placeholder="全部编码"
            @change="
              page = 1;
              refresh();
            "
            ><el-option
              v-for="x in ['h264', 'h265', 'av1', 'unknown']"
              :key="x"
              :label="x==='unknown' ? '未知' : x.toUpperCase()"
              :value="x"
          /></el-select>
        </div>
        <div>
          <label>站点</label
          ><el-select
            v-model="resourceFilters.accountId"
            multiple
            clearable
            collapse-tags
            collapse-tags-tooltip
            aria-label="结果站点"
            placeholder="全部站点"
            @change="
              page = 1;
              refresh();
            "
            ><el-option
              v-for="x in run.sites"
              :key="x.accountId"
              :label="x.name"
              :value="x.accountId"
          /></el-select>
        </div>
        <div>
          <label>关键字筛选</label
          ><el-input
            v-model="resourceFilters.keyword"
            aria-label="资源关键字筛选"
            placeholder="筛选标题或副标题，空格分隔多个关键字"
            clearable
            :maxlength="128"
            @input="queueFilter"
            @keyup.enter="applyFilter"
          />
        </div>
      </div>
      <p class="field-hint">
        筛选整个已取得快照，共
        {{ run.total }} 条匹配；标题识别属性仅为线索。部分结果仍按站点提示核对。
      </p>
      <el-alert
        v-if="run.state === 'partial'"
        title="这里只展示已取得的部分结果。请查看各站点原因，缩小关键词或核对分页配置；没有显示不等于站点没有资源。"
        type="warning"
        :closable="false"
        role="status"
      />
      <div class="panel-title">
        <h2>搜索结果</h2>
        <StatusBadge
          :state="run.state"
          :label="run.state === 'partial' ? '部分结果' : undefined"
        />
      </div>
      <el-table
        ref="resourceTable"
        :default-sort="tableSort"
        @sort-change="changeSort"
        :data="run.resources"
        :empty-text="
          ['pending', 'running'].includes(run.state)
            ? 'Worker 正在查询站点…'
            : '没有匹配资源；如有站点失败，请先检查上方原因。'
        "
        ><el-table-column label="资源" min-width="290"
          ><template #default="scope"
            ><strong>{{ scope.row.title }}</strong>
            <p class="field-hint">{{ scope.row.metadata?.subtitle }}</p>
            <small>{{scope.row.siteName}} · {{codecLabel(scope.row.metadata?.attributes?.codec)}} · {{sourceLabel(scope.row.metadata?.attributes?.source)}} · {{scope.row.metadata?.attributes?.resolution || '分辨率未知'}}</small></template
          ></el-table-column
        ><el-table-column
          prop="size"
          label="大小"
          width="126"
          sortable="custom"
          :sort-orders="['descending', 'ascending']"
        >
          <template #header
            ><button
              type="button"
              class="column-sort"
              aria-label="按文件大小排序"
              @click.stop="toggleSort('size')"
            >
              大小
            </button></template
          ><template #default="scope">{{
            size(scope.row.size)
          }}</template></el-table-column
        ><el-table-column
          prop="seeders"
          label="做种人数"
          width="126"
          sortable="custom"
          :sort-orders="['descending', 'ascending']"
          ><template #header
            ><button
              type="button"
              class="column-sort"
              aria-label="按做种人数排序"
              @click.stop="toggleSort('seeders')"
            >
              做种人数
            </button></template
          ></el-table-column
        ><el-table-column label="促销" width="110"><template #default="scope"><PromotionBadge :factor="scope.row.downloadFactor"/></template></el-table-column
        ><el-table-column label="操作" width="140"
          ><template #default="scope"
            ><el-button type="primary" plain @click="openDownload(scope.row)"
              >确认并下载</el-button
            ></template
          ></el-table-column
        ></el-table
      ><el-pagination
        v-if="run.total > 50"
        v-model:current-page="page"
        :page-size="50"
        :total="run.total"
        layout="prev,pager,next"
        @current-change="refresh()"
      />
    </section>
    <section v-else class="panel search-history"><RecentSearches :items="history" @select="page=1;refresh($event)"/></section>
  </template>
  <el-dialog
    v-model="dialog"
    title="确认媒体身份与下载"
    width="min(760px,94vw)"
    :close-on-click-modal="false"
    ><p class="field-hint">{{ resource?.title }}</p>
    <el-alert
      title="下载后保留原文件做种；上传与入库状态独立。匹配不确定时不要选择相似标题。"
      type="info"
      :closable="false"
    />
    <div class="search-form">
      <el-select v-model="mediaType" aria-label="媒体类型"
        ><el-option label="自动识别" value="auto" /><el-option
          label="电影"
          value="movie" /><el-option label="剧集" value="series" /></el-select
      ><el-input v-model="mediaQuery" aria-label="TMDB关键词" /><el-button
        :loading="metadataBusy"
        @click="lookup"
        >查询 TMDB</el-button
      >
    </div>
    <el-table
      :data="media"
      highlight-current-row
      @current-change="chosen = $event"
      ><el-table-column prop="title" label="标题" /><el-table-column
        prop="year"
        label="年份"
        width="90" /><el-table-column
        prop="originalTitle"
        label="原名" /><el-table-column prop="id" label="TMDB ID" width="100"
    /></el-table>
    <p class="field-hint">
      下载使用打开弹窗时选中的搜索快照；过期或站点配置变化时请重新搜索。
    </p>
    <el-alert
      v-if="downloadError"
      :title="downloadError"
      type="error"
      :closable="false"
      role="alert"
    />
    <p class="field-hint">
      当前确认：{{
        chosen ? `${chosen.title} (${chosen.year})` : "请点击正确的媒体条目"
      }}
    </p>
    <el-button
      type="primary"
      :loading="downloading"
      :disabled="!chosen"
      @click="download"
      >创建下载任务</el-button
    ></el-dialog
  >
</template>

<style scoped>
.column-sort {
  border: 0;
  background: transparent;
  padding: 10px 0;
  color: inherit;
  font: inherit;
  font-weight: 600;
  cursor: pointer;
  min-height: 44px;
}
.column-sort:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
  border-radius: 4px;
}
.search-history-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 120px minmax(0, 1fr);
  align-items: center;
  gap: 16px;
  padding: 16px 0;
}
.search-history-row + .search-history-row {
  border-top: 1px solid var(--border);
}
.history-keyword {
  min-width: 0;
  overflow-wrap: anywhere;
}
.history-status {
  justify-self: start;
}
.search-history-row :deep(.el-button) {
  justify-self: end;
  margin: 0;
  min-height: 44px;
}
@media (max-width: 680px) {
  .search-history-row {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 10px;
  }
  .history-keyword {
    grid-column: 1/-1;
  }
  .history-status {
    grid-column: 1;
  }
  .search-history-row :deep(.el-button) {
    grid-column: 2;
  }
}

.resource-filter-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 14px;
}
.resource-filter-grid label {
  display: block;
  font-size: 12px;
  color: var(--muted);
  margin-bottom: 6px;
}
.resource-filter-grid .el-select,
.resource-filter-grid .el-input-number {
  width: 100%;
}
@media (max-width: 680px) {
  .resource-filter-grid {
    grid-template-columns: 1fr 1fr;
  }
}
</style>

<style scoped>.media-title{background:none;border:0;padding:0;text-align:left;font:inherit;font-weight:700;color:var(--primary);cursor:pointer;min-height:44px}.media-title:hover{text-decoration:underline}.media-title:disabled{color:var(--muted);cursor:wait}</style>

<style scoped>.tmdb-media{background:none;border:0;padding:16px 0;border-bottom:1px solid var(--border);border-radius:0}.tmdb-poster{width:88px;height:132px}.tmdb-media h2{margin-top:0}@media(max-width:600px){.tmdb-poster{width:64px;height:96px}}</style>
