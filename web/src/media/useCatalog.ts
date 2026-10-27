import {usePageNumber,usePageSize} from '../ui/page-query';
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { request } from '../api/client';
import {managedCatalog,managedDetail,type Category} from '../api/mediaCatalog';
import { rows, type MediaItem } from './model';
export function useCatalog() {
  const route = useRoute(), router = useRouter(), source = ref('managed'), view = ref(route.query.view === 'all' ? 'all' : 'home'), page = usePageNumber(),pageSize=usePageSize(24), query = ref(''), kind = ref(''), parentId = ref(String(route.query.parentId || '')), items = ref<MediaItem[]>([]), total = ref<number | null>(null), loading = ref(true), error = ref<unknown>(), detail = ref<MediaItem | null>(null), detailOpen = ref(false), detailLoading = ref(false), detailError = ref<unknown>(), children = ref<MediaItem[]>([]), childTotal = ref(0), childPage = usePageNumber('childPage'),childSize=usePageSize(24,'childPage'), childError = ref<unknown>(), childLoading = ref(false), series = ref<MediaItem | null>(null), category=ref(''), categories=ref<Category[]>([]);
  let epoch = 0, detailEpoch = 0, childEpoch = 0, alive = true;
  const displayItems = computed(() => items.value);
  const selectedSeason = ref<MediaItem | null>(null);
  const episodes = computed(() => children.value);
  async function chooseSeason(item: MediaItem | null) { selectedSeason.value=item; childPage.value=1; await readChildren(); }
  const isSeries = (item: MediaItem) => !['Episode', 'Season'].includes(item.type || '') && item.type === 'Series' && ['series', 'Series'].includes(item.mediaType || item.type || '') && item.episode == null;
  const catalogPage = (parent: string, p: number, child = false) => managedCatalog(child?'':query.value,parent,category.value,p,child?childSize.value:pageSize.value);
  let hasLoaded=false;
  async function load() {
    const version = ++epoch;
    loading.value = !hasLoaded;
    error.value = undefined;
    try {
      {
        const out = await catalogPage(parentId.value, page.value);
        if (alive && version === epoch) {
          items.value = rows(out.items);
          total.value = out.total ?? null;
          if(source.value==='managed')categories.value=rows(out.categories);
        }
      }
    hasLoaded=true;
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
    children.value = [];
  }
  function search() {
    page.value = 1;
    void load();
  }
  async function chooseSource(value: string) {
    category.value='';
    if (!['managed'].includes(value))
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
      if(source.value==='managed'){
        const out=await managedDetail(item.id);
        if(alive&&version===detailEpoch)detail.value={...item,...out,source:'managed'};
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
  async function refresh() { await load(); }
  watch(() => [route.query.source, route.query.view, route.query.parentId], () => {
    const next = 'managed';
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
  return { pageSize,childSize,selectedSeason, chooseSeason, displayItems, source, view, page, query, kind, parentId, items, total, loading, error, detail, detailOpen, detailLoading, detailError, children, childTotal, childPage, childError, childLoading, episodes, series, category, categories, load, search, chooseSource, all, browse, inspect, refresh, readChildren };
}
