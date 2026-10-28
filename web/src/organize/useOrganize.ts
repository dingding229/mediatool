import {usePageNumber,usePageSize} from '../ui/page-query';
import { useRefresh } from '../composables/useRefresh';
import { onMounted, onUnmounted, reactive, ref } from 'vue';
import { request } from '../api/client';
export function useOrganize() {
  const sizes=reactive({candidates:usePageSize(50,'candidatesPage'),assets:usePageSize(50,'assetsPage'),scans:usePageSize(8,'scansPage')});
  const data = reactive<Record<string, any>>({ candidates: [], assets: [], scans: [], schedules: [] }), errors = reactive<Record<string, unknown>>({}), loading = reactive<Record<string, boolean>>({}), total = reactive({ candidates: 0, assets: 0,scans:0 }), page = reactive({candidates:usePageNumber('candidatesPage'),assets:usePageNumber('assetsPage'),scans:usePageNumber('scansPage')}), filter = ref('needs_confirmation'), assetId = ref(''), selected = reactive<Record<string, any[]>>({ candidates: [], assets: [] }), editing = ref(false);
  let disposed = false;
  const versions: Record<string, number> = {};
  const ready = new Set<string>();
  async function load(key: string) {
    const version = versions[key] = (versions[key] || 0) + 1;
    loading[key] = !ready.has(key);
    errors[key] = null;
    try {
      let out: any;
      if (key === 'candidates')
        out = await request(`/library/candidates?page=${page.candidates}&pageSize=${sizes.candidates}&state=${filter.value}`);
      else if (key === 'assets')
        out = await request(`/media/assets?page=${page.assets}&pageSize=${sizes.assets}${assetId.value ? '&assetId=' + encodeURIComponent(assetId.value) : ''}`);
      else if (key === 'scans')
        out = await request(`/library/scans?page=${page.scans}&pageSize=${sizes.scans}`);
      else if (key === 'schedules')
        out = await request('/library/scan-schedules');
      else
        return;
      if (disposed || version !== versions[key])
        return;
      if (['candidates', 'assets','scans'].includes(key)) {
        if (selected[key]?.length || editing.value) return;
        data[key] = Array.isArray(out) ? out : out.items || [];
        total[key as 'candidates' | 'assets' | 'scans'] = out.total ?? data[key].length;
      } else data[key] = out || [];
      ready.add(key);
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
    await Promise.all(['candidates', 'assets', 'scans', 'schedules'].map(load));
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
  useRefresh(refresh, { interval: 15000, active: () => !editing.value && !selected.candidates!.length && !selected.assets!.length });
  onUnmounted(() => {
    disposed = true;
  });
  return { sizes,data, errors, loading, total, page, filter, assetId, selected, editing, load, refresh, toggle, changed };
}
