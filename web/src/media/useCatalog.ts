import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request } from '../api/client';
import { rows, type Home, type MediaItem } from './model';
export function useCatalog() {
  const route = useRoute(), router = useRouter(), source = ref(route.query.source === 'emby' ? 'emby' : 'managed'), view = ref(route.query.view === 'all' ? 'all' : 'home'), page = ref(1), query = ref(''), kind = ref(''), parentId = ref(String(route.query.parentId || '')), user = ref(''), items = ref<MediaItem[]>([]), total = ref<number | null>(null), home = ref<Home | null>(null), loading = ref(true), error = ref<unknown>(), embyConnected = ref<boolean | null>(null), connectionError = ref<unknown>(), detail = ref<MediaItem | null>(null), detailOpen = ref(false), detailLoading = ref(false), detailError = ref<unknown>(), children = ref<MediaItem[]>([]), childTotal = ref(0), childPage = ref(1), childError = ref<unknown>(), childLoading = ref(false), series = ref<MediaItem | null>(null), copyMessage = ref(''), playLink = ref(''), copyBusy = ref(false);
  let epoch = 0, detailEpoch = 0, childEpoch = 0, copyEpoch = 0, alive = true;
  const displayItems = computed(() => items.value);
  const groups = computed(() => [{ key: 'libraries', title: '媒体库分类' }, { key: 'playing', title: '正在播放' }, { key: 'resume', title: '继续观看' }, { key: 'recent', title: '近期观看' }, { key: 'latest', title: '最新入库' }].map(g => ({ ...g, items: rows((home.value as any)?.[g.key]) })));
  const selectedSeason = ref<MediaItem | null>(null);
  const episodes = computed(() => children.value);
  async function chooseSeason(item: MediaItem | null) { selectedSeason.value=item; childPage.value=1; await readChildren(); }
  const isSeries = (item: MediaItem) => !['Episode', 'Season'].includes(item.type || '') && (source.value === 'emby' || item.type === 'Series') && ['series', 'Series'].includes(item.mediaType || item.type || '') && item.episode == null;
  async function connections() {
    try {
      const list = await request<any[]>('/integrations');
      if (alive) {
        embyConnected.value = rows(list).some(x => x.kind === 'emby' && x.enabled);
        connectionError.value = undefined;
      }
    }
    catch (e) {
      if (alive) {
        embyConnected.value = null;
        connectionError.value = e;
      }
    }
  }
  const catalogPage = (parent: string, p: number, child = false) => request<any>(`/media/library?source=${source.value}&q=${encodeURIComponent(child ? '' : query.value)}&mediaType=${child ? '' : kind.value}&page=${p}&pageSize=24&parentId=${encodeURIComponent(parent)}`);
  async function load() {
    const version = ++epoch;
    loading.value = true;
    error.value = undefined;
    items.value = [];
    home.value = null;
    total.value = null;
    try {
      if (source.value === 'emby' && embyConnected.value === false)
        return;
      if (source.value === 'emby' && view.value === 'home') {
        const out = await request<Home>(`/media/library/emby/home?userId=${encodeURIComponent(user.value)}`);
        if (alive && version === epoch) {
          home.value = out;
          user.value = out.userId || '';
        }
      }
      else {
        const out = await catalogPage(parentId.value, page.value);
        if (alive && version === epoch) {
          items.value = rows(out.items);
          total.value = out.total ?? null;
        }
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
  function close() {
    detailOpen.value = false;
    detailEpoch++;
    childEpoch++;
    copyEpoch++;
    children.value = [];
    playLink.value = '';
    copyMessage.value = '';
    copyBusy.value = false;
  }
  function search() {
    page.value = 1;
    void load();
  }
  async function chooseSource(value: string) {
    if (!['managed', 'emby'].includes(value))
      return;
    close();
    source.value = value;
    view.value = 'home';
    parentId.value = '';
    page.value = 1;
    await router.replace({ query: { ...route.query, source: value, view: 'home', parentId: undefined } });
    await load();
  }
  function all() {
    view.value = 'all';
    parentId.value = '';
    page.value = 1;
    void router.replace({ query: { ...route.query, source: source.value, view: 'all', parentId: undefined } });
    void load();
  }
  function browse(id: string) {
    parentId.value = id;
    view.value = 'all';
    page.value = 1;
    void router.replace({ query: { ...route.query, source: source.value, view: 'all', parentId: id || undefined } });
    void load();
  }
  async function readChildren() {
    if (!series.value)
      return;
    const version = ++childEpoch;
    childLoading.value = true;
    childError.value = undefined;
    children.value = [];
    try {
      const out = await catalogPage(selectedSeason.value?.id || series.value.id, childPage.value, true);
      if (alive && version === childEpoch) {
        children.value = rows(out.items);
        childTotal.value = out.total ?? 0;
      }
    }
    catch (e) {
      if (alive && version === childEpoch)
        childError.value = e;
    }
    finally {
      if (alive && version === childEpoch)
        childLoading.value = false;
    }
  }
  async function inspect(item: MediaItem, withinSeries = false) {
    const version = ++detailEpoch;
    copyEpoch++;
    copyBusy.value = false;
    playLink.value = '';
    copyMessage.value = '';
    detail.value = { ...item, source: source.value };
    detailOpen.value = true;
    detailError.value = undefined;
    detailLoading.value = true;
    if (!withinSeries) {
      series.value = isSeries(item) ? item : null;
      children.value = [];
      childEpoch++;
      childPage.value = 1;
      selectedSeason.value = null;
      if (series.value)
        void readChildren();
    }
    try {
      if (source.value === 'emby') {
        const out = await request<MediaItem>(`/media/emby/items/${encodeURIComponent(item.id)}`);
        if (alive && version === detailEpoch)
          detail.value = { ...item, ...out, source: 'emby' };
      }
      if (item.tmdbId) {
        const out = await request<any>(`/metadata/details?mediaType=${item.mediaType}&tmdbId=${item.tmdbId}`);
        if (alive && version === detailEpoch)
          detail.value = { ...detail.value!, overview: out.item?.overview || detail.value?.overview, genres: out.item?.genres || item.genres, episodeDetails: out.item?.episodeDetails || item.episodeDetails, poster: detail.value?.poster || out.item?.posterPath };
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
  function homeItem(group: string, item: MediaItem) {
    if (group === 'libraries')
      browse(item.id);
    else
      void inspect({ ...item, source: 'emby', mediaType: item.type === 'Movie' ? 'movie' : 'series', poster: item.image });
  }
  async function refresh() {
    const checking = connections();
    if (source.value === 'managed')
      await load();
    await checking;
    if (source.value === 'emby')
      await load();
  }
  async function copyLink(path: string) {
    const version = ++copyEpoch;
    copyBusy.value = true;
    copyMessage.value = '';
    playLink.value = '';
    try {
      const response = await fetch(path, { credentials: 'same-origin' });
      if (!response.ok)
        throw Error('暂时无法取得此媒体的播放链接。');
      const text = (await response.text()).trim();
      const url = new URL(text);
      if (!['https:', 'http:'].includes(url.protocol) || /[\r\n]/.test(text))
        throw Error('播放链接格式不可用。');
      if (!alive || version !== copyEpoch)
        return;
      playLink.value = text;
      try {
        if (!navigator.clipboard?.writeText)
          throw Error();
        await navigator.clipboard.writeText(text);
        if (alive && version === copyEpoch)
          copyMessage.value = '播放链接已复制，请勿公开分享。';
      }
      catch {
        if (alive && version === copyEpoch)
          copyMessage.value = '复制失败，请在下方字段手动选择并复制。';
      }
    }
    catch (e) {
      if (alive && version === copyEpoch)
        copyMessage.value = e instanceof Error && /[\u4e00-\u9fff]/.test(e.message) ? e.message : '暂时无法取得播放链接，请稍后重新读取。';
    }
    finally {
      if (alive && version === copyEpoch)
        copyBusy.value = false;
    }
  }
  watch(() => [route.query.source, route.query.view, route.query.parentId], () => {
    const next = route.query.source === 'emby' ? 'emby' : 'managed';
    const nextView = route.query.view === 'all' ? 'all' : 'home', parent = String(route.query.parentId || '');
    if (next !== source.value || nextView !== view.value || parent !== parentId.value) {
      close();
      source.value = next;
      view.value = nextView;
      parentId.value = parent;
      page.value = 1;
      void load();
    }
  });
  watch(detailOpen, v => {
    if (!v)
      close();
  });
  onMounted(refresh);
  onUnmounted(() => {
    alive = false;
    epoch++;
    close();
  });
  return { selectedSeason, chooseSeason, displayItems, source, view, page, query, kind, parentId, user, items, total, home, groups, loading, error, embyConnected, connectionError, detail, detailOpen, detailLoading, detailError, children, childTotal, childPage, childError, childLoading, episodes, series, copyMessage, playLink, copyBusy, load, search, chooseSource, all, browse, inspect, homeItem, refresh, readChildren, copyLink };
}
