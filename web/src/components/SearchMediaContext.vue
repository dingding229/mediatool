<script setup lang="ts">
import { ref, watch } from "vue";
import { useRouter } from "vue-router";
import { Search, Collection, InfoFilled } from "@element-plus/icons-vue";
import { request } from "../api/client";
export interface SearchMedia {
  id: number;
  mediaType: string;
  title: string;
  originalTitle?: string;
  year: string;
  overview?: string;
  posterPath?: string;
  backdropPath?: string;
  genres?: string[];
}
const props = defineProps<{ keyword: string; initial?: SearchMedia | null }>(),
  router = useRouter();
const item = ref<SearchMedia | null>(null),
  candidates = ref<SearchMedia[]>([]),
  selecting = ref(false),
  details = ref(false),
  busy = ref(false),
  error = ref(""),
  posterError = ref(false);
let sequence = 0;
function image(path: string | undefined, size = "w500") {
  return path && /^\/[A-Za-z0-9_-]+\.(?:jpg|png|webp)$/.test(path)
    ? `https://image.tmdb.org/t/p/${size}${path}`
    : "";
}
async function select(media: SearchMedia) {
  const current = ++sequence;
  item.value = media;
  selecting.value = false;
  busy.value = true;
  error.value = "";
  posterError.value = false;
  try {
    const out = await request<{ item: SearchMedia }>(
      `/metadata/details?mediaType=${media.mediaType}&tmdbId=${media.id}`,
    );
    if (current === sequence) item.value = out.item;
  } catch (e) {
    if (current === sequence) error.value = String(e);
  } finally {
    if (current === sequence) busy.value = false;
  }
}
const normalize = (value: string) =>
  value.replace(/[\s._-]+/g, "").toLocaleLowerCase();
async function suggest(automatic = false) {
  if (!props.keyword.trim()) return;
  const current = ++sequence;
  busy.value = true;
  error.value = "";
  try {
    const out = await request<{ items: SearchMedia[] }>(
      `/search/media?mediaType=auto&q=${encodeURIComponent(props.keyword)}`,
    );
    if (current !== sequence) return;
    candidates.value = out.items;
    if (automatic) {
      const exact = out.items.filter(
        (media) =>
          normalize(media.title) === normalize(props.keyword) ||
          normalize(media.originalTitle || "") === normalize(props.keyword),
      );
      if (exact.length === 1) {
        await select(exact[0]);
        return;
      }
    } else selecting.value = true;
  } catch (e) {
    if (current === sequence) error.value = String(e);
  } finally {
    if (current === sequence) busy.value = false;
  }
}

