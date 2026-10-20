import { computed, ref, watch } from 'vue';
import { APIError, post, request } from '../api/client';
import { list, readIntent, sameMedia, writeIntent, type DownloadTask, type Intent, type Media, type Resource, type Run } from './model';
export function useDelivery() {
  const visible = ref(false), resource = ref<Resource | null>(null), identity = ref<Media | null>(null), chosen = ref<Media | null>(null), usingIdentity = ref(false), runId = ref(''), media = ref<Media[]>([]), query = ref(''), kind = ref('auto'), busy = ref(false), loading = ref(false), error = ref<unknown>(null), yearConfirmed = ref(false), yearRequired = ref(false), intent = ref<Intent | null>(null), matches = ref<DownloadTask[]>([]);
  const scope = computed(() => `download:${runId.value}:${resource.value?.id}`), yearWarning = computed(() => resource.value?.metadata?.yearWarning || (yearRequired.value ? '年份不一致，请核对资源与所选媒体身份。' : ''));
  let epoch = 0;
  watch(() => [chosen.value?.id, chosen.value?.mediaType], () => {
    yearConfirmed.value = false;
  });
  watch(() => [query.value, kind.value], () => {
    if (!usingIdentity.value && !intent.value) { epoch++; media.value = []; chosen.value = null; loading.value = false; }
  });
  function close() {
    epoch++;
    busy.value = false;
    loading.value = false;
    visible.value = false;
    chosen.value = null;
    identity.value = null;
    resource.value = null;
    media.value = [];
  }
  function open(run: Run, item: Resource) {
    epoch++;
    runId.value = run.id;
    resource.value = { ...item, metadata: { ...item.metadata } };
    identity.value = run.media ? { ...run.media } : null;
    chosen.value = identity.value ? { ...identity.value } : null;
    usingIdentity.value = !!chosen.value;
    query.value = chosen.value?.title || run.keyword;
    kind.value = chosen.value?.mediaType || 'auto';
    media.value = [];
    yearConfirmed.value = false;
    yearRequired.value = false;
    error.value = null;
    matches.value = [];
    intent.value = readIntent(scope.value);
    visible.value = true;
  }
  async function lookup() {
    if (loading.value)
      return;
    const version = epoch;
    loading.value = true;
    error.value = null;
    chosen.value = null;
    try {
      const out = await request<Media[]>(`/metadata/search?mediaType=${kind.value}&q=${encodeURIComponent(query.value)}`);
      if (version === epoch)
        media.value = list(out);
    }
    catch (e) {
      if (version === epoch)
        error.value = e;
    }
    finally {
      if (version === epoch)
        loading.value = false;
    }
  }
  async function reconcile() {
    error.value = null;
    try {
      matches.value = list(await request<DownloadTask[]>('/downloads')).filter(t => t.resourceId === resource.value?.id);
      if (matches.value.some(t => sameMedia(t.media, chosen.value || identity.value))) {
        intent.value = { phase: 'queued', id: matches.value[0]!.id };
        writeIntent(scope.value, intent.value);
      }
    }
    catch (e) {
      error.value = e;
    }
  }
  async function submit() {
    if (busy.value || intent.value || !resource.value || !chosen.value || (yearWarning.value && !yearConfirmed.value))
      return;
    const key = scope.value, version = epoch, payload = { searchRunId: runId.value, resourceId: resource.value.id, mediaType: chosen.value.mediaType, tmdbId: chosen.value.id, correctMediaIdentity: !!identity.value && !usingIdentity.value, confirmYearMismatch: yearConfirmed.value };
    busy.value = true;
    error.value = null;
    intent.value = { phase: 'reconciling' };
    writeIntent(key, intent.value);
    try {
      const out = await post<{
        id: string;
      }>('/downloads', payload);
      writeIntent(key, { phase: 'queued', id: out.id });
      if (version === epoch)
        intent.value = { phase: 'queued', id: out.id };
    }
    catch (e) {
      if (version === epoch) {
        error.value = e;
        if (e instanceof APIError && e.status >= 400 && e.status < 500) {
          intent.value = null;
          writeIntent(key, null);
          if (e.code === 'media_year_confirmation_required')
            yearRequired.value = true;
        }
      }
    }
    finally {
      if (version === epoch)
        busy.value = false;
    }
  }
  return { visible, resource, identity, chosen, usingIdentity, runId, media, query, kind, busy, loading, error, yearConfirmed, yearWarning, intent, matches, open, close, lookup, reconcile, submit };
}
