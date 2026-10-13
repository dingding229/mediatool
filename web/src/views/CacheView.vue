<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { Refresh, Delete } from "@element-plus/icons-vue";
import {tmdbImage} from "../utils/tmdb-image";
import StatusBadge from "../components/StatusBadge.vue";
import { request, post } from "../api/client";
interface Entry {
  id: string;
  posterPath?:string;
  createdAt?:string;
  query: string;
  state: string;
  bytes: number;
  ttlSeconds: number;
  count: number;
  data?: unknown;
}
const props = defineProps<{ kind: "tmdb" | "search" }>(),
  name = computed(() => (props.kind === "tmdb" ? "TMDB缓存" : "搜索缓存"));
const posterErrors=ref(new Set<string>());
const list = ref<Entry[]>([]),
  total = ref(0),
  page = ref(1),
  busy = ref(false),
  error = ref(""),
  truncated = ref(false),
  detail = ref<Entry | null>(null),
  dialog = ref(false);
const bytes = computed(() =>
  list.value.reduce((sum, row) => sum + row.bytes, 0),
);
function size(n: number) {
  return n > 1048576
    ? (n / 1048576).toFixed(2) + " MiB"
    : (n / 1024).toFixed(1) + " KiB";
}
let loadEpoch = 0;
async function load() {
  const epoch = ++loadEpoch;
  const kind = props.kind;
  busy.value = true;
  error.value = "";
  try {
    const out = await request<{
      items: Entry[];
      total: number;
      truncated: boolean;
    }>(`/cache/${kind}?page=${page.value}`);
    if (epoch !== loadEpoch || kind !== props.kind) return;
    list.value = out.items;
    total.value = out.total;
    truncated.value = out.truncated;
  } catch (e) {
    if (epoch === loadEpoch) error.value = String(e);
  } finally {
    if (epoch === loadEpoch) busy.value = false;
  }
}
async function inspect(row: Entry) {
  const kind = props.kind;
  try {
    const out = await request<Entry>(`/cache/${kind}/${row.id}`);
    if (kind !== props.kind) return;
    detail.value = out;
    dialog.value = true;
  } catch (e) {
    ElMessage.error(String(e));
    load();
  }
}
async function remove(row: Entry) {
  const kind = props.kind;
  try {
    await ElMessageBox.confirm(
      kind==='search'?"从搜索缓存移除此记录并停止复用，不删除底层结果、下载记录或业务数据。":"只删除此TMDB缓存项，不删除业务数据。",
      "删除缓存项",
      { type: "warning" },
    );
    await request(`/cache/${kind}/${row.id}?confirm=true`, {
      method: "DELETE",
    });
    ElMessage.success("缓存项已删除");
    await load();
  } catch (e) {
    if (e !== "cancel" && e !== "close") error.value = String(e);
  }
}
async function clear() {
  const kind = props.kind;
  try {
    await ElMessageBox.confirm(
      `清理全部${name.value}？${kind==='search'?'隐藏现有搜索记录并停止缓存复用，保留底层结果和下载证明。':'不删除其他类缓存或业务数据。'}`,
      "清理此类缓存",
      { type: "warning", confirmButtonText: "确认清理" },
    );
    const out = await post<{ deleted: number; remainingMayExist: boolean }>(
      `/cache/${kind}/clear`,
      { confirm: true },
    );
    ElMessage.success(
      `已删除${out.deleted}项${out.remainingMayExist ? "，仍可能有更多缓存，请刷新核对" : ""}`,
    );
    page.value = 1;
    await load();
  } catch (e) {
    if (e !== "cancel" && e !== "close") error.value = String(e);
  }
}
watch(
  () => props.kind,
  () => {
    page.value = 1;
    list.value = [];
    total.value = 0;
    dialog.value = false;
    detail.value = null;
    load();
  },
);
onMounted(load);
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 缓存管理</span>
      <h1>{{ name }}</h1>
      <p>
        {{
          kind === "tmdb"
            ? "媒体查询与详情的结果缓存"
            : "最近搜索取得的真实结果和记录"
        }}{{kind==='tmdb'?'，按系统设置自动到期。':'；索引到期后仍可查看历史结果。'}}
      </p>
    </div>
    <div class="heading-actions">
      <el-button :icon="Refresh" :loading="busy" @click="load"
        >刷新缓存</el-button
      ><el-button
        :icon="Delete"
        type="danger"
        plain
        :disabled="busy"
        @click="clear"
        >清理此类缓存</el-button
      >
    </div>
  </section>
  <section class="cache-summary">
    <article class="panel">
      <span>当前扫描项数</span><strong>{{ total }}</strong>
    </article>
    <article class="panel">
      <span>本页缓存内容大小</span><strong>{{ size(bytes) }}</strong>
    </article>
    <article class="panel">
      <span>有效期设置</span
      ><RouterLink to="/settings">系统设置中修改</RouterLink>
    </article>
  </section>
  <el-alert
    v-if="error"
    :title="error"
    type="error"
    :closable="false"
  /><el-alert
    v-if="truncated"
    title="当前扫描有上限，列表可能未覆盖全部缓存；请缩短TTL或分批清理。"
    type="warning"
    :closable="false"
  />
  <section class="panel">
    <el-table
      class="cache-desktop"
      :data="list"
      v-loading="busy"
      empty-text="暂无缓存，完成媒体或站点搜索后会自动记录"
      ><el-table-column label="内容" min-width="250"
        ><template #default="{ row }"
          ><div class="cache-content"><img v-if="tmdbImage(row.posterPath) && !posterErrors.has(row.id)" :src="tmdbImage(row.posterPath)" :alt="row.query+'海报'" loading="lazy" @error="posterErrors.add(row.id)"/><div v-else class="cache-poster" aria-hidden="true">影</div><div><strong>{{row.query || '媒体查询'}}</strong><p v-if="row.createdAt" class="field-hint">{{new Date(row.createdAt).toLocaleString('zh-CN')}}</p><StatusBadge :state="row.state"/></div></div></template
        ></el-table-column
      ><el-table-column
        prop="count"
        label="资源数量"
        width="100"
      /><el-table-column label="剩余时间" width="140"
        ><template #default="{ row }">{{
          kind==='search' && row.ttlSeconds===0 ? "历史结果" : row.ttlSeconds < 0 ? "无自动到期" : row.ttlSeconds + " 秒"
        }}</template></el-table-column
      ><el-table-column label="大小" width="120"
        ><template #default="{ row }">{{
          size(row.bytes)
        }}</template></el-table-column
      ><el-table-column label="操作" width="170"
        ><template #default="{ row }"
          ><RouterLink v-if="kind==='search'" :to="{path:'/search',query:{run:row.id}}">查看结果</RouterLink><el-button v-else @click="inspect(row)">查看</el-button><el-button text type="danger" @click="remove(row)"
            >删除</el-button
          ></template
        ></el-table-column
      ></el-table
