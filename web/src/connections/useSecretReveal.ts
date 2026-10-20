import { onUnmounted, reactive, ref } from 'vue'
import { post } from '../api/client'

// Saved values never enter the replacement draft or survive a closed editor.
export function useSecretReveal(source: () => { id: string; revision: number }) {
  const values = reactive<Record<string, string>>({}), busy = reactive<Record<string, boolean>>({})
  const message = ref('')
  let generation = 0
  function hideAll() {
    generation++
    for (const key of Object.keys(values)) delete values[key]
    for (const key of Object.keys(busy)) delete busy[key]
    message.value = ''
  }
  function hide(key: string) { delete values[key] }
  async function show(key: string) {
    if (busy[key] || !source().id) return
    const version = generation, current = { ...source() }
    busy[key] = true; message.value = ''
    try {
      const result = await post<{ values: Record<string, string>; revision: number }>(`/integrations/${current.id}/secrets/reveal`, { revision: current.revision, fields: [key], confirm: true })
      if (version !== generation || current.id !== source().id || current.revision !== source().revision) return
      if (result.revision !== current.revision) throw Error('changed')
      values[key] = result.values[key] ?? ''
    } catch {
      if (version === generation) message.value = '无法显示已保存值，请重新读取配置并打开编辑后再查看。你的替换输入未变。'
    } finally { if (version === generation) busy[key] = false }
  }
  async function copy(key: string) {
    const version = generation
    if (!(key in values)) return
    try {
      if (!navigator.clipboard?.writeText) throw Error('clipboard unavailable')
      await navigator.clipboard.writeText(values[key]!)
      if (version === generation) message.value = '已复制，请妥善保管。'
    } catch { if (version === generation) message.value = '复制失败，请在已显示的字段内手动选择并复制。' }
  }
  onUnmounted(hideAll)
  return { values, busy, message, show, hide, hideAll, copy }
}
