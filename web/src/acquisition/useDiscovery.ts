import { onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request } from '../api/client';
import { rows, type MediaItem } from '../media/model';
export function useDiscovery() {
  const route = useRoute(), router = useRouter(), kind = ref(route.query.kind === 'recommend.source' ? 'recommend.source' : 'discover.source'), page = ref(1), items = ref<MediaItem[]>([]), hasMore = ref(false), truncated = ref(false), loading = ref(true), error = ref<unknown>(), detail = ref<MediaItem | null>(null), detailOpen = ref(false), detailLoading = ref(false), detailError = ref<unknown>();
  let epoch = 0, detailEpoch = 0, alive = true;
  async function load() {
    const version = ++epoch;
    loading.value = true;
    error.value = undefined;
    items.value = [];
    hasMore.value = false;
    truncated.value = false;
    try {
      const out = await request<any>(`/media/library/sources?kind=${kind.value}&page=${page.value}&limit=24`);
      if (alive && version === epoch) {
        items.value = rows(out.items).map(v => ({ ...v, id: `${v.instanceId}:${v.mediaType}:${v.tmdbId}`, source: v.source || 'plugin' }));
        hasMore.value = !!out.hasMore;
        truncated.value = !!out.truncated;
      }
    }
    catch (e) {
      if (alive && version === epoch)
        error.value = e;
    }
    finally {
      if (alive && version === epoch)
        loading.value = false;
    }
  }
  async function inspect(item: MediaItem) {
    const version = ++detailEpoch;
    detail.value = item;
    detailOpen.value = true;
    detailLoading.value = true;
    detailError.value = undefined;
    try {
      if (item.tmdbId) {
        const out = await request<any>(`/metadata/details?mediaType=${item.mediaType}&tmdbId=${item.tmdbId}`);
        if (alive && version === detailEpoch)
          detail.value = { ...item, ...out.item, id: item.id, tmdbId: item.tmdbId, instanceId: item.instanceId, source: item.source, poster: out.item?.posterPath || item.poster };
      }
    }
    catch (e) {
      if (alive && version === detailEpoch)
        detailError.value = e;
    }
    finally {
      if (alive && version === detailEpoch)
        detailLoading.value = false;
    }
  }
  function change(value: string) {
    kind.value = value === 'recommend.source' ? 'recommend.source' : 'discover.source';
    page.value = 1;
    detailOpen.value = false;
    void router.replace({ query: { ...route.query, tab: 'discover', kind: kind.value } });
    void load();
  }
  watch(() => route.query.kind, value => {
    const next = value === 'recommend.source' ? 'recommend.source' : 'discover.source';
    if (next !== kind.value) {
      kind.value = next;
      page.value = 1;
      void load();
    }
  });
  watch(detailOpen, v => {
    if (!v)
      detailEpoch++;
  });
  onMounted(load);
  onUnmounted(() => {
    alive = false;
    epoch++;
    detailEpoch++;
  });
  return { kind, page, items, hasMore, truncated, loading, error, detail, detailOpen, detailLoading, detailError, load, inspect, change };
}
