import {usePageNumber} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { onMounted, onUnmounted, ref } from 'vue';
import { APIError, post, request,checkedPage,type ListPage } from '../api/client';
import { list, readIntent, writeIntent, type Intent } from './model';
export interface ManualTask {
  id: string;
  processingState?:string;
  managedTaskId?:string;
  title: string;
  state: string;
  submitAttempted?: boolean;
  sourceKind?: string;
  source?: string;
  infoHash?: string;
  progress: number | null;
  qbState?: string;
  downloadSpeed?: number;
  savePath?: string;
  message?: string;
  syncedAt?: string;
}
export function useManual() {
  const page=usePageNumber('manualPage'),total=ref(0);
  const tasks = ref<ManualTask[]>([]), dialog = ref(false), kind = ref('file'), url = ref(''), file = ref<File>(), busy = ref(false), loading = ref(true), error = ref<unknown>(null), listError = ref<unknown>(null), intent = ref<Intent | null>(readIntent('manual:create')), confirmOpen = ref(false), target = ref<ManualTask | null>(null), actionBusy = ref(''), observed = ref('');
  let disposed = false, epoch = 0, ready = false;
  async function load() {
    const version = ++epoch;
    loading.value = !ready;
    try {
      const out = await request<ListPage<ManualTask>>(`/downloads/manual?page=${page.value}&pageSize=25`).then(checkedPage<ManualTask>);
      if (!disposed && version === epoch) {
        tasks.value = list(out.items);total.value=out.total;
        ready = true;
        for (const task of tasks.value) {
          const saved = readIntent('manual-retry:' + task.id);
          if (saved?.phase === 'queued' && (['blocked', 'failed'].includes(task.state) && !task.submitAttempted || task.state === 'absent_confirmed'))
            writeIntent('manual-retry:' + task.id, null);
        }
        listError.value = null;
      }
    }
    catch (e) {
      if (!disposed && version === epoch)
        listError.value = e;
    }
    finally {
      if (!disposed && version === epoch)
        loading.value = false;
    }
  }
  async function sync() {
    let syncError: unknown;
    try {
      await request('/downloads/sync', { method: 'POST', body: '{}' });
    }
    catch (e) {
      syncError = e;
    }
    await load();
    if (syncError)
      listError.value = syncError;
  }
  function create() {
    if (intent.value?.phase === 'queued') {
      intent.value = null;
      writeIntent('manual:create', null);
    }
    dialog.value = true;
    error.value = null;
    kind.value = 'file';
    url.value = '';
    file.value = undefined;
  }
  async function submit() {
    if (busy.value || intent.value)
      return;
    if (kind.value === 'file' && (!file.value || !file.value.name.toLowerCase().endsWith('.torrent') || file.value.size > 10 * 1024 * 1024)) {
      error.value = Error('请选择不超过10MiB的.torrent文件');
      return;
    }
    if (kind.value === 'link' && !url.value.trim()) {
      error.value = Error('请填写种子链接或磁力链接');
      return;
    }
    busy.value = true;
    error.value = null;
    intent.value = { phase: 'reconciling' };
    writeIntent('manual:create', intent.value);
    try {
      let body: BodyInit;
      if (kind.value === 'file') {
        const form = new FormData();
        form.append('torrent', file.value!);
        body = form;
      }
      else
        body = JSON.stringify({ url: url.value.trim() });
      const out = await request<{
        id: string;
      }>('/downloads/manual', { method: 'POST', body });
      intent.value = { phase: 'queued', id: out.id };
      writeIntent('manual:create', intent.value);
      dialog.value = false;
      url.value = '';
      file.value = undefined;
      await load();
    }
    catch (e) {
      error.value = e;
      if (e instanceof APIError && e.status >= 400 && e.status < 500) {
        intent.value = null;
        writeIntent('manual:create', null);
      }
    }
    finally {
      busy.value = false;
    }
  }
  async function reconcile(task: ManualTask) {
    if (actionBusy.value)
      return;
    actionBusy.value = task.id;
    try {
      await post(`/downloads/manual/${task.id}/inspect`, {});
      writeIntent('manual-retry:' + task.id, null);
      await load();
    }
    catch (e) {
      listError.value = e;
    }
    finally {
      actionBusy.value = '';
    }
  }
  function retry(task: ManualTask) {
    if (!['blocked', 'failed', 'absent_confirmed'].includes(task.state) || (task.submitAttempted && task.state !== 'absent_confirmed') || readIntent('manual-retry:' + task.id))
      return;
    target.value = task;
    confirmOpen.value = true;
  }
  async function approveRetry() {
    const task = target.value;
    if (!task || actionBusy.value || readIntent('manual-retry:' + task.id))
      return;
    actionBusy.value = task.id;
    writeIntent('manual-retry:' + task.id, { phase: 'reconciling' });
    try {
      await post(`/downloads/manual/${task.id}/retry`, { confirm: true, acceptDuplicateRisk: task.state === 'absent_confirmed' });
      writeIntent('manual-retry:' + task.id, { phase: 'queued' });
      confirmOpen.value = false;
      await load();
    }
    catch (e) {
      confirmOpen.value = false;
      listError.value = e;
      if (e instanceof APIError && e.status >= 400 && e.status < 500)
        writeIntent('manual-retry:' + task.id, null);
    }
    finally {
      actionBusy.value = '';
    }
  }
  function associate() {
    if (tasks.value.some(t => t.id === observed.value)) {
      intent.value = { phase: 'queued', id: observed.value };
      writeIntent('manual:create', intent.value);
      dialog.value = false;
    }
  }
  useRefresh(load, { interval: 15000, active: () => !busy.value });
  onUnmounted(() => {
    disposed = true;
    epoch++;
    file.value = undefined;
    url.value = '';
  });
  return { page,total,tasks, dialog, kind, url, file, busy, loading, error, listError, intent, confirmOpen, target, actionBusy, observed, load, sync, create, submit, reconcile, retry, approveRetry, associate };
}
