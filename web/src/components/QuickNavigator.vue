<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search, ArrowRight } from '@element-plus/icons-vue'
import { searchNavigation } from '../navigation/items'
const open = defineModel<boolean>({ required: true })
const router = useRouter(), query = ref(''), selected = ref(0), input = ref<HTMLInputElement>()
const results = computed(() => searchNavigation(query.value))
watch(query, () => { selected.value = 0 })
watch(open, value => { if (value) { query.value = ''; selected.value = 0 } })
async function go(id: string) { open.value = false; await router.push(`/${id}`) }
function keydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    if (!results.value.length) return
    selected.value = (selected.value + (event.key === 'ArrowDown' ? 1 : -1) + results.value.length) % results.value.length
    nextTick(() => document.getElementById(`quick-option-${results.value[selected.value]?.id}`)?.scrollIntoView({ block: 'nearest' }))
  } else if (event.key === 'Enter' && results.value[selected.value]) {
    event.preventDefault(); go(results.value[selected.value].id)
  }
}
</script>
<template>
  <el-dialog v-model="open" title="快速前往" width="min(580px,94vw)" class="quick-navigator" @opened="input?.focus()">
    <div class="quick-search-field"><Search aria-hidden="true" />
      <input ref="input" v-model="query" role="combobox" aria-label="搜索功能入口" aria-autocomplete="list" aria-controls="quick-options" aria-expanded="true"
        :aria-activedescendant="results[selected] ? `quick-option-${results[selected].id}` : undefined"
        placeholder="搜索功能，例如 115、日志、命名模板…" autocomplete="off" @keydown="keydown" />
    </div>
    <div id="quick-options" role="listbox" aria-label="功能入口" class="quick-results">
      <button v-for="(item,index) in results" :id="`quick-option-${item.id}`" :key="item.id" role="option" tabindex="-1"
        :aria-selected="index === selected" class="quick-option" @pointermove="selected=index" @click="go(item.id)">
        <component :is="item.icon" aria-hidden="true" /><span>{{ item.name }}<small>{{ item.group }}</small></span><ArrowRight aria-hidden="true" />
      </button>
    </div>
    <p v-if="!results.length" class="quick-empty" role="status">没有匹配的功能，请尝试其他关键词。</p>
    <div class="quick-help"><span><kbd>↑</kbd> <kbd>↓</kbd> 选择 <kbd>Enter</kbd> 前往</span><span><kbd>Esc</kbd> 关闭</span></div>
  </el-dialog>
</template>
