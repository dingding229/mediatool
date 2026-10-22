import {usePageNumber} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { APIError, post, request } from '../api/client';
import { list, readIntent, sameMedia, writeIntent, type Account, type Media, type Run } from './model';
import { useDelivery } from './useDelivery';
export function useResults() {
  const route = useRoute(), router = useRouter(), run = ref<Run | null>(null), loading = ref(true), error = ref<unknown>(null), accounts = ref<Account[]>([]), mediaSaving = ref(false), mediaReady = ref(true), page = usePageNumber(), busy = ref(false), searchIntent = ref(readIntent('search-again:' + route.params.id)), delivery = reactive(useDelivery());
  const filters = reactive({ resolution: [] as string[], source: [] as string[], codec: [] as string[], accountId: [] as string[], keyword: '', sort: 'seeders_desc' });
  const activeMedia = computed(() => run.value?.media || null);
  let epoch = 0, controller: AbortController | undefined, filterTimer: ReturnType<typeof setTimeout> | undefined, disposed = false;
  async function refresh(id = String(route.params.id), afterSave = false) {
    if (mediaSaving.value && !afterSave && run.value?.id === id)
      return;
    const version = ++epoch;
    controller?.abort();
    controller = new AbortController();
    loading.value = !run.value;
    error.value = null;
    const p = new URLSearchParams({ page: String(page.value), limit: '50' });
    for (const [key, value] of Object.entries(filters)) {
      if (Array.isArray(value))
        value.forEach(v => p.append(key, v));
      else
        p.set(key, value);
    }
    try {
      const out = await request<Run>(`/search/${id}?${p}`, { signal: controller.signal });
      if (version === epoch && !disposed) {
        run.value = { ...out, sites: list(out.sites), resources: list(out.resources) };
        mediaReady.value = true;
      }
    }
    catch (e) {
      if (version === epoch && !disposed && (e as Error)?.name !== 'AbortError')
        error.value = e;
    }
    finally {
      if (version === epoch && !disposed)
        loading.value = false;
    }
  }
  function applyFilter() {
    clearTimeout(filterTimer);
    page.value = 1;
    void refresh();
  }
  function queueFilter() {
    clearTimeout(filterTimer);
    filterTimer = setTimeout(applyFilter, 250);
  }
  function sort(prop: string) {
    filters.sort = prop + '_' + (filters.sort === prop + '_desc' ? 'asc' : 'desc');
    applyFilter();
  }
  function openDownload(item: any) {
    if (run.value && !mediaSaving.value && mediaReady.value)
      delivery.open(run.value, item);
  }
  async function persistMedia(item: Media) {
    if (!run.value || mediaSaving.value || !mediaReady.value || sameMedia(run.value.media, item))
      return;
    const id = run.value.id;
    epoch++;
    controller?.abort();
    mediaSaving.value = true;
    mediaReady.value = false;
    delivery.close();
    try {
      await post(`/search/${id}/media`, { id: item.id, mediaType: item.mediaType });
      if (run.value?.id === id && !disposed)
        await refresh(id, true);
    }
    catch (e) {
      if (run.value?.id === id && !disposed)
        error.value = e;
    }
    finally {
      if (!disposed)
        mediaSaving.value = false;
    }
  }
  async function searchAgain() {
    if (!run.value || busy.value || searchIntent.value)
      return;
    const key = 'search-again:' + run.value.id;
    busy.value = true;
    error.value = null;
    searchIntent.value = { phase: 'reconciling' };
    writeIntent(key, searchIntent.value);
    try {
      const out = await post<{
        id: string;
      }>('/search', { keyword: run.value.keyword, refresh: true, accountIds: run.value.sites.map(s => s.accountId), media: run.value.media ? { id: run.value.media.id, mediaType: run.value.media.mediaType } : undefined });
      writeIntent(key, { phase: 'queued', id: out.id });
      searchIntent.value = { phase: 'queued', id: out.id };
      await router.push(`/search/results/${out.id}`);
    }
    catch (e) {
      error.value = e;
      if (e instanceof APIError && e.status >= 400 && e.status < 500) {
        searchIntent.value = null;
        writeIntent(key, null);
      }
    }
    finally {
      busy.value = false;
    }
  }
  onMounted(() => {
    void refresh();
    void request<Account[]>('/integrations').then(v => accounts.value = list(v)).catch(() => {
    });

  });
  useRefresh(() => refresh(), {interval:5000, immediate:false, active:()=>!mediaSaving.value && ['pending','running'].includes(run.value?.state || '')});
  watch(() => route.params.id, () => {
    epoch++;
    controller?.abort();
    run.value = null;
    mediaReady.value = true;
    page.value = 1;
    delivery.close();
    searchIntent.value = readIntent('search-again:' + route.params.id);
    void refresh();
  });
  onUnmounted(() => {
    disposed = true;
    epoch++;
    controller?.abort();
    clearTimeout(filterTimer);
  });
  return { run, loading, error, accounts, mediaSaving, mediaReady, page, filters, activeMedia, delivery, busy, searchIntent, refresh, applyFilter, queueFilter, sort, persistMedia, openDownload, searchAgain };
}
