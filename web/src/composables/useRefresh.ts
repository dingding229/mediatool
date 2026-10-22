import { getCurrentInstance, onMounted, onUnmounted, ref } from 'vue'

interface Options { interval: number | (() => number); immediate?: boolean; active?: () => boolean }
/** One in-flight refresh, completion-based cadence, hidden-page pause, no data reset. */
export function useRefresh(read: () => unknown | Promise<unknown>, options: Options) {
  const instance = getCurrentInstance(), refreshing = ref(false)
  let timer: ReturnType<typeof setTimeout> | undefined, flight: Promise<unknown> | undefined, disposed = false, mounted = false
  const visible = () => {
    const root = instance?.subTree.el, element = typeof Element !== 'undefined' && root instanceof Element ? root : root?.parentElement
    if (element) { for(let node=element;node;node=node.parentElement){const style=globalThis.getComputedStyle?.(node);if(node.hidden||style?.display==='none'||style?.visibility==='hidden')return false} }
    return !(typeof document !== 'undefined' && document.hidden) && (options.active?.() ?? true)
  }
  const clear = () => { clearTimeout(timer); timer = undefined }
  function schedule() {
    clear()
    const ms = typeof options.interval === 'function' ? options.interval() : options.interval
    if (!disposed && mounted && !(typeof document !== 'undefined' && document.hidden) && ms > 0) timer = setTimeout(() => { void refresh().catch(() => {}) }, ms)
  }
  function refresh(): Promise<unknown> {
    if (disposed || !visible()) { schedule(); return Promise.resolve() }
    if (flight) return flight
    clear(); refreshing.value = true
    try { flight = Promise.resolve(read()) } catch (e) { flight = Promise.reject(e) }
    flight = flight.finally(() => { flight = undefined; refreshing.value = false; schedule() })
    return flight
  }
  function visibility() { clear(); if (visible()) void refresh().catch(() => {}) }
  onMounted(() => { mounted = true; globalThis.document?.addEventListener('visibilitychange', visibility); if (options.immediate !== false) void refresh().catch(() => {}); else schedule() })
  onUnmounted(() => { disposed = true; clear(); globalThis.document?.removeEventListener('visibilitychange', visibility) })
  return { refresh, refreshing }
}
