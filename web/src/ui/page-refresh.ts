import { computed, inject, onUnmounted, provide, ref, type InjectionKey } from 'vue';
import { useRoute } from 'vue-router';
type Read = () => unknown | Promise<unknown>;
const key: InjectionKey<ReturnType<typeof createRefreshScope>> = Symbol('page-refresh');
export function createRefreshScope(defaultTab = '') {
  const readers = ref(new Map<Read, () => boolean>()), busy = ref(false), error = ref('');
  const active = computed(() => [...readers.value].filter(([, visible]) => visible()).map(([read]) => read));
  async function refresh() {
    if (busy.value)
      return;
    busy.value = true;
    error.value = '';
    const out = await Promise.allSettled(active.value.map(read => Promise.resolve().then(read)));
    if (out.some(r => r.status === 'rejected'))
      error.value = '部分内容未能刷新，请查看对应提示。';
    busy.value = false;
  }
  function register(read: Read, visible: () => boolean) {
    readers.value = new Map(readers.value).set(read, visible);
    return () => {
      const next = new Map(readers.value);
      next.delete(read);
      readers.value = next;
    };
  }
  return { busy, error, active, refresh, register, defaultTab };
}
export function providePageRefresh(defaultTab = '') {
  const scope = createRefreshScope(defaultTab);
  provide(key, scope);
  return scope;
}
export function usePageRefresh(read: Read, tab?: string | (() => boolean)) {
  const scope = inject(key, null);
  if (scope) {
    const route = useRoute(), initial = route?.query?.tab || scope.defaultTab;
    const remove = scope.register(read, typeof tab === 'function' ? tab : () => tab === '*' || (tab ? (route?.query?.tab || scope.defaultTab) === tab : (route?.query?.tab || scope.defaultTab) === initial));
    onUnmounted(remove);
  }
}
export const useRefreshControl = () => inject(key, null);
