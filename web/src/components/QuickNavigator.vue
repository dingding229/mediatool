<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, ArrowRight } from '@element-plus/icons-vue'
import { searchNavigation, type QuickSearchMatch } from '../navigation/items'
const open = defineModel<boolean>({ required: true })
const router = useRouter(), query = ref(''), selected = ref(0), input = ref<HTMLInputElement>()
const results = computed(() => searchNavigation(query.value))
watch(query, () => { selected.value = 0 })
watch(open, value => { if (value) { query.value = ''; selected.value = 0 } })
function focusInput(event?: Event) { event?.preventDefault(); void nextTick(() => input.value?.focus()) }
async function go(item: QuickSearchMatch) { open.value = false; await router.push(item.to) }
function keydown(event: KeyboardEvent) {
  if (['ArrowDown', 'ArrowUp'].includes(event.key)) {
    event.preventDefault(); if (!results.value.length) return
    selected.value = (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + results.value.length) % results.value.length
    void nextTick(() => document.getElementById(`quick-option-${results.value[selected.value]?.id}`)?.scrollIntoView({ block: 'nearest' }))
  } else if (event.key === 'Enter' && results.value[selected.value]) { event.preventDefault(); void go(results.value[selected.value]!) }
}
</script>
<template>
  <el-dialog v-model="open" title="快速前往" width="min(620px, 94vw)" class="quick-navigator" @open-auto-focus="focusInput" @opened="focusInput()">
    <div class="destination-search"><Search aria-hidden="true" /><input ref="input" v-model="query" role="combobox" aria-label="搜索功能入口" aria-autocomplete="list" aria-controls="quick-options" aria-expanded="true" :aria-activedescendant="results[selected] ? `quick-option-${results[selected].id}` : undefined" placeholder="搜索页面或标签，例如 日志、115、命名模板" autocomplete="off" @keydown="keydown" /></div>
    <div id="quick-options" role="listbox" aria-label="功能入口" class="destination-results"><button v-for="(item, index) in results" :id="`quick-option-${item.id}`" :key="item.id" type="button" role="option" tabindex="-1" :aria-selected="selected === index" @pointermove="selected = index" @click="go(item)"><component :is="item.icon" aria-hidden="true" /><span>{{ item.name }}<small>{{ item.group || '首页' }}</small></span><ArrowRight aria-hidden="true" /></button></div>
    <p v-if="!results.length" role="status">没有匹配的功能，请尝试其他关键词。</p>
    <footer class="destination-help">↑ ↓ 选择 · Enter 前往 · Esc 关闭</footer>
  </el-dialog>
</template>
<style scoped>
.destination-search { display: flex; gap: 12px; align-items: center; padding: 12px; border: 1px solid var(--mt-border-color); border-radius: 8px; }
.destination-search svg, .destination-results svg { width: 20px; height: 20px; flex-shrink: 0; }
.destination-search input { min-height: 44px; min-width: 0; width: 100%; border: 0; background: none; color: var(--mt-text-primary); outline: none; font: inherit; }
.destination-results { max-height: min(420px, 55dvh); overflow-y: auto; margin-top: 12px; }
.destination-results button { width: 100%; min-height: 56px; display: flex; align-items: center; text-align: left; gap: 12px; padding: 10px 12px; border: 0; border-radius: 8px; background: none; color: var(--mt-text-primary); cursor: pointer; }
.destination-results button[aria-selected=true] { background: var(--mt-color-primary-light); color: var(--mt-color-primary); }
.destination-results button > span { flex: 1; }.destination-results small { display: block; margin-top: 3px; color: var(--mt-text-muted); font-size: 12px; }
.destination-help { padding-top: 12px; color: var(--mt-text-muted); font-size: 12px; }
</style>
