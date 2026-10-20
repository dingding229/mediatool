import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { post, request, APIError } from '../api/client';
import { list, readIntent, writeIntent, type Account, type Media, type Run, type Intent } from './model';
export function useSearch() {
  const router = useRouter(), mode = ref('media'), query = ref(''), kind = ref('auto'), year = ref(''), keyword = ref(''), selected = ref<Media | null>(null), media = ref<Media[]>([]), history = ref<Run[]>([]), sites = ref<Account[]>([]), accountIds = ref<string[]>([]);
  const loading = ref(true), searching = ref(false), busy = ref(false), searched = ref(false), cached = ref(false), error = ref<unknown>(null), accountsError = ref<unknown>(null), historyError = ref<unknown>(null), intent = ref<Intent | null>(null);
  const ready = computed(() => sites.value.length > 0), text = computed(() => mode.value === 'site' ? keyword.value.trim() : selected.value?.title || ''), scope = computed(() => JSON.stringify([text.value, mode.value === 'media' ? selected.value?.id : 0, mode.value === 'media' ? selected.value?.mediaType : '', accountIds.value]));
  let epoch = 0, disposed = false, lastScope = '';
  watch(() => [query.value, kind.value, year.value], () => {
    epoch++;
    searched.value = false;
    selected.value = null;
    media.value = [];
    searching.value = false;
  });
  async function load() {
    await Promise.all([
      request<Account[]>('/integrations').then(v => {
        if (!disposed) {
          sites.value = list(v).filter(a => a.kind === 'ptsite' && a.enabled);
          accountsError.value = null;
        }
      }).catch(e => {
        if (!disposed)
          accountsError.value = e;
      }),
      request<Run[]>('/search').then(v => {
        if (!disposed) {
          history.value = list(v);
          historyError.value = null;
        }
      }).catch(e => {
        if (!disposed)
          historyError.value = e;
      })
    ]);
    if (!disposed)
      loading.value = false;
  }
  async function findMedia(refresh = false) {
    if (!query.value.trim() || searching.value)
      return;
    const version = ++epoch;
    searching.value = true;
    error.value = null;
    selected.value = null;
    searched.value = false;
    try {
      const p = new URLSearchParams({ q: query.value.trim(), mediaType: kind.value, year: year.value, refresh: String(refresh) });
      const out = await request<{
        items: Media[];
        cached: boolean;
      }>(`/search/media?${p}`);
      if (version === epoch && !disposed) {
        media.value = list(out.items);
        cached.value = out.cached;
        searched.value = true;
      }
    }
    catch (e) {
      if (version === epoch && !disposed)
        error.value = e;
    }
    finally {
      if (version === epoch && !disposed)
        searching.value = false;
    }
  }
  async function reconcile() {
    await load();
    if (intent.value?.id) {
      try {
        const out = await request<Run>(`/search/${intent.value.id}?page=1&limit=50`);
        if (!['pending', 'running'].includes(out.state)) {
          writeIntent('search:' + lastScope, null);
          intent.value = null;
        }
      }
      catch (e) {
        error.value = e;
      }
    }
  }
  async function start(refresh = false) {
    if (!text.value || busy.value || !ready.value)
      return;
    lastScope = scope.value;
    intent.value = readIntent('search:' + lastScope);
    if (intent.value) {
      await reconcile();
      return;
    }
    busy.value = true;
    error.value = null;
    intent.value = { phase: 'reconciling' };
    writeIntent('search:' + lastScope, intent.value);
    try {
      const out = await post<{
        id: string;
        cached?: boolean;
      }>('/search', { keyword: text.value, refresh, accountIds: accountIds.value.length ? accountIds.value : undefined, media: mode.value === 'media' && selected.value ? { id: selected.value.id, mediaType: selected.value.mediaType } : undefined });
      intent.value = { phase: 'queued', id: out.id };
      writeIntent('search:' + lastScope, intent.value);
      await router.push(`/search/results/${out.id}`);
    }
    catch (e) {
      error.value = e;
      if (e instanceof APIError && e.status >= 400 && e.status < 500) {
        intent.value = null;
        writeIntent('search:' + lastScope, null);
      }
    }
    finally {
      busy.value = false;
    }
  }
  onMounted(load);
  onUnmounted(() => {
    disposed = true;
    epoch++;
  });
  return { loading, mode, query, kind, year, keyword, selected, media, history, sites, accountIds, searching, busy, searched, cached, error, accountsError, historyError, ready, intent, load, findMedia, start, reconcile };
}
