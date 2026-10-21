<script setup lang="ts">
import { computed } from 'vue';
defineOptions({ inheritAttrs: false });
const props = withDefaults(defineProps<{
  currentPage?: number;
  pageSize?: number;
  total?: number;
  hasNext?: boolean;
  disabled?: boolean;
  label?: string;
}>(), { currentPage: 1, pageSize: 25, label: '分页', hasNext: undefined });
const emit = defineEmits<{
  'update:currentPage': [
    number
  ];
  'current-change': [
    number
  ];
}>();
const canNext = computed(() => props.hasNext ?? (props.total != null && props.currentPage * props.pageSize < props.total));
function go(n: number) {
  if (props.disabled || n < 1 || n === props.currentPage || n > props.currentPage && !canNext.value)
    return;
  emit('update:currentPage', n);
  emit('current-change', n);
}
</script>
<template>
  <nav class="mt-pagination" :aria-label="label">
    <button type="button" :disabled="disabled||currentPage<=1" @click="go(currentPage-1)">上一页</button>
    <span aria-live="polite">第 {{currentPage}} 页</span>
    <button type="button" :disabled="disabled||!canNext" @click="go(currentPage+1)">下一页</button>
  </nav>
</template>
<style scoped>
.mt-pagination{display:flex;align-items:center;justify-content:center;gap:16px;flex-wrap:wrap;margin:24px 0 4px;min-width:0;color:var(--mt-text-muted);font-size:13px}.mt-pagination button{min-height:44px;min-width:80px;padding:8px 12px;border:1px solid var(--mt-border-color);border-radius:8px;background:var(--mt-bg-surface);color:var(--mt-text-primary);font:inherit;cursor:pointer}.mt-pagination button:hover:not(:disabled){border-color:var(--mt-color-primary);color:var(--mt-color-primary)}.mt-pagination button:disabled{color:var(--mt-text-muted);opacity:1;cursor:not-allowed}.mt-pagination button:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}
</style>