watch(
  () => [props.keyword, props.initial] as const,
  () => {
    ++sequence;
    item.value = null;
    error.value = "";
    if (props.initial) select(props.initial);
    else suggest(true);
  },
  { immediate: true },
);
function subscribe() {
  if (item.value)
    router.push({
      path: "/subscriptions",
      query: {
        create: "1",
        mediaType: item.value.mediaType,
        tmdbId: String(item.value.id),
      },
    });
}
</script>
<template>
  <section
    v-if="item"
    class="media-context"
    :style="
      image(item.backdropPath, 'w1280')
        ? {
            backgroundImage: `linear-gradient(90deg,rgba(18,38,42,.32),rgba(18,38,42,.12)),url(${image(item.backdropPath, 'w1280')})`,
          }
        : {}
    "
  >
    <img
      v-if="image(item.posterPath) && !posterError"
      :src="image(item.posterPath)"
      :alt="item.title + '海报'"
      class="context-poster"
      @error="posterError = true"
    />
    <div v-else class="context-poster poster-empty">{{ item.title }}</div>
    <article class="context-copy">
      <div class="context-title">
        <div>
          <span class="section-kicker">TMDB媒体信息</span>
          <h2>
            {{ item.title }} <small v-if="item.year">({{ item.year }})</small>
          </h2>
        </div>
        <el-button text @click="suggest(false)">更换媒体</el-button>
      </div>
      <p>
        {{ item.mediaType === "series" ? "电视剧" : "电影"
        }}<span v-if="item.genres?.length">
          · {{ item.genres.join(" / ") }}</span
        >
      </p>
      <p class="field-hint">TMDB ID：{{ item.id }}</p>
      <div class="context-actions">
        <el-button type="primary" :icon="Collection" @click="subscribe"
          >订阅</el-button
        ><el-button :icon="InfoFilled" @click="details = true"
          >查看详情</el-button
        >
      </div>
      <p class="context-overview">{{ item.overview || "暂无简介" }}</p>
      <p v-if="error" class="field-hint" role="status">
        详情暂不可用；已选择身份保留。{{ error }}
      </p>
    </article>
  </section>
  <section v-else class="panel context-empty">
    <div>
      <span class="section-kicker">MEDIA CONTEXT</span>
      <h2>为“{{ keyword }}”选择媒体信息</h2>
      <p class="field-hint">
        站点关键词不能证明媒体身份，先选择TMDB作品再展示海报、简介和订阅入口。
      </p>
      <p v-if="error" role="alert">{{ error }}</p>
    </div>
    <el-button :icon="Search" :loading="busy" @click="suggest(false)"
      >匹配媒体信息</el-button
    >
  </section>
  <el-dialog v-model="selecting" title="选择媒体信息" width="min(720px,94vw)"
    ><el-table
      :data="candidates"
      empty-text="没有匹配作品，请调整关键词或从TMDB媒体搜索进入"
      ><el-table-column prop="title" label="名称" /><el-table-column
        label="类型"
        width="100"
        ><template #default="{ row }">{{
          row.mediaType === "series" ? "剧集" : "电影"
        }}</template></el-table-column
      ><el-table-column prop="year" label="年份" width="100" /><el-table-column
        label="操作"
        width="110"
        ><template #default="{ row }"
          ><el-button @click="select(row)">选择</el-button></template
        ></el-table-column
      ></el-table
    ></el-dialog
  ><el-dialog v-model="details" title="媒体详情" width="min(720px,94vw)"
    ><h2>{{ item?.title }} ({{ item?.year }})</h2>
    <p class="field-hint">{{ item?.originalTitle }} · TMDB {{ item?.id }}</p>
    <p class="detail-overview">{{ item?.overview || "暂无简介" }}</p></el-dialog
  >
</template>
<style scoped>
.media-context {
  display: grid;
  grid-template-columns: 170px minmax(0, 1fr);
  gap: 24px;
  background: var(--accent-soft);
  background-size: cover;
  background-position: center;
  border-radius: 18px;
  padding: 24px;
  margin-bottom: 24px;
}
.context-poster {
  width: 170px;
  height: 250px;
  object-fit: cover;
  border-radius: 12px;
  align-self: center;
}
.poster-empty {
  display: grid;
  place-items: center;
  background: var(--accent);
  color: white;
  padding: 14px;
  text-align: center;
}
.context-copy {
  padding: 24px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.93);
  color: #20363e;
}
.context-title {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}
.context-title h2 {
  margin: 8px 0;
  font-size: 22px;
}
.context-title small {
  font-size: 18px;
}
.context-actions {
  display: flex;
  gap: 10px;
  margin: 12px 0;
}
.context-actions .el-button {
  margin: 0;
  min-height: 44px;
}
.context-overview {
  line-height: 1.75;
  display: -webkit-box;
  -webkit-line-clamp: 5;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.context-empty {
  display: flex;
  gap: 18px;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 24px;
}
.context-empty h2 {
  font-size: 18px;
  margin: 8px 0;
}
.detail-overview {
  line-height: 1.85;
  white-space: pre-wrap;
}
@media (max-width: 680px) {
  .media-context {
    padding: 16px;
    gap: 16px;
    grid-template-columns: 96px minmax(0, 1fr);
  }
  .context-poster {
    width: 96px;
    height: 144px;
    align-self: start;
  }
  .context-copy {
    padding: 16px;
    min-width: 0;
  }
  .context-title {
    flex-direction: column;
  }
  .context-title h2 {
    font-size: 18px;
  }
  .context-actions {
    flex-wrap: wrap;
  }
  .context-overview {
    grid-column: 1/-1;
  }
  .context-empty {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
