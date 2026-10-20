import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request } from '../api/client';
import { image, rows, type Home, type MediaItem } from './model';
export function useCatalog() {
  const route = useRoute(), router = useRouter(), source = ref(['managed', 'emby', 'plugins'].includes(String(route.query.source)) ? String(route.query.source) : 'managed'), view = ref(route.query.view === 'all' ? 'all' : 'home'), page = ref(1), query = ref(''), kind = ref(''), parentId = ref(''), user = ref(''), pluginKind = ref('discover.source'), items = ref<MediaItem[]>([]), total = ref<number | null>(null), home = ref<Home | null>(null), hasMore = ref(false), truncated = ref(false), loading = ref(true), error = ref<unknown>(null), detail = ref<MediaItem | null>(null), detailOpen = ref(false), detailLoading = ref(false), detailError = ref<unknown>(null);
  const groups = computed(() => [{ key: 'libraries', title: '媒体库分类' }, { key: 'playing', title: '正在播放' }, { key: 'resume', title: '继续观看' }, { key: 'recent', title: '近期观看' }, { key: 'latest', title: '最新入库' }].map(g => ({ ...g, items: rows((home.value as any)?.[g.key]) })));
  let epoch = 0, detailEpoch = 0, disposed = false;
  async function load() {
    const current = ++epoch;
    loading.value = true;
    error.value = null;
    try {
      if (source.value === 'plugins') {
        const out = await request<any>(`/media/library/sources?kind=${pluginKind.value}&page=${page.value}&size=24`);
        if (current !== epoch || disposed)
          return;
        items.value = rows(out.items).map(v => ({ ...v, id: `${v.instanceId}:${v.mediaType}:${v.tmdbId}`, source: v.source || 'plugin' }));
        hasMore.value = out.hasMore;
        truncated.value = out.truncated;
        total.value = null;
      }
      else if (source.value === 'emby' && view.value === 'home') {
        const out = await request<Home>(`/media/library/emby/home?userId=${encodeURIComponent(user.value)}`);
        if (current !== epoch || disposed)
          return;
        home.value = out;
        user.value = out.userId;
      }
      else {
        const out = await request<any>(`/media/library?source=${source.value}&q=${encodeURIComponent(query.value)}&mediaType=${kind.value}&page=${page.value}&pageSize=24&parentId=${encodeURIComponent(parentId.value)}`);
        if (current !== epoch || disposed)
          return;
        items.value = rows(out.items);
        total.value = out.total;
      }
    }
    catch (e) {
      if (current === epoch && !disposed) {
        items.value = [];
        home.value = null;
        total.value = null;
        error.value = e;
      }
    }
    finally {
      if (current === epoch && !disposed)
        loading.value = false;
    }
  }
  function search() {
    page.value = 1;
    void load();
  }
  function chooseSource(value: string) {
    source.value = value;
    view.value = 'home';
    page.value = 1;
    parentId.value = '';
    detailOpen.value = false;
    detailEpoch++;
    void router.replace({ query: { ...route.query, source: value, view: 'home' } });
    void load();
  }
  function all() {
    view.value = 'all';
    page.value = 1;
    void router.replace({ query: { ...route.query, source: source.value, view: 'all' } });
    void load();
  }
  function browse(id: string) {
    parentId.value = id;
    view.value = 'all';
    search();
  }
  async function inspect(item: MediaItem) {
    const version = ++detailEpoch;
    detail.value = item;
    detailOpen.value = true;
    detailError.value = null;
    detailLoading.value = false;
    try {
      if (source.value === 'emby') {
        detailLoading.value = true;
        const out = await request<MediaItem>(`/media/emby/items/${encodeURIComponent(item.id)}`);
        if (version === detailEpoch && !disposed)
          detail.value = out;
      }
      else if (source.value === 'plugins' && item.tmdbId) {
        detailLoading.value = true;
        const out = await request<any>(`/metadata/details?mediaType=${item.mediaType}&tmdbId=${item.tmdbId}`);
        if (version === detailEpoch && !disposed)
          detail.value = { ...item, overview: out.item?.overview, poster: image(out.item?.posterPath) };
      }
    }
    catch (e) {
      if (version === detailEpoch && !disposed)
        detailError.value = e;
    }
    finally {
      if (version === detailEpoch && !disposed)
        detailLoading.value = false;
    }
  }
  function homeItem(group: string, item: MediaItem) {
    if (group === 'libraries')
      browse(item.id);
    else
      void inspect({ ...item, source: 'emby', mediaType: item.type, poster: item.image });
  }
  watch(() => route.query.source, value => {
    if (['managed', 'emby', 'plugins'].includes(String(value)) && value !== source.value) {
      source.value = String(value);
      view.value = route.query.view === 'all' ? 'all' : 'home';
      page.value = 1;
      parentId.value = '';
      detailOpen.value = false;
      detailEpoch++;
      void load();
    }
  });
  watch(detailOpen, v => {
    if (!v)
      detailEpoch++;
  });
  onMounted(load);
  onUnmounted(() => {
    disposed = true;
    epoch++;
    detailEpoch++;
  });
  return { source, view, page, query, kind, parentId, user, pluginKind, items, total, home, groups, hasMore, truncated, loading, error, detail, detailOpen, detailLoading, detailError, load, search, chooseSource, all, browse, inspect, homeItem };
}
