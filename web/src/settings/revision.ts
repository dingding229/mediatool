import { computed, onUnmounted, ref } from 'vue';
import { APIError, request, responseStatus } from '../api/client';
export type RevisionRecord<T> = {
  value: T;
  revision: number;
  [key: string]: any;
};
export const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value));
export const equal = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
export const stateFor = (e: unknown) => e instanceof APIError && [401, 403].includes(e.status) ? 'unauthorized' : e instanceof APIError && [404, 405, 501].includes(e.status) ? 'unsupported' : 'error';
export const messageFor = (e: unknown) => e instanceof Error ? e.message : '读取失败，请检查连接后重新读取。';
export function useRevision<T extends Record<string, any>>(path: string) {
  const baseline = ref<RevisionRecord<T>>(), draft = ref<T>(), latest = ref<RevisionRecord<T>>(), loading = ref(false), busy = ref(false), error = ref<unknown>(), notice = ref(''), conflict = ref(false);
  let alive = true, version = 0;
  onUnmounted(() => {
    alive = false;
    version++;
  });
  const changed = computed(() => !!draft.value && !!baseline.value && !equal(draft.value, baseline.value.value));
  async function load() {
    const v = ++version;
    loading.value = !baseline.value;
    error.value = undefined;
    try {
      const out = await request<RevisionRecord<T>>(path);
      if (!alive || v !== version)
        return;
      if (changed.value) {
        latest.value = clone(out);
        conflict.value = true;
        notice.value = '配置已重新读取，未保存的输入已保留，请对照。';
      }
      else {
        baseline.value = clone(out);
        draft.value = clone(out.value);
        latest.value = undefined;
        conflict.value = false;
      }
    }
    catch (e) {
      if (alive && v === version)
        error.value = e;
    }
    finally {
      if (alive && v === version)
        loading.value = false;
    }
  }
  function useServer() {
    if (!latest.value)
      return;
    baseline.value = clone(latest.value);
    draft.value = clone(latest.value.value);
    latest.value = undefined;
    conflict.value = false;
    notice.value = '已使用服务器配置。';
  }
  function keepDraft() {
    if (!latest.value)
      return;
    baseline.value = clone(latest.value);
    latest.value = undefined;
    conflict.value = false;
    notice.value = '草稿已保留，请再次核对后保存。';
  }
  async function save(keys: readonly (keyof T)[]) {
    if (busy.value || loading.value || conflict.value || !baseline.value || !draft.value)
      return false;
    const edits = keys.filter(k => !equal(draft.value![k], baseline.value!.value[k]));
    if (!edits.length) {
      notice.value = '当前分区没有改动。';
      return true;
    }
    busy.value = true;
    error.value = undefined;
    notice.value = '';
    const input = clone(draft.value), base = clone(baseline.value);
    let attempted = false;
    try {
      const fresh = await request<RevisionRecord<T>>(path);
      if (!alive)
        return false;
      if (edits.some(k => !equal(fresh.value[k], base.value[k]))) {
        latest.value = clone(fresh);
        conflict.value = true;
        notice.value = '配置已被修改，已重新读取。你的输入已保留，请对照后再保存。';
        return false;
      }
      const value = clone(fresh.value);
      for (const key of edits)
        value[key] = input[key];
      attempted = true;
      const out = await request<RevisionRecord<T>>(path, { method: 'PUT', body: JSON.stringify({ value, revision: fresh.revision }) });
      if (!alive)
        return false;
      if (responseStatus(out) === 202) {
        conflict.value = true;
        notice.value = '保存请求已排队，尚未确认生效。请重新读取并核对配置。';
        return false;
      }
      const remaining = clone(draft.value!);
      const result = clone(out.value);
      for (const key of Object.keys(remaining) as (keyof T)[])
        if (!equal(remaining[key], input[key]))
          result[key] = remaining[key];
      baseline.value = clone(out);
      draft.value = result;
      notice.value = '当前分区已保存。';
      return true;
    }
    catch (e) {
      if (!alive)
        return false;
      error.value = e;
      if (attempted) {
        conflict.value = true;
        notice.value = e instanceof APIError && e.status === 409 ? '配置已被修改，正在重新读取。你的输入已保留，请对照。' : '保存结果尚未确认，请先重新读取核对；不会自动重复保存。';
        try {
          const out = await request<RevisionRecord<T>>(path);
          if (alive) {
            latest.value = clone(out);
            if (e instanceof APIError && e.status === 409)
              notice.value = '配置已被修改，已重新读取。你的输入已保留，请对照。';
          }
        }
        catch {
          if (e instanceof APIError && e.status === 409)
            notice.value = '配置已被修改，暂时无法重新读取。输入已保留，请先重新读取核对。';
        }
      }
      return false;
    }
    finally {
      if (alive)
        busy.value = false;
    }
  }
  return { baseline, draft, latest, changed, loading, busy, error, notice, conflict, load, save, useServer, keepDraft };
}
export type RevisionForm = Omit<ReturnType<typeof useRevision<any>>, 'save'>;
