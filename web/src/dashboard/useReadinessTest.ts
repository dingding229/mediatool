import { computed, onUnmounted, ref } from 'vue';
import { APIError, responseStatus } from '../api/client';
import {checkConnection} from '../api/connections';
import { readPrivate, savePrivate } from '../organize/model';
import { list } from './model';
export type ReadinessTestKind = 'open115' | 'clouddrive2' | 'qbittorrent';
export function useReadinessTest(accounts: () => any, reload: () => Promise<unknown>, kind: ReadinessTestKind = 'open115') {
  // Keep the existing 115 receipt key; each other connection reconciles independently.
  const key = 'media-tool:readiness-test' + (kind === 'open115' ? '' : ':' + kind), pending = ref<any>(readPrivate(key)), busy = ref(false), phase = ref<'queued' | 'reconciling' | null>(pending.value ? 'reconciling' : null), message = ref(pending.value ? '上次检测结果未确认，请先核对，不重复检测。' : '');
  const connection = computed(() => {
    const rows = list(accounts()).filter(r => r.kind === kind && r.enabled);
    return rows.length === 1 ? rows[0] : null;
  }), available = computed(() => !!connection.value);
  let alive = true;
  onUnmounted(() => {
    alive = false;
  });
  function clear() {
    pending.value = null;
    phase.value = null;
    savePrivate(key, null);
  }
  async function start() {
    const row = connection.value;
    if (!row || busy.value || pending.value)
      return;
    busy.value = true;
    pending.value = { id: row.id, revision: row.revision, checkedAt: row.lastCheckedAt || '' };
    savePrivate(key, pending.value);
    phase.value = 'reconciling';
    message.value = '正在检测，尚未确认结果。';
    try {
      const out = await checkConnection(row.id);
      if (!alive)
        return;
      if (responseStatus(out) === 202 || 'id' in out || ['pending', 'queued', 'running'].includes(out?.state)) {
        phase.value = 'queued';
        message.value = '检测已排队，尚未完成，请核对结果。';
      }
      else if (typeof out?.passed === 'boolean') {
        clear();
        message.value = out.reused ? '冷却期内复用上次检测结果，检测时间不变。' : out.passed ? '连接检测通过，不代表播放器已经试播成功。' : '连接检测未通过，请到连接页检查配置。';
        await reload();
      }
      else
        message.value = '检测回执不明确，请先核对，不重复检测。';
    }
    catch (e) {
      if (!alive)
        return;
      if (e instanceof APIError && e.status >= 400 && e.status < 500 && ![408, 499].includes(e.status) && e.code !== 'invalid_response') {
        clear();
        message.value = e.status===409?'配置已被修改，请刷新后再检测。':'检测请求未被接受，请到连接页检查权限和配置。';
      }
      else
        message.value = '检测结果未知，请先核对，不重复检测。';
    }
    finally {
      if (alive)
        busy.value = false;
    }
  }
  async function reconcile() {
    if (busy.value)
      return;
    busy.value = true;
    try {
      await reload();
      if (!alive || !pending.value)
        return;
      const row = list(accounts()).find(r => r.id === pending.value.id);
      if (row?.revision === pending.value.revision && row.lastCheckedAt && row.lastCheckedAt !== pending.value.checkedAt) {
        clear();
        message.value = '已读取新的检测记录，请查看连接状态。';
      }
      else
        message.value = '尚未取得新的检测记录，请到连接页核对；不会重复检测。';
    }
    finally {
      if (alive)
        busy.value = false;
    }
  }
  return { available, busy, phase, message, start, reconcile };
}
