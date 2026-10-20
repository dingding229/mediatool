import { onMounted, onUnmounted, reactive, ref } from 'vue';
import { request } from '../api/client';
export function useOrganize() {
  const data = reactive<Record<string, any>>({ candidates: [], assets: [], scans: [], schedules: [], repairs: [] }), errors = reactive<Record<string, unknown>>({}), loading = reactive<Record<string, boolean>>({}), total = reactive({ candidates: 0, assets: 0 }), page = reactive({ candidates: 1, assets: 1 }), filter = ref('needs_confirmation'), assetId = ref(''), selected = reactive<Record<string, any[]>>({ candidates: [], assets: [] }), editing = ref(false);
  let disposed = false, timer: ReturnType<typeof setInterval> | undefined;
  const versions: Record<string, number> = {};
  async function load(key: string) {
    const version = versions[key] = (versions[key] || 0) + 1;
    loading[key] = true;
    errors[key] = null;
    try {
      let out: any;
      if (key === 'candidates')
        out = await request(`/library/candidates?page=${page.candidates}&pageSize=50&state=${filter.value}`);
      else if (key === 'assets')
        out = await request(`/media/assets?page=${page.assets}&pageSize=50${assetId.value ? '&assetId=' + encodeURIComponent(assetId.value) : ''}`);
      else if (key === 'scans')
        out = await request('/library/scans');
      else if (key === 'schedules')
        out = await request('/library/scan-schedules');
      else
        out = await request('/library/artifact-repairs');
      if (disposed || version !== versions[key])
        return;
      if (['candidates', 'assets'].includes(key)) {
        if (selected[key]!.length || editing.value)
          return;
        data[key] = Array.isArray(out) ? out : out.items || [];
        total[key as 'candidates' | 'assets'] = out.total ?? data[key].length;
      }
      else
        data[key] = out || [];
    }
    catch (e) {
      if (!disposed && version === versions[key])
        errors[key] = e;
    }
    finally {
      if (!disposed && version === versions[key])
        loading[key] = false;
    }
  }
  async function refresh() {
    await Promise.all(['candidates', 'assets', 'scans', 'schedules', 'repairs'].map(load));
  }
  function toggle(kind: string, row: any, checked: boolean) {
    selected[kind] = selected[kind]!.filter(r => r.id !== row.id);
    if (checked)
      selected[kind]!.push(row);
  }
  function changed() {
    selected.candidates = [];
    selected.assets = [];
    void refresh();
  }
  onMounted(() => {
    void refresh();
    timer = setInterval(() => {
      if (!editing.value && !selected.candidates!.length && !selected.assets!.length)
        void refresh();
    }, 10000);
  });
  onUnmounted(() => {
    disposed = true;
    if (timer)
      clearInterval(timer);
  });
  return { data, errors, loading, total, page, filter, assetId, selected, editing, load, refresh, toggle, changed };
}
