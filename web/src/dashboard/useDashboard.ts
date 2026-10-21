import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { request } from '../api/client';
import { guideSteps, inboxGroups, type Slot } from './model';
import { readGuideHidden, writeGuideHidden } from '../workflows/readiness';
const endpoints: Record<string, string> = { readiness: '/readiness', runtime: '/system/runtime', recent: '/search', jobs: '/jobs/browse?state=failed&page=1&pageSize=100', candidates: '/library/candidates?state=needs_confirmation&page=1&pageSize=5', transfers: '/transfers', manual: '/downloads/manual', batches: '/library/activity?state=needs_reconcile&page=1&pageSize=5', alerts: '/ops', plugins: '/plugins?page=1&pageSize=200', accounts: '/integrations', classification: '/classification' };
export function useDashboard() {
  const slots = reactive<Record<string, Slot>>(Object.fromEntries(Object.keys(endpoints).map(k => [k, { data: null, loading: true, error: null }])));
  const hidden = ref(readGuideHidden());
  const groups = computed(() => inboxGroups(slots)), steps = computed(() => guideSteps(slots.readiness?.data, slots.accounts?.data, slots.classification?.data));
  const complete = computed(() => steps.value.every(s => s.done)), guideVisible = computed(() => !complete.value || !hidden.value);
  let disposed = false;
  async function load(key: string) {
    const slot = slots[key]!;
    if (!slot)
      return;
    slot.loading = true;
    slot.error = null;
    try {
      const data = await request(endpoints[key]!);
      if (!disposed)
        slot.data = data;
    }
    catch (e) {
      if (!disposed) {
        slot.data = null;
        slot.error = e;
      }
    }
    finally {
      if (!disposed)
        slot.loading = false;
    }
  }
  async function refresh() {
    await Promise.all(Object.keys(endpoints).map(load));
  }
  function hideGuide() {
    if (complete.value) {
      hidden.value = true;
      writeGuideHidden(true);
    }
  }
  function reopen() {
    hidden.value = false;
    writeGuideHidden(false);
  }
  onMounted(() => {
    void refresh();
    window.addEventListener('media-tool:show-setup', reopen);
  });
  onUnmounted(() => {
    disposed = true;
    window.removeEventListener('media-tool:show-setup', reopen);
  });
  return { slots, groups, steps, complete, guideVisible, refresh, load, hideGuide };
}
