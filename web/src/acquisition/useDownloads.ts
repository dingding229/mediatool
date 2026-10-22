import {useRoute} from 'vue-router';
import { useRefresh } from '../composables/useRefresh';
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import { APIError, post, request } from '../api/client';
import { list, readIntent, writeIntent, type DownloadTask, type Intent } from './model';
export interface Transfer {
  id: string;
  destination: string;
  state: string;
  phase?: string;
  flow?: {
    retryable: boolean;
    reason: string;
  };
  bytes?: number;
  hashBytes?: number;
  total?: number;
  processingProgress?: number;
  processingStage?: string;
  transferMethod?: string;
  controlRevision?: number;
  updatedAt?: string;
  message?: string;
}
export function useDownloads() {
  const route=useRoute();
  const tasks = ref<DownloadTask[]>([]), transfers = ref<Transfer[]>([]), batches = ref<any[]>([]), loading = reactive({ tasks: true, transfers: true, batches: true }), errors = reactive<Record<string, unknown>>({}), pending = reactive<Record<string, Intent>>({}), notice = ref<Intent | null>(null), message = ref(''), busy = ref(false), confirmOpen = ref(false), confirmation = ref<any>(null), sessions = ref<any[]>([]), sessionBusy = ref(false), sessionError = ref<unknown>(null), binding = ref(false), sessionId = ref(''), sessionTarget = ref<Transfer | null>(null);
  let disposed = false;
  const epochs: Record<string, number> = {};
  const ready = new Set<string>();
  const key = (kind: string, id: string, action: string) => `${kind}:${id}:${action}`;
  function locked(kind: string, id: string, action: string) {
    return pending[key(kind, id, action)] || readIntent(key(kind, id, action));
  }
  async function load(kind: 'tasks' | 'transfers' | 'batches') {
    const version = epochs[kind] = (epochs[kind] || 0) + 1;
    loading[kind] = !ready.has(kind);
    try {
      const out: any = kind === 'tasks' ? await request<DownloadTask[]>('/downloads') : kind === 'transfers' ? await request<Transfer[]>('/transfers') : await request('/library/activity?state=needs_reconcile&page=1&pageSize=25');
      if (disposed || version !== epochs[kind])
        return;
      const rows: any[] = kind === 'batches' ? list<any>(out.items) : list<any>(out);
      if (kind === 'tasks')
        tasks.value = rows;
      else if (kind === 'transfers')
        transfers.value = rows;
      else
        batches.value = rows;
      errors[kind] = null;
      ready.add(kind);
      for (const row of rows)
        for (const action of ['pause', 'resume', 'cancel', 'bind', 'reconcile', 'retry', 'cleanup']) {
          const scope = key(kind, row.id, action), saved = readIntent(scope);
          if (!saved)
            continue;
          pending[scope] = saved;
          const done = action === 'pause' || action === 'bind' ? row.state === 'paused' : action === 'resume' ? ['uploading', 'verified', 'imported'].includes(row.state) : action === 'cancel' ? row.state === 'cancelled' : action === 'cleanup' ? ['succeeded', 'blocked'].includes(row.cleanupState) : action === 'retry' ? row.flow?.retryable === false : action === 'reconcile' ? ['verified', 'imported'].includes(row.state) : false;
          if (done) {
            writeIntent(scope, null);
            delete pending[scope];
          }
        }
      if (!Object.keys(pending).length)
        notice.value = null;
    }
    catch (e) {
      if (!disposed && version === epochs[kind])
        errors[kind] = e;
    }
    finally {
      if (!disposed && version === epochs[kind])
        loading[kind] = false;
    }
  }
  async function refresh(sync = false) {
    let syncError: unknown;
    if (sync) {
      try {
        await post('/downloads/sync', {});
        message.value = '已请求核对下载器进度，没有重新投递种子。';
      }
      catch (e) {
        syncError = e;
      }
    }
    await Promise.all([load('tasks'), load('transfers'), load('batches')]);
    if (syncError)
      errors.tasks = syncError;
  }
  function ask(row: DownloadTask | Transfer, kind: string, action: string) {
    if (busy.value)
      return;
    if (locked(kind, row.id, action)) {
      notice.value = locked(kind, row.id, action) || null;
      message.value = '上一操作结果待核对，不重复提交。';
      void load(kind === 'tasks' ? 'tasks' : 'transfers');
      return;
    }
    confirmation.value = { row, kind, action };
    confirmOpen.value = true;
  }
  async function execute() {
    const c = confirmation.value;
    if (!c || busy.value)
      return;
    if (c.action === 'retry' && (!c.row.flow?.retryable || c.row.sourceRecoveryRequired))
      return;
    const scope = key(c.kind, c.row.id, c.action);
    if (locked(c.kind, c.row.id, c.action))
      return;
    busy.value = true;
    pending[scope] = { phase: 'reconciling' };
    writeIntent(scope, pending[scope]!);
    message.value = '';
    try {
      if (c.action === 'cleanup')
        await post(`/downloads/${c.row.id}/cleanup`, { confirm: true, hrSatisfied: true });
      else if (c.action === 'retry')
        await post(`/pipeline/${c.kind === 'tasks' ? 'downloads' : 'transfers'}/${c.row.id}/retry`, {});
      else
        await post(`/transfers/${c.row.id}/control`, { action: c.action === 'restart' ? 'reconcile' : c.action, allowRestartIfAbsent: c.action === 'restart', ...(c.action === 'bind' ? { sessionId: sessionId.value, confirmSessionBinding: true } : {}) });
      pending[scope] = { phase: 'queued' };
      writeIntent(scope, pending[scope]!);
      notice.value = pending[scope]!;
      message.value = '操作已排队或受理，尚未确认后续执行完成。';
      confirmOpen.value = false;
      binding.value = false;
      await refresh();
    }
    catch (e) {
      confirmOpen.value = false;
      binding.value = false;
      errors[c.kind] = e;
      notice.value = { phase: 'reconciling' };
      message.value = '操作回执未确认，请先重新读取核对，不重复提交。';
      if (e instanceof APIError && e.status >= 400 && e.status < 500) {
        delete pending[scope];
        writeIntent(scope, null);
        notice.value = null;
        message.value = '操作未受理，请核对提示后再决定。';
      }
    }
    finally {
      busy.value = false;
    }
  }
  async function inspectTransfer(row: Transfer) {
    if (busy.value)
      return;
    busy.value = true;
    try {
      await post(`/transfers/${row.id}/control`, { action: 'reconcile', allowRestartIfAbsent: false });
      message.value = '远端会话与文件已核对；没有批准重建。';
      await load('transfers');
    }
    catch (e) {
      errors.transfers = e;
      if (e instanceof APIError && e.code === 'transfer_control_failed' && e.message.includes('explicitly confirm a restart')) {
        confirmation.value = { kind: 'transfers', row, action: 'restart' };
        confirmOpen.value = true;
      }
    }
    finally {
      busy.value = false;
    }
  }
  async function reconcileCleanup(row: DownloadTask) {
    if (busy.value)
      return;
    busy.value = true;
    try {
      await post(`/downloads/${row.id}/cleanup/reconcile`, {});
      message.value = '清理结果已只读核对，没有重新执行删除。';
      await load('tasks');
    }
    catch (e) {
      errors.tasks = e;
    }
    finally {
      busy.value = false;
    }
  }
  async function bind(row: Transfer) {
    const version = epochs.sessions = (epochs.sessions || 0) + 1;
    sessionTarget.value = row;
    sessionId.value = '';
    binding.value = true;
    sessions.value = [];
    sessionBusy.value = true;
    sessionError.value = null;
    try {
      const out = await request<any[]>(`/transfers/${row.id}/sessions`);
      if (!disposed && version === epochs.sessions) sessions.value = list(out);
    }
    catch (e) {
      if (!disposed && version === epochs.sessions) sessionError.value = e;
    }
    finally {
      if (!disposed && version === epochs.sessions) sessionBusy.value = false;
    }
  }
  function askBinding() {
    if (sessionTarget.value && sessions.value.some(s => s.id === sessionId.value))
      ask(sessionTarget.value, 'transfers', 'bind');
  }
  useRefresh(async()=>{const tab=route?.query?.tab||'tasks';if(tab==='uploads')await Promise.all([load('transfers'),load('batches')]);else await load('tasks')}, { interval:15000, immediate:false, active:()=>!busy.value&&route?.query?.tab!=='manual' });
  onMounted(()=>void refresh());
  onUnmounted(() => {
    disposed = true;
  });
  return { tasks, transfers, batches, loading, errors, pending, notice, message, busy, confirmOpen, confirmation, sessions, sessionBusy, sessionError, binding, sessionId, load, refresh, locked, ask, execute, inspectTransfer, reconcileCleanup, bind, askBinding };
}