>
    <div class="cache-mobile"><article v-for="row in list" :key="row.id"><div class="cache-content"><img v-if="tmdbImage(row.posterPath) && !posterErrors.has(row.id)" :src="tmdbImage(row.posterPath)" :alt="row.query+'海报'" loading="lazy" @error="posterErrors.add(row.id)"/><div v-else class="cache-poster" aria-hidden="true">影</div><div><strong>{{row.query || '媒体查询'}}</strong><p class="field-hint">{{row.count}} 个结果 · {{row.ttlSeconds>0 ? row.ttlSeconds+' 秒' : '历史结果'}}</p><StatusBadge :state="row.state"/></div></div><div class="cache-mobile-actions"><RouterLink v-if="kind==='search'" :to="{path:'/search',query:{run:row.id}}">查看结果</RouterLink><el-button v-else @click="inspect(row)">查看</el-button><el-button text type="danger" @click="remove(row)">删除</el-button></div></article><p v-if="!list.length" class="field-hint">暂无缓存，完成媒体或站点搜索后会自动记录。</p></div>
    <el-pagination
      v-if="total > 30"
      v-model:current-page="page"
      :total="total"
      :page-size="30"
      layout="prev,pager,next"
      @current-change="load"
    />
  </section>
  <el-dialog v-model="dialog" title="缓存内容" width="min(900px,94vw)"
    ><p class="field-hint">{{ detail?.query }}</p>
    <pre class="cache-json">{{
      JSON.stringify(detail?.data ?? detail, null, 2)
    }}</pre>
  </el-dialog>
</template>
<style scoped>
.cache-mobile{display:none}.cache-mobile article{padding:16px 0;border-bottom:1px solid var(--border)}.cache-mobile article:last-child{border-bottom:0}.cache-mobile-actions{display:flex;justify-content:flex-end;gap:16px;align-items:center;margin-top:12px}.cache-mobile-actions a{min-height:44px;display:flex;align-items:center}
.cache-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  margin-bottom: 24px;
}
.cache-summary span {
  display: block;
  color: var(--muted);
  font-size: 12px;
  margin-bottom: 8px;
}
.cache-summary strong {
  font-size: 26px;
}
.cache-content{display:flex;gap:16px;align-items:center}.cache-content img,.cache-poster{width:44px;height:66px;flex:0 0 44px;object-fit:cover;border-radius:4px}.cache-poster{display:grid;place-items:center;background:var(--canvas);color:var(--muted)}
.cache-key {
  overflow-wrap: anywhere;
}
.cache-json {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 65vh;
  overflow: auto;
  padding: 16px;
  background: #14252b;
  color: #dcece8;
  border-radius: 12px;
}
@media (max-width: 680px) {
 .cache-desktop{display:none}.cache-mobile{display:block}
  .cache-summary {
    grid-template-columns: 1fr;
  }
  .cache-summary article {
    padding: 16px;
  }
}
</style>
