import { onUnmounted, ref } from 'vue';
export function useResource<T = any>(read: () => Promise<T>, initial: T = null as T) {
  const data = ref<T>(initial), loading = ref(false), refreshing = ref(false), error = ref<unknown>(null), loaded = ref(false);
  let version = 0, disposed = false;
  async function load() {
    const current = ++version;
    loading.value = !loaded.value;
    refreshing.value = loaded.value;
    error.value = null;
    try {
      const value = await read();
      if (current === version && !disposed) {
        data.value = value as any;
        loaded.value = true;
        return value;
      }
    }
    catch (e) {
      if (current === version && !disposed)
        error.value = e;
    }
    finally {
      if (current === version && !disposed)
        { loading.value = false; refreshing.value = false; }
    }
  }
  onUnmounted(() => {
    disposed = true;
    version++;
  });
  return { data, loading, refreshing, loaded, error, load };
}
