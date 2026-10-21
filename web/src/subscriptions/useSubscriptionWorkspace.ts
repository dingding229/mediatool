import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import 'element-plus/es/components/message/style/css'
import { APIError, post, request } from '../api/client'
import { asError, draftFor, failure, payloadFor, pendingStorageKey, restorePending } from './model'
import type { Account, Draft, HistoryRun, Media, PendingRun, Run, Subscription } from './model'
export function useSubscriptionWorkspace() {
  const route = useRoute(), router = useRouter()
  const tab = computed(() => route.query.tab === 'history' ? 'history' : 'list')
  const scope = computed(() => route.query.view === 'history' ? 'history' : 'current')
  const records = ref<Subscription[]>([]), accounts = ref<Account[]>([]), loading = ref(true)
  const listError = ref<Error | null>(null), accountError = ref<Error | null>(null), historyError = ref<Error | null>(null)
  const search = ref(''), filter = ref('all'), selected = ref<Subscription | null>(null), detailOpen = ref(false)
  const history = ref<HistoryRun[]>([]), historyLoading = ref(false), run = ref<Run | null>(null), runLoading = ref(false), runError = ref<Error | null>(null)
  const historyId = computed(() => typeof route.query.subscription === 'string' ? route.query.subscription : records.value[0]?.id ?? '')
  const activeRunId = ref(''), runDetailOpen = ref(false), pending = ref<PendingRun | null>(restorePending())
  const editorOpen = ref(false), draft = ref<Draft>(draftFor(null, [])), saving = ref(false), saveError = ref('')
  const previewId = ref(''), previewSignature = ref('')
  const signature = computed(() => { try { return JSON.stringify({ ...payloadFor(draft.value), enabled: false, revision: 0 }) } catch { return '' } })
  const previewRun = computed(() => run.value?.id === previewId.value ? run.value : null)
  const previewError = computed(() => activeRunId.value === previewId.value ? runError.value : null)
  const previewReady = computed(() => !!previewId.value && previewSignature.value === signature.value && !!previewRun.value && !['pending','running','needs_refresh'].includes(previewRun.value.state) && !pending.value && !previewError.value)
  const mutation = ref(''), deleteOpen = ref(false), deleting = ref(false), deleteTarget = ref<Subscription | null>(null)
  const visible = computed(() => records.value.filter(s => (filter.value === 'all' || s.enabled === (filter.value === 'enabled')) && `${s.title} ${s.media.title}`.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
  let disposed = false, listSeq = 0, historySeq = 0, detailSeq = 0, polling = false, timer: ReturnType<typeof setInterval> | undefined
  watch(pending, p => { try { if (p) sessionStorage.setItem(pendingStorageKey, JSON.stringify(p)); else sessionStorage.removeItem(pendingStorageKey) } catch {} }, { deep: true, flush: 'sync' })
  function location(values: Record<string, string | undefined>) { return router.replace({ query: { ...route.query, ...values } }) }
  function changeScope(value: string) { return location({ view: value, subscription: undefined, run: undefined }) }
  async function refresh() {
    const seq = ++listSeq; loading.value = true; listError.value = null
    const [list, sites] = await Promise.allSettled([request<Subscription[]>(`/subscriptions?view=${scope.value}`), request<Account[]>('/integrations')])
    if (disposed || seq !== listSeq) return
    if (list.status === 'fulfilled') records.value = list.value; else listError.value = asError(list.reason)
    if (sites.status === 'fulfilled') { accounts.value = sites.value.filter(a => a.kind === 'ptsite' && a.enabled); accountError.value = null } else accountError.value = asError(sites.reason)
    loading.value = false
    if (!listError.value && tab.value === 'history') await loadHistory()
  }
  function inspect(record: Subscription) { selected.value = record; detailOpen.value = true }
  function compose(record: Subscription | null = null) { previewId.value = ''; previewSignature.value = ''; draft.value = draftFor(record, accounts.value); saveError.value = ''; editorOpen.value = true; detailOpen.value = false }
  async function save(input: Draft) {
    if (saving.value || pending.value || saveError.value.includes('刷新列表')) return
    let payload: ReturnType<typeof payloadFor>
    try { payload = payloadFor(input) } catch (e) { saveError.value = failure(e); return }
    saving.value = true; saveError.value = ''
    try {
      if (input.id) await request(`/subscriptions/${input.id}`, { method: 'PUT', body: JSON.stringify(payload) }); else await post('/subscriptions', payload)
      if (!disposed) { editorOpen.value = false; ElMessage.success(input.enabled ? '订阅已保存并启用' : '订阅已保存，先预演核对再启用'); await refresh() }
    } catch (e) {
      const definite = e instanceof APIError && [400, 401, 403, 404, 409, 422].includes(e.status)
      saveError.value = definite ? failure(e) : '保存回执未确认，请刷新列表核对是否已保存，暂不重复提交。'
    } finally { saving.value = false }
  }
  async function previewDraft(input: Draft) {
    if (saving.value || pending.value || saveError.value.includes('刷新列表')) return
    let payload: ReturnType<typeof payloadFor>
    try { payload = payloadFor(input); payload.enabled = false } catch (e) { saveError.value = failure(e); return }
    saving.value = true; saveError.value = ''; previewId.value = ''; previewSignature.value = ''; activeRunId.value = ''; run.value = null; runError.value = null
    try {
      // The API previews saved revisions only. Explicitly persist a paused draft before evaluating it.
      const record = input.id ? await request<Subscription>(`/subscriptions/${input.id}`, { method: 'PUT', body: JSON.stringify(payload) }) : await post<Subscription>('/subscriptions', payload)
      if (disposed) return
      if (!record?.id || !Number.isInteger(record.revision) || !record.rules || !record.media) throw new Error('保存回执不完整')
      draft.value = { ...input, enabled: false, id: record.id, revision: record.revision }
      previewSignature.value = signature.value
      await evaluate(record, true, false)
      previewId.value = activeRunId.value
    } catch (e) {
      const definite = e instanceof APIError && [400,401,403,404,409,422].includes(e.status)
      saveError.value = definite ? failure(e) : '草稿保存回执未确认，请刷新列表核对，暂不重复提交。'
    } finally { saving.value = false }
  }
  async function reloadEditor() { editorOpen.value = false; await refresh() }
  async function toggle(record: Subscription) {
    if (mutation.value || pending.value) return
    mutation.value = record.id
    try {
      const payload = payloadFor(draftFor(record, accounts.value)); payload.enabled = !record.enabled
      await request(`/subscriptions/${record.id}`, { method: 'PUT', body: JSON.stringify(payload) })
      detailOpen.value = false; await refresh()
    } catch (e) { ElMessage.error(failure(e)) } finally { mutation.value = '' }
  }
  function askDelete(record: Subscription) { deleteTarget.value = record; deleteOpen.value = true }
  async function confirmDelete() {
    if (!deleteTarget.value || deleting.value) return
    deleting.value = true
    try { await request(`/subscriptions/${deleteTarget.value.id}`, { method: 'DELETE' }); deleteOpen.value = false; detailOpen.value = false; await refresh(); ElMessage.success('订阅已归档，下载和文件不受影响') }
    catch (e) { ElMessage.error(failure(e)) } finally { deleting.value = false }
  }
  function showHistory(id: string) { detailOpen.value = false; return location({ tab: 'history', subscription: id, run: undefined }) }
  async function loadHistory() {
    const seq = ++historySeq, id = historyId.value
    history.value = []; historyError.value = null; historyLoading.value = !!id
    if (!id) { run.value = null; activeRunId.value = ''; return }
    try {
      const rows = await request<HistoryRun[]>(`/subscriptions/${id}/history`)
      if (disposed || seq !== historySeq) return
      history.value = rows
      const queryId = typeof route.query.run === 'string' ? route.query.run : ''
      const next = queryId || (pending.value?.subscriptionId === id ? pending.value.runId : '') || rows[0]?.id || ''
      if (next) await readRun(next); else { run.value = null; activeRunId.value = ''; runError.value = null }
    } catch (e) { if (!disposed && seq === historySeq) historyError.value = asError(e) }
    finally { if (!disposed && seq === historySeq) historyLoading.value = false }
  }
  async function readRun(id: string, display = true) {
    const seq = display ? ++detailSeq : detailSeq
    if (display) { activeRunId.value = id; run.value = null; runLoading.value = true; runError.value = null }
    try {
      const result = await request<Run>(`/subscription-runs/${id}`)
      if (disposed) return
      if (seq === detailSeq && activeRunId.value === id) { run.value = result; runError.value = null }
      if (pending.value?.runId === id) {
        if (['pending', 'running'].includes(result.state)) pending.value.status = 'queued'
        else { pending.value = null; await refresh() }
      }
    } catch (e) {
      if (!disposed) {
        if (seq === detailSeq && activeRunId.value === id) runError.value = asError(e)
        if (pending.value?.runId === id) pending.value.status = 'reconciling'
      }
    } finally { if (!disposed && seq === detailSeq && display) runLoading.value = false }
  }
  function chooseRun(id: string) { return location({ tab: 'history', subscription: historyId.value, run: id }) }
  async function reconcile() {
    const task = pending.value
    if (!task || polling || disposed) return
    polling = true
    try {
      if (task.runId) await readRun(task.runId, false)
      else {
        // History can establish facts but cannot prove which run owns a lost response. Never replay POST.
        if (tab.value !== 'history' || historyId.value !== task.subscriptionId) await location({ tab: 'history', subscription: task.subscriptionId, run: undefined })
        else await loadHistory()
      }
    } finally { polling = false }
  }
  async function evaluate(record: Subscription, preview: boolean, navigate = true) {
    if (pending.value) return
    pending.value = { subscriptionId: record.id, runId: '', preview, status: 'locked' }
    try {
      const accepted = await post<{ id: string }>(`/subscriptions/${record.id}/evaluate`, { preview })
      if (disposed) return
      if (!accepted?.id) throw new Error('未收到运行编号')
      pending.value = { subscriptionId: record.id, runId: accepted.id, preview, status: 'queued' }
      if (navigate) await location({ tab: 'history', subscription: record.id, run: accepted.id })
      else await readRun(accepted.id)
    } catch (e) {
      if (disposed) return
      if (e instanceof APIError && [400, 401, 403, 404, 409, 422].includes(e.status)) { pending.value = null; ElMessage.error(failure(e)) }
      else { pending.value.status = 'reconciling'; if (navigate) await reconcile(); else runError.value = asError(e) }
    }
  }
  async function poll() {
    if (pending.value?.runId) await reconcile()
    else if (run.value && ['pending', 'running'].includes(run.value.state)) await readRun(run.value.id, false)
  }
  watch(() => route.query.view, () => { if (!loading.value) void refresh() })
  watch(() => [route.query.tab, route.query.subscription], () => { if (!loading.value && tab.value === 'history') void loadHistory() })
  watch(() => route.query.run, id => { if (typeof id === 'string' && id !== activeRunId.value && tab.value === 'history') void readRun(id) })
  onMounted(async () => {
    await refresh(); if (disposed) return
    if (pending.value) { if (pending.value.runId) activeRunId.value = pending.value.runId; await reconcile() }
    if (disposed) return
    if (route.query.create === '1' && ['movie', 'series'].includes(String(route.query.mediaType)) && /^\d+$/.test(String(route.query.tmdbId))) {
      try {
        const out = await request<{ item: Media }>(`/metadata/details?mediaType=${route.query.mediaType}&tmdbId=${route.query.tmdbId}`)
        if (disposed) return
        compose(); draft.value.media = out.item; draft.value.keyword = out.item.title
        const { create: _create, mediaType: _mediaType, tmdbId: _tmdbId, ...query } = route.query
        await router.replace({ query })
      } catch (e) { listError.value = asError(e) }
    }
    if (!disposed) timer = setInterval(poll, 3000)
  })
  onUnmounted(() => { disposed = true; if (timer) clearInterval(timer) })
  return { tab, scope, records, accounts, loading, listError, accountError, search, filter, visible, selected, detailOpen, inspect,
    history, historyLoading, historyError, historyId, activeRunId, run, runLoading, runError, runDetailOpen, readRun, chooseRun, showHistory, loadHistory,
    pending, evaluate, reconcile, refresh, changeScope, compose, draft, editorOpen, saving, saveError, save, reloadEditor,
    previewDraft, previewReady, previewRun, previewError, toggle, mutation, askDelete, deleteOpen, deleteTarget, deleting, confirmDelete }
}
