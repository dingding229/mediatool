<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue';
import { APIError, post, request } from '../api/client';
import ConfirmAction from '../ui/ConfirmAction.vue';
import PreviewApprove from '../ui/PreviewApprove.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { readIntent, writeIntent } from '../acquisition/model';
import { errorText } from '../workflows/readiness';
import { episodes, selectable } from './model';
const emit = defineEmits<{
  changed: [
  ];
  editing: [
    boolean
  ];
}>(), visible = ref(false), asset = ref<any>(null), batch = ref<any[]>([]), query = ref(''), kind = ref('movie'), results = ref<any[]>([]), selected = ref<any>(null), season = ref(1), coverage = ref(''), migrate = ref(false), lookup = ref(false), recognizing = ref(false), saving = ref(false), error = ref<unknown>(null), evidence = ref<string[]>([]), episodeItems = ref<any[]>([]), episodeBusy = ref(false), episodeError = ref(''), plan = ref<any>(null), payload = ref<any>(null), confirmOpen = ref(false), phase = ref<'queued' | 'reconciling' | null>(null), outcomes = ref<any[]>([]);
let generation = 0, searchVersion = 0, episodeVersion = 0, originPage = 1, originState = 'needs_confirmation';
watch(visible, v => {
  emit('editing', v);
  if (!v) {
    generation++;
    searchVersion++;
    episodeVersion++;
  }
});
watch(() => [selected.value?.id, selected.value?.mediaType, season.value, coverage.value, JSON.stringify(batch.value.map(r => [r.season, r.episodes])), migrate.value], () => {
  episodeVersion++;
  episodeItems.value = [];
  episodeError.value = '';
  episodeBusy.value = false;
  plan.value = null;
});
function subjects() {
  return (asset.value ? [asset.value.id] : batch.value.map(r => r.candidate.id)).map(id => 'identity-pending:' + id);
}
function pendingIdentity() {
  return subjects().map(key => readIntent(key)).find(Boolean);
}
function lockIdentity(value: any) {
  subjects().forEach(key => writeIntent(key, value));
}
function scope() {
  return 'identity:' + JSON.stringify([asset.value?.id, asset.value?.revision, batch.value.map(r => [r.candidate.id, r.candidate.revision, r.fileDigest]), selected.value?.id, selected.value?.mediaType, season.value, coverage.value, batch.value.map(r => [r.season, r.episodes]), migrate.value]);
}
async function recognize(preserve = false) {
  const version = ++generation;
  recognizing.value = true;
  evidence.value = [];
  error.value = null;
  try {
    for (const row of batch.value) {
      const out = await post<any>(`/library/candidates/${row.candidate.id}/recognize`, {});
      if (version !== generation)
        return;
      row.fileDigest = out.fileDigest;
      if (!preserve) {
        row.season = out.hint.season;
        row.episodes = (out.hint.episodes || []).join(',');
      }
      evidence.value.push(`${row.candidate.name}：${(out.evidence || []).join('；')}`);
      if (batch.value.length === 1 && !preserve) {
        query.value = out.hint.title;
        kind.value = ['movie', 'series'].includes(out.hint.mediaType) ? out.hint.mediaType : 'movie';
        results.value = out.matches || [];
        selected.value = out.selected || null;
      }
    }
  }
  catch (e) {
    if (version === generation)
      error.value = e;
  }
  finally {
    if (version === generation)
      recognizing.value = false;
  }
}
async function open(files: any[], item: any = null, context: any = {}) {
  originPage = context.page || 1;
  originState = context.state ?? 'needs_confirmation';
  generation++;
  phase.value = null;
  asset.value = item ? { ...item } : null;
  batch.value = files.map(candidate => ({ candidate: { ...candidate }, season: 0, episodes: '', fileDigest: '' }));
  query.value = item?.title || files[0]?.name.replace(/\.[^.]+$/, '') || '';
  kind.value = item?.mediaType || 'movie';
  selected.value = null;
  results.value = [];
  season.value = 1;
  coverage.value = '';
  migrate.value = false;
  plan.value = null;
  payload.value = null;
  error.value = null;
  outcomes.value = [];
  visible.value = true;
  phase.value = pendingIdentity()?.phase || null;
  if (!item)
    await recognize();
}
async function search() {
  if (!query.value.trim())
    return;
  const version = ++searchVersion;
  lookup.value = true;
  error.value = null;
  selected.value = null;
  try {
    const out = await request<any[]>(`/metadata/search?mediaType=${kind.value}&q=${encodeURIComponent(query.value.trim())}`);
    if (version === searchVersion && visible.value)
      results.value = out || [];
  }
  catch (e) {
    if (version === searchVersion)
      error.value = e;
  }
  finally {
    if (version === searchVersion)
      lookup.value = false;
  }
}
async function previewEpisodes() {
  if (selected.value?.mediaType !== 'series')
    return;
  const version = ++episodeVersion;
  episodeBusy.value = true;
  episodeError.value = '';
  try {
    const s = asset.value ? season.value : batch.value[0]?.season, c = episodes(asset.value ? coverage.value : batch.value[0]?.episodes || '', true);
    const out = await request<any[]>(`/metadata/episodes?tmdbId=${selected.value.id}&season=${s}&episodes=${c.join(',')}`);
    if (version === episodeVersion && visible.value)
      episodeItems.value = out || [];
  }
  catch (e) {
    if (version === episodeVersion)
      episodeError.value = errorText(e);
  }
  finally {
    if (version === episodeVersion)
      episodeBusy.value = false;
  }
}
async function prepare() {
  if (!selected.value || saving.value || phase.value)
    return;
  error.value = null;
  try {
    const identity = { mediaType: selected.value.mediaType, tmdbId: selected.value.id };
    if (asset.value) {
      payload.value = { ...identity, season: season.value, episodes: episodes(coverage.value, identity.mediaType === 'series'), revision: asset.value.revision, migrateOutputs: migrate.value };
      if (migrate.value) {
        if (!plan.value) {
          const version = episodeVersion;
          const next: any = await post(`/media/assets/${asset.value.id}/identity-preview`, payload.value);
          if (version !== episodeVersion || !visible.value)
            return;
          plan.value = next;
          payload.value = { ...payload.value, planDigest: next.planDigest };
          return;
        }
        payload.value = { ...payload.value, planDigest: plan.value.planDigest };
      }
    }
    else {
      if (batch.value.some(r => !selectable(r.candidate)))
        throw Error('文件状态已变化，请先核对已有资产或恢复输出，不重复确认。');
      if (batch.value.some(r => !r.fileDigest))
        throw Error('文件校验证据尚未取得，请先重新读取识别。');
      payload.value = { items: batch.value.map(r => ({ id: r.candidate.id, confirmation: { ...identity, season: r.season, episodes: episodes(r.episodes, identity.mediaType === 'series'), revision: r.candidate.revision, fileDigest: r.fileDigest } })) };
    }
    phase.value = readIntent(scope())?.phase || null;
    if (!phase.value)
      confirmOpen.value = true;
  }
  catch (e) {
    error.value = e;
  }
}
async function apply() {
  if (!payload.value || saving.value || phase.value)
    return;
  const key = scope();
  saving.value = true;
  phase.value = 'reconciling';
  writeIntent(key, { phase: 'reconciling' });
  lockIdentity({ phase: 'reconciling' });
  try {
    const out: any = asset.value ? await post(`/media/assets/${asset.value.id}/identity`, payload.value) : await post('/library/candidates/batch-confirm', payload.value);
    phase.value = 'queued';
    writeIntent(key, { phase: 'queued' });
    if (Array.isArray(out)) {
      outcomes.value = out;
      if (out.some(r => !r.succeeded)) {
        error.value = Error('部分文件尚未确认，请重新读取版本核对每项结果。');
      }
    }
    confirmOpen.value = false;
    emit('changed');
  }
  catch (e) {
    error.value = e;
    confirmOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      phase.value = null;
      writeIntent(key, null);
      lockIdentity(null);
      plan.value = null;
      payload.value = null;
      if (e.status === 409)
        error.value = Error('配置或文件版本已被修改，请保留输入并重新读取版本后再核对。');
    }
  }
  finally {
    saving.value = false;
  }
}
async function reconcile() {
  try {
    if (asset.value) {
      const out: any = await request(`/media/assets?page=1&pageSize=50&assetId=${asset.value.id}`);
      const fresh = (out.items || out).find((r: any) => r.id === asset.value.id);
      if (fresh?.revision !== asset.value.revision && fresh) {
        asset.value = { ...fresh };
        lockIdentity(null);
        phase.value = null;
        plan.value = null;
        payload.value = null;
      }
    }
    else {
      const out: any = await request(`/library/candidates?page=${originPage}&pageSize=50&state=${originState}`);
      let changed = true;
      for (const row of batch.value) {
        const current = out.items?.find((r: any) => r.id === row.candidate.id);
        if (current) {
          changed &&= current.revision !== row.candidate.revision;
          row.candidate = { ...current };
        }
        else
          changed = false;
      }
      if (changed) {
        lockIdentity(null);
        phase.value = null;
      }
      await recognize(true);
    }
    emit('changed');
  }
  catch (e) {
    error.value = e;
  }
}
onUnmounted(() => {
  generation++;
  searchVersion++;
  episodeVersion++;
});
defineExpose({ open });
</script>
<template>
  <el-dialog v-model="visible" :title="asset?'修正媒体身份与季集':'确认候选媒体'" width="min(780px,94vw)" :close-on-click-modal="false" :show-close="!saving" :close-on-press-escape="!saving" class="media-dialog">
    <div class="media-form">
      <p>逐文件核对准确作品与季集；未提供分段边界时，多集别名都播放同一个完整源文件。</p>
      <div class="media-filter-row">
        <label>
          媒体关键词
          <el-input v-model="query" aria-label="媒体关键词" :disabled="!!phase||saving"/>
        </label>
        <label>
          媒体类型
          <el-select v-model="kind" aria-label="媒体类型" :disabled="!!phase||saving">
            <el-option value="movie" label="电影"/>
            <el-option value="series" label="剧集"/>
          </el-select>
        </label>
        <el-button :disabled="!!phase||saving" :loading="lookup" @click="search">查询TMDB</el-button>
      </div>
      <p v-if="!lookup&&!results.length">请输入标题查询，或先重新读取识别线索。</p>
      <div class="media-choice-list" role="group" aria-label="匹配影视">
        <button v-for="item in results" :key="item.mediaType+item.id" type="button" :disabled="!!phase||saving" :aria-pressed="selected?.id===item.id&&selected?.mediaType===item.mediaType" @click="selected=item">{{ item.title }} · {{ item.year||'—' }} · {{ item.mediaType==='series'?'剧集':'电影' }}</button>
      </div>
      <template v-if="asset">
        <el-button :disabled="saving" @click="reconcile">核对媒体识别</el-button>
        <label>
          修正季度
          <el-input-number :controls="false" v-model="season" aria-label="修正季度" :min="0" :max="9999" :disabled="!!phase||saving"/>
        </label>
        <label>
          修正实际覆盖集
          <el-input v-model="coverage" aria-label="修正实际覆盖集" placeholder="逗号分隔，如 1,3" :disabled="!!phase||saving"/>
        </label>
        <p v-if="coverage.includes(',')">每个条目播放同一个完整源文件，不切片、不按单集跳转。</p>
        <el-checkbox v-model="migrate" :disabled="!!phase||saving">同时迁移Movies/TV输出目录（先预览）</el-checkbox>
        <p>不勾选时只修正身份，不变更文件、输出位置或固定播放链接。</p>
      </template>
      <template v-else>
        <article v-for="row in batch" :key="row.candidate.id" class="organize-file-draft">
          <strong>{{ row.candidate.name }}</strong>
          <p>{{ row.candidate.path }}</p>
          <label>
            季度
            <el-input-number :controls="false" v-model="row.season" :aria-label="'季度 '+row.candidate.name" :min="0" :max="9999" :disabled="!!phase||saving"/>
          </label>
          <label>
            实际覆盖集
            <el-input v-model="row.episodes" :aria-label="'实际覆盖集 '+row.candidate.name" :disabled="!!phase||saving"/>
          </label>
        </article>
        <el-button :loading="recognizing" :disabled="saving" @click="reconcile">核对结果</el-button>
        <p v-for="text in evidence" :key="text">{{ text }}</p>
      </template>
      <el-button v-if="selected?.mediaType==='series'" :disabled="saving||!!phase" :loading="episodeBusy" @click="previewEpisodes">预览单集信息</el-button>
      <p v-if="episodeError" role="alert">{{ episodeError }}</p>
      <article v-for="ep in episodeItems" :key="ep.id">
        <strong>S{{ String(ep.season).padStart(2,'0') }}E{{ String(ep.number).padStart(2,'0') }} · {{ ep.title }}</strong>
        <p>{{ ep.airDate||'播出日期 —' }} · {{ ep.runtimeMinutes?ep.runtimeMinutes+' 分钟':'时长 —' }}</p>
        <p>{{ ep.overview||'—' }}</p>
      </article>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <p v-for="result in outcomes" :key="result.id">{{ result.succeeded?'输出确认已受理':'需处理' }} · {{ result.message||'—' }}</p>
      <AsyncResultBanner v-if="phase" :can-refresh="false" :status="phase" message="确认结果需核对输出与记录，不重复提交同一确认。" @refresh="reconcile"/>
      <PreviewApprove v-if="plan" :current-step="2">
        <h3>输出迁移预览</h3>
        <p>仅预览，不写入文件；原媒体与固定链接保持不变。</p>
        <p v-if="plan.outputProjectionMode==='sharedWholeFileEpisodeAliases'||plan.wholeFilePlayback">每个条目都播放同一个完整源文件，没有自动切片或重新封装。</p>
        <div v-for="(operation,index) in plan.operations" :key="index">
          <strong>{{ operation.operation==='archive'?'归档旧输出':'生成新输出' }}</strong>
          <p>{{ operation.path }}</p>
        </div>
      </PreviewApprove>
    </div>
    <template #footer>
      <el-button :disabled="saving" @click="visible=false">取消</el-button>
      <el-button v-if="asset" :disabled="saving" @click="reconcile">保留输入并核对版本</el-button>
      <el-button type="primary" :disabled="!selected||recognizing||!!phase" :loading="saving" @click="prepare" class="mt-save-button">{{ asset&&migrate?plan?'核对后确认迁移':'预览输出迁移':'确认入库或保存身份' }}</el-button>
    </template>
  </el-dialog>
  <ConfirmAction v-model:visible="confirmOpen" :title="migrate?'确认输出迁移':asset?'确认修正身份':'确认入库'" :description="`将核对并保存「${selected?.title||'—'}」的媒体绑定。`" :impact-scope="migrate?'新输出就绪后归档旧托管输出；不移动云端源文件、不改变文件播放链接，不覆盖手工内容。':'确认后可能生成 STRM / NFO；保留媒体和做种源，播放仍需真实播放器验证。'" :confirm-text="migrate?'确认生成并归档':'确认生成'" cancel-text="返回检查" :loading="saving" class="media-dialog" @confirm="apply"/>
</template>
