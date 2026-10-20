<script setup lang="ts">
import { computed } from 'vue';
const props = defineProps<{
  factor: number | null | undefined;
}>();
const tone = computed(() => props.factor == null ? 'unknown' : props.factor === 0 ? 'free' : props.factor === 1 ? 'normal' : 'discount');
const text = computed(() => props.factor == null ? '未知' : props.factor === 0 ? '免费' : props.factor === 1 ? '普通' : `${Number((props.factor * 100).toFixed(2))}%DL`);
</script>
<template>
  <span class="promotion-badge" :class="tone">{{ text }}</span>
</template>
<style scoped>
.promotion-badge {display:inline-flex;align-items:center;justify-content:center;border:1px solid var(--mt-border-color);border-radius:6px;padding:3px 8px;font-size:12px;white-space:nowrap;color:var(--mt-text-muted);background:var(--mt-bg-canvas)}
.free {color:var(--mt-color-success);background:var(--el-color-success-light-9)}
.discount {color:var(--mt-color-warning);background:var(--el-color-warning-light-9)}
.normal {color:var(--mt-color-primary);background:var(--mt-color-primary-light)}
</style>
