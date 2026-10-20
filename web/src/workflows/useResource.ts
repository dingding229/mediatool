import { onUnmounted, ref } from 'vue';
export function useResource<T = any>(read: () => Promise<T>, initial: T = null as T) {
  const data = ref<T>(initial), loading = ref(false), error = ref<unknown>(null);
  let version = 0, disposed = false;
  async function load() {
    const current = ++version;
    loading.value = true;
    error.value = null;
    try {
      const value = await read();
      if (current === version && !disposed) {
        data.value = value as any;
        return value;
      }
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
  onUnmounted(() => {
    disposed = true;
    version++;
  });
  return { data, loading, error, load };
}
