import { ref } from 'vue';
import { requestUUID } from '../utils/request-uuid';
import { APIError, responseStatus } from '../api/client';
import { readPrivate, savePrivate } from '../organize/model';
export function useOperation(scope: string) {
  const storage = 'media-tool:u5:' + scope, pending = ref<any>(readPrivate(storage)), phase = ref<'queued' | 'reconciling' | null>(pending.value ? 'reconciling' : null), busy = ref(false), error = ref<unknown>(null);
  function clear() {
    if (readPrivate(storage)?.token === pending.value?.token)
      savePrivate(storage, null);
    pending.value = null;
    phase.value = null;
  }
  async function run(key: string, write: () => Promise<any>) {
    if (busy.value || pending.value)
      return;
    busy.value = true;
    error.value = null;
    pending.value = { key, token: requestUUID() };
    savePrivate(storage, pending.value);
    phase.value = 'reconciling';
    const ticket = { ...pending.value };
    try {
      const out = await write();
      const stored = readPrivate(storage);
      if (pending.value?.token === ticket.token && (!stored || stored.token === ticket.token)) {
        const status = responseStatus(out);
        pending.value = { ...ticket, status };
        savePrivate(storage, pending.value);
        phase.value = status === 202 ? 'queued' : 'reconciling';
      }
      return out;
    }
    catch (e) {
      error.value = e;
      if (e instanceof APIError && e.status >= 400 && e.status < 500)
        clear();
    }
    finally {
      busy.value = false;
    }
  }
  return { pending, phase, busy, error, run, clear };
}
