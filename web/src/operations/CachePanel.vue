<script setup lang="ts">
import { pageResult } from './model';
import { formatBytes as bytes } from './model';
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import { cacheTime } from '../utils/cache-time';
import { tmdbImage } from '../utils/tmdb-image';
import { date, errorText } from '../workflows/readiness';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
const notice = ref(''), posterErrors = ref(new Set<string>());
const route = useRoute(), router = useRouter(), kind = computed<'tmdb' | 'search'>(() => route.query.cacheKind === 'search' ? 'search' : 'tmdb'), page = ref(1), loadedAt = ref(Date.now()), now = ref(Date.now()), data = reactive(useResource<any>(() => request(`/cache/${kind.value}?page=${page.value}`).then(pageResult))), detail = ref<any>(null), open = ref(false), detailError = ref<unknown>(null), op = reactive(useOperation('cache')), confirm = ref(false), removeId = ref(''), snapshot = reactive(useResource<any>(() => request('/settings/application'))), draft = reactive({ searchCacheSeconds: 0, tmdbCacheSeconds: 0 }), revision = ref(0), latest = ref<any>(null), ttlOp = reactive(useOperation('cache-ttl'));
function setKind(value: string) {
  void router.replace({ query: { ...route.query, cacheKind: value } });
}
async function load() {
  const out = await data.load();
  if (out)
    loadedAt.value = Date.now();
  if (op.pending && !data.error) {
    const p = JSON.parse(op.pending.key);
    if (p.kind === kind.value) {
      if (op.pending.status === 200)
        op.clear();
      else if (p.id)
        try {
          await request(`/cache/${p.kind}/${encodeURIComponent(p.id)}`);
        }
        catch (e: any) {
          if (e.status === 404)
            op.clear();
        }
    }
  }
}
async function settings() {
  const out = await snapshot.load();
  if (out && !out.value) {
    snapshot.error = Error('设置回执不完整，请重新读取。');
    return;
  }
  if (out && !revision.value) {
    Object.assign(draft, { searchCacheSeconds: out.value.searchCacheSeconds, tmdbCacheSeconds: out.value.tmdbCacheSeconds });
    revision.value = out.revision;
  }
  else if (out && out.revision !== revision.value)
    latest.value = out;
  if (ttlOp.pending && out && out.revision !== Number(ttlOp.pending.key))
    ttlOp.clear();
}
async function saveTTL() {
  if (latest.value || ttlOp.pending || !snapshot.data)
    return;
  const value = { ...snapshot.data.value, ...draft };
  await ttlOp.run(String(revision.value), () => request('/settings/application', { method: 'PUT', body: JSON.stringify({ value, revision: revision.value }) }));
  await settings();
  if (!ttlOp.error && !ttlOp.pending && snapshot.data) {
    revision.value = snapshot.data.revision;
    latest.value = null;
  }
  else if (ttlOp.error && snapshot.error)
    latest.value = { revision: null };
}
async function inspect(id: string) {
  detailError.value = null;
  open.value = true;
  detail.value = null;
  try {
    detail.value = await request(`/cache/${kind.value}/${encodeURIComponent(id)}`);
  }
  catch (e) {
    detailError.value = e;
  }
}
function ask(id = '') {
  removeId.value = id;
  confirm.value = true;
}
async function apply() {
  const k = kind.value, id = removeId.value;
  const out = await op.run(JSON.stringify({ kind: k, id }), () => id ? request(`/cache/${k}/${encodeURIComponent(id)}?confirm=true`, { method: 'DELETE' }) : post(`/cache/${k}/clear`, { confirm: true }));
  if (out && op.pending?.status === 200)
    notice.value = id ? '此缓存项已处理，请核对当前列表。' : `本轮删除 ${out.deleted ?? '—'} 项；${out.remainingMayExist ? '仍可能有更多缓存，请核对后分批清理。' : '请核对当前列表。'}`;
  confirm.value = false;
  page.value = 1;
  await load();
}
watch(kind, () => {
  page.value = 1;
  data.data = null;
  open.value = false;
  void load();
});
let timer: ReturnType<typeof setInterval>;
onMounted(() => {
  void load();
  void settings();
  timer = setInterval(() => now.value = Date.now(), 1000);
});
onUnmounted(() => clearInterval(timer));
defineExpose({ load, settings, saveTTL, draft, revision, latest, ttlOp, op, apply, ask, data, kind, inspect });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>缓存管理</h2>
        <el-button :disabled="!!op.pending||!!data.error" @click="ask()">清理此类缓存</el-button>
      </header>
      <nav class="media-actions" aria-label="缓存范围">
        <el-button :aria-pressed="kind==='tmdb'" @click="setKind('tmdb')">TMDB</el-button>
        <el-button :aria-pressed="kind==='search'" @click="setKind('search')">搜索缓存</el-button>
      </nav>
      <p>只管理缓存复用；清理搜索缓存会隐藏记录，底层结果、下载证明和媒体保留。</p>
      <ResourceState :loading="data.loading" :error="data.error" :empty="!!data.data&&!data.data.items?.length" @reload="load"/>
      <p>当前检索共 {{ data.loading||data.error?'—':data.data?.total??'—' }} 项 · 本页 {{ data.loading||data.error?'—':bytes(data.data?.items.reduce((n:number,r:any)=>n+(r.bytes||0),0)) }}</p>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="data.data?.truncated">扫描存在上限，当前列表未必覆盖全部缓存；可分批清理并重新读取。</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" message="缓存清理结果尚未确认，请先读取当前记录，不重复清理。" @refresh="load"/>
      <article v-for="row in data.error?[]:data.data?.items||[]" :key="row.id" class="organize-record">
        <div class="u6-cache-content">
          <img v-if="tmdbImage(row.posterPath)&&!posterErrors.has(row.id)" :src="tmdbImage(row.posterPath)" :alt="row.query+'海报'" loading="lazy" @error="posterErrors.add(row.id)"/>
          <div>
            <h3>{{ row.query||'媒体查询' }}</h3>
            <p>{{ row.count??'—' }} 个结果 · {{ cacheTime(kind,row,(now-loadedAt)/1000) }} · {{ date(row.createdAt) }}</p>
          </div>
        </div>
        <div class="media-actions">
          <RouterLink v-if="kind==='search'" :to="'/search/results/'+encodeURIComponent(row.id)">查看搜索结果</RouterLink>
          <el-button @click="inspect(row.id)">缓存详情</el-button>
          <el-button :disabled="!!op.pending" @click="ask(row.id)">删除缓存项</el-button>
        </div>
      </article>
      <el-pagination v-if="data.data?.total>30" v-model:current-page="page" :total="data.data.total" :page-size="30" layout="prev,pager,next" @current-change="load"/>
    </section>
    <section class="media-surface">
      <h2>缓存有效期</h2>
      <ResourceState :loading="snapshot.loading" :error="snapshot.error" @reload="settings"/>
      <form class="u5-form" @submit.prevent="saveTTL">
        <label>
          站点搜索缓存有效期（秒）
          <input v-model.number="draft.searchCacheSeconds" type="number" min="0" max="86400" aria-label="站点搜索缓存有效期（秒）"/>
        </label>
        <label>
          TMDB 搜索缓存有效期（秒）
          <input v-model.number="draft.tmdbCacheSeconds" type="number" min="0" max="2592000" aria-label="TMDB 搜索缓存有效期（秒）"/>
        </label>
        <p>0 表示不复用缓存；设置只修改这两个有效期，不覆盖其他设置。</p>
        <p v-if="latest">配置已被修改，已重新读取；你的输入仍保留供对照。</p>
        <p v-if="latest?.value">当前保存值：站点搜索 {{ latest.value.searchCacheSeconds??'—' }} 秒 · TMDB 搜索 {{ latest.value.tmdbCacheSeconds??'—' }} 秒。</p>
        <el-button v-if="latest" :disabled="latest.revision==null" @click="revision=latest.revision;latest=null">保留输入并采用最新修订</el-button>
        <el-alert v-if="ttlOp.error" :title="errorText(ttlOp.error)" type="error" :closable="false"/>
        <AsyncResultBanner v-if="ttlOp.phase" :status="ttlOp.phase" message="有效期保存结果尚未确认，请先重新读取核对。" @refresh="settings"/>
        <el-button native-type="submit" :disabled="!!latest||!!ttlOp.pending||!revision||!!snapshot.error">保存缓存有效期</el-button>
      </form>
    </section>
    <ConfirmAction v-model:visible="confirm" :title="removeId?'删除缓存项':'清理此类缓存'" description="确认清理所选范围？" :impact-scope="kind==='search'?'从搜索缓存移除并停止复用，保留底层结果、下载记录和媒体。':'仅删除当前影视缓存，不删除媒体或其他业务数据。'" confirm-text="确认清理" :loading="op.busy" class="media-dialog" @confirm="apply"/>
    <DetailDrawer v-model:visible="open" title="缓存内容" :technical-data="detail?.data??detail" class="media-detail">
      <ResourceState :error="detailError" @reload="load"/>
      <h3>{{ detail?.query||'—' }}</h3>
    </DetailDrawer>
  </div>
</template>
