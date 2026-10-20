<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue';
import { request } from '../api/client';
import { tmdbImage } from '../utils/tmdb-image';
import { errorText } from '../workflows/readiness';
import type { Media } from './model';
const props = defineProps<{
  keyword: string;
  initial?: Media | null;
  disabled?: boolean;
}>(), emit = defineEmits<{
  selected: [
    Media
  ];
}>();
const item = ref<Media | null>(null), candidates = ref<Media[]>([]), selecting = ref(false), details = ref(false), busy = ref(false), error = ref<unknown>(null), broken = ref(false);
let epoch = 0;
async function select(media: Media, notify = true) {
  const version = ++epoch;
  busy.value = true;
  selecting.value = false;
  error.value = null;
  broken.value = false;
  item.value = media;
  try {
    const out = await request<{
      item: Media;
    }>(`/metadata/details?mediaType=${media.mediaType}&tmdbId=${media.id}`);
    if (version !== epoch)
      return;
    if (out.item) {
      item.value = out.item;
      if (notify)
        emit('selected', out.item);
    }
  }
  catch (e) {
    if (version === epoch)
      error.value = e;
  }
  finally {
    if (version === epoch)
      busy.value = false;
  }
}
async function suggest(automatic = false) {
  if (props.disabled)
    return;
  const version = ++epoch;
  busy.value = true;
  error.value = null;
  try {
    const out = await request<{
      items: Media[];
    }>(`/search/media?mediaType=auto&q=${encodeURIComponent(props.keyword)}`);
    if (version !== epoch)
      return;
    candidates.value = out.items || [];
    const norm = (s: string) => s.replace(/[\s._-]+/g, '').toLocaleLowerCase();
    const exact = candidates.value.filter(m => [m.title, m.originalTitle || ''].some(s => norm(s) === norm(props.keyword)));
    if (automatic && exact.length === 1)
      await select(exact[0]!);
    else if (!automatic)
      selecting.value = true;
  }
  catch (e) {
    if (version === epoch)
      error.value = e;
  }
  finally {
    if (version === epoch)
      busy.value = false;
  }
}
watch(() => [props.keyword, props.initial] as const, () => {
  epoch++;
  if (props.initial)
    void select(props.initial, false);
  else {
    item.value = null;
    void suggest(true);
  }
}, { immediate: true });
onUnmounted(() => {
  epoch++;
});
</script>
<template>
  <section class="acquire-media-context media-context" aria-label="所选媒体信息">
    <template v-if="item">
      <img v-if="tmdbImage(item.posterPath)&&!broken" class="context-poster" :src="tmdbImage(item.posterPath)" :alt="item.title+'海报'" @error="broken=true"/>
      <div v-else class="acquire-poster-fallback">{{ item.title }}</div>
      <div>
        <h2>
          {{ item.title }}
          <small>{{ item.year||'—' }}</small>
        </h2>
        <p>{{ item.mediaType==='series'?'剧集':'电影' }} · {{ item.genres?.join(' / ')||'—' }}</p>
        <p>TMDB ID：{{ item.id }}</p>
        <p class="acquire-overview">{{ item.overview||'暂无简介' }}</p>
        <div class="acquire-actions">
          <el-button @click="details=true">查看详情</el-button>
          <el-button :disabled="disabled" :loading="busy" @click="suggest(false)">更换媒体</el-button>
        </div>
      </div>
    </template>
    <template v-else>
      <div>
        <h2>为“{{ keyword }}”核对作品</h2>
        <p>按关键词搜索不能证明媒体身份，投递前请选择准确作品。</p>
      </div>
      <el-button :disabled="disabled" :loading="busy" @click="suggest(false)">匹配媒体信息</el-button>
    </template>
    <p v-if="error" role="status">媒体详情暂不可用，已选身份保留。{{ errorText(error) }}</p>
  </section>
  <el-dialog v-model="selecting" title="选择媒体信息" width="min(680px,94vw)" class="acquire-dialog">
    <div class="acquire-choices">
      <button v-for="candidate in candidates" :key="candidate.mediaType+candidate.id" type="button" @click="select(candidate)">{{ candidate.title }} · {{ candidate.year||'—' }} · {{ candidate.mediaType==='series'?'剧集':'电影' }}</button>
    </div>
    <p v-if="!candidates.length">没有匹配作品，请调整关键词。</p>
  </el-dialog>
  <el-dialog v-model="details" title="媒体详情" width="min(680px,94vw)" class="acquire-dialog">
    <h2>{{ item?.title }}</h2>
    <p>{{ item?.originalTitle||'—' }} · {{ item?.year||'—' }}</p>
    <p>{{ item?.overview||'暂无简介' }}</p>
  </el-dialog>
</template>
