import { onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { post, request } from '../api/client';
import { changedEndpointMissing, isConflict, secretPayload, safeSettings, type Connection } from './model';
export function useConnections() {
  const rows = ref<Connection[]>([]), loading = ref(true), error = ref(''), editing = ref(false), saving = ref(false), notice = ref(''), conflict = ref<Connection | null>(null);
  const loadError = ref<unknown>(null);
  const saved = ref<Connection | null>(null), blocked = ref(false), testState = ref<'queued' | 'reconciling' | null>(null), testId = ref(''), testTime = ref(''), testing = ref('');
  const authOpen = ref(false), authId = ref(''), removeOpen = ref(false), removing = ref(false), removeTarget = ref<Connection | null>(null);
  const form = reactive({ id: '', kind: 'clouddrive2', name: '', enabled: false, settings: {} as Record<string, any>, secrets: {} as Record<string, string>, clear: [] as string[], revision: 0 });
  let disposed = false;
  async function load() {
    loading.value = true;
    loadError.value = null;
    error.value = '';
    try {
      const value = await request<Connection[]>('/integrations');
      if (!disposed)
        rows.value = value;
    }
    catch (e) {
      if (!disposed) {
        loadError.value = e;
        error.value = '连接列表读取失败，请重新读取。';
      }
    }
    finally {
      if (!disposed)
        loading.value = false;
    }
  }
  function create(kind: string) {
    saved.value = null;
    conflict.value = null;
    blocked.value = false;
    error.value = '';
    Object.assign(form, { id: '', kind, name: kind === 'tmdb' ? 'TMDB' : '', enabled: false, settings: kind === 'open115' ? { cloudMountPrefix: '/115' } : kind === 'clouddrive2' ? { cloudRoot: '/115/Media' } : kind === 'tmdb' ? { language: 'zh-CN' } : kind === 'qbittorrent' ? { category: 'media-tool' } : {}, secrets: {}, clear: [], revision: 0 });
    editing.value = true;
  }
  function edit(row: Connection) {
    create(row.kind);
    saved.value = { ...row, settings: { ...row.settings } };
    Object.assign(form, { id: row.id, name: row.name, enabled: row.enabled, settings: safeSettings(row.settings), revision: row.revision });
  }
  async function save(startAuth = false) {
    if (saving.value || blocked.value || conflict.value)
      return;
    const missing = changedEndpointMissing(form, saved.value);
    if (missing.length) {
      error.value = '更换地址前必须重新填写或明确清除全部已设置凭据；不会自动将原密钥发送到新地址。';
      return;
    }
    saving.value = true;
    error.value = '';
    notice.value = '';
    try {
      const out = await request<Connection>(form.id ? `/integrations/${form.id}` : '/integrations', { method: form.id ? 'PUT' : 'POST', body: JSON.stringify({ kind: form.kind, name: form.name, enabled: form.enabled, settings: form.settings, secrets: secretPayload(form.secrets, form.clear), revision: form.revision }) });
      if (disposed)
        return;
      editing.value = false;
      notice.value = out.lastCheckState === 'ready' ? '配置已保存，保存时的连接检测已通过。' : '配置已保存，未完成授权或检测时仍需处理。';
      await load();
      if (startAuth && out.kind === 'open115') {
        authId.value = out.id;
        authOpen.value = true;
      }
    }
    catch (e: any) {
      if (isConflict(e) && form.id) {
        try {
          const fresh = await request<Connection[]>('/integrations');
          if (disposed)
            return;
          rows.value = fresh;
          conflict.value = fresh.find(r => r.id === form.id) ?? null;
          error.value = conflict.value ? '配置已被修改，已重新读取。你的输入已保留，请与当前设置对照后确认。' : '配置已被修改，重新读取后未找到此记录。';
          if (!conflict.value)
            blocked.value = true;
        }
        catch {
          error.value = '配置已被修改，但重新读取失败。你的输入已保留，请稍后重新读取。';
          blocked.value = true;
        }
      }
      else {
        error.value = e?.status >= 400 && e.status < 500 ? (e.code === 'connection_validation_failed' ? '保存前连接检测失败，请核对地址、凭据与网络。' : e.message) : '保存回执未确认，请先重新读取核对，不要重复保存。';
        if (!(e?.status >= 400 && e.status < 500))
          blocked.value = true;
      }
    }
    finally {
      saving.value = false;
    }
  }
  function useRevision() {
    if (!conflict.value)
      return;
    form.revision = conflict.value.revision;
    saved.value = conflict.value;
    conflict.value = null;
    error.value = '已采用最新修订号；你的输入未变，确认后可再次保存。';
  }
  async function test(row: Connection) {
    if (testState.value || testing.value)
      return;
    testing.value = row.id;
    notice.value = '';
    try {
      const out = await post<any>(`/integrations/${row.id}/test`, {});
      if (out?.id || ['pending', 'queued', 'running'].includes(out?.state)) {
        testId.value = row.id;
        testTime.value = row.lastCheckedAt ?? '';
        testState.value = 'queued';
        notice.value = '检测已排队，尚未完成。';
      }
      else
        notice.value = out?.passed ? '连接检测通过。' : '连接检测未通过，请核对配置。';
      await load();
    }
    catch {
      testId.value = row.id;
      testTime.value = row.lastCheckedAt ?? '';
      testState.value = 'reconciling';
      notice.value = '检测回执未知，请先重新读取状态核对。';
    }
    finally {
      testing.value = '';
    }
  }
  async function reconcile() {
    await load();
    const row = rows.value.find(r => r.id === testId.value);
    if (row?.lastCheckedAt && row.lastCheckedAt !== testTime.value) {
      testState.value = null;
      notice.value = '已读取到新的检测记录，请核对连接状态。';
    }
  }
  function authorize(row: Connection) {
    authId.value = row.id;
    authOpen.value = true;
  }
  function askRemove(row: Connection) {
    removeTarget.value = row;
    removeOpen.value = true;
  }
  async function remove() {
    if (!removeTarget.value || removing.value)
      return;
    removing.value = true;
    try {
      await request(`/integrations/${removeTarget.value.id}`, { method: 'DELETE' });
      removeOpen.value = false;
      await load();
    }
    catch {
      error.value = '删除未确认，请重新读取连接列表核对。';
    }
    finally {
      removing.value = false;
    }
  }
  watch(editing, open => {
    if (!open) {
      form.secrets = {};
      form.clear = [];
    }
  });
  onMounted(load);
  onUnmounted(() => {
    disposed = true;
    form.secrets = {};
    form.clear = [];
  });
  return { rows, loading, loadError, error, editing, saving, notice, conflict, saved, blocked, form, load, create, edit, save, useRevision, test, testState, testing, reconcile, authorize, authOpen, authId, askRemove, removeOpen, removeTarget, removing, remove };
}
