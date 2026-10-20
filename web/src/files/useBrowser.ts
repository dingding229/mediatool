import { computed, onMounted, onUnmounted, ref } from 'vue';
import { request } from '../api/client';
import type { FileRoot, FileEntry, FileResult, FileCapabilities } from './types';
export function useBrowser() {
  const roots = ref<FileRoot[]>([]), connection = ref(''), directory = ref('.'), page = ref(1), total = ref(0), entries = ref<FileEntry[]>([]), filter = ref(''), selection = ref<FileEntry[]>([]), capability = ref<FileCapabilities | null>(null), loading = ref(true), rootsLoading = ref(true), error = ref<unknown>(null), rootError = ref<unknown>(null), history = ref<FileResult[]>([]), historyLoading = ref(false), historyError = ref<unknown>(null);
  const root = computed(() => roots.value.find(r => r.id === connection.value)), shown = computed(() => entries.value.filter(f => f.name.toLocaleLowerCase().includes(filter.value.toLocaleLowerCase()))), crumbs = computed(() => [{ name: '根目录', path: '.' }, ...(directory.value === '.' ? [] : directory.value.split('/').map((name, index, all) => ({ name, path: all.slice(0, index + 1).join('/') })))]);
  let version = 0, historyVersion = 0, rootVersion = 0, disposed = false;
  async function load() {
    const current = ++version;
    if (!connection.value) {
      entries.value = [];
      loading.value = false;
      return;
    }
    loading.value = true;
    error.value = null;
    try {
      const out: any = await request(`/files?connectionId=${encodeURIComponent(connection.value)}&path=${encodeURIComponent(directory.value)}&page=${page.value}&pageSize=50`);
      if (current !== version || disposed)
        return;
      if (selection.value.length)
        return;
      entries.value = out.items;
      total.value = out.total;
      capability.value = out.capabilities || root.value?.capabilities || null;
    }
    catch (e) {
      if (current === version && !disposed)
        error.value = e;
    }
    finally {
      if (current === version && !disposed)
        loading.value = false;
    }
  }
  async function loadRoots() {
    const current = ++rootVersion;
    rootsLoading.value = true;
    rootError.value = null;
    try {
      const out = await request<FileRoot[]>('/files/roots');
      if (current !== rootVersion || disposed)
        return;
      roots.value = out;
      if (!out.some(r => r.id === connection.value)) {
        connection.value = out[0]?.id || '';
        directory.value = '.';
        page.value = 1;
        selection.value = [];
        capability.value = null;
      }
      await load();
    }
    catch (e) {
      if (current === rootVersion && !disposed)
        rootError.value = e;
    }
    finally {
      if (current === rootVersion && !disposed) {
        rootsLoading.value = false;
        loading.value = false;
      }
    }
  }
  async function loadHistory() {
    const current = ++historyVersion;
    historyLoading.value = true;
    historyError.value = null;
    try {
      const out = await request<FileResult[]>('/files/operations');
      if (current === historyVersion && !disposed)
        history.value = out;
    }
    catch (e) {
      if (current === historyVersion && !disposed)
        historyError.value = e;
    }
    finally {
      if (current === historyVersion && !disposed)
        historyLoading.value = false;
    }
  }
  function navigate(path = '.') {
    version++;
    directory.value = path;
    page.value = 1;
    selection.value = [];
    entries.value = [];
    capability.value = null;
    filter.value = '';
    void load();
  }
  function changeRoot() {
    navigate('.');
  }
  function select(row: FileEntry, checked: boolean) {
    selection.value = selection.value.filter(f => f.path !== row.path);
    if (checked && selection.value.length < 20)
      selection.value.push(row);
  }
  function refresh() {
    selection.value = [];
    return loadRoots();
  }
  onMounted(loadRoots);
  onUnmounted(() => {
    disposed = true;
    version++;
    historyVersion++;
    rootVersion++;
  });
  return { roots, connection, directory, page, total, entries, filter, selection, capability, loading, rootsLoading, error, rootError, root, shown, crumbs, history, historyLoading, historyError, load, loadRoots, loadHistory, navigate, changeRoot, select, refresh };
}
export function permission(operation: string, row?: FileEntry, cap?: FileCapabilities | null) {
  const c = row?.capabilities || cap;
  return !!c && !c.readOnly && !row?.unsupported && c.write === true && (operation === 'mkdir' || (operation === 'trash' ? c.recycle === true : c.move === true));
}
