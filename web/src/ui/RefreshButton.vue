<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue';
defineProps<{
  loading?: boolean;
  disabled?: boolean;
}>();
defineEmits<{
  refresh: [
  ];
}>();
</script>
<template>
  <button class="mt-refresh" type="button" aria-label="刷新" title="刷新" :aria-busy="!!loading" :disabled="loading||disabled" @click="$emit('refresh')">
    <Refresh aria-hidden="true" :class="{'is-spinning':loading}"/>
  </button>
</template>
<style scoped>
.mt-refresh{width:44px;height:44px;min-width:44px;border-radius:50%;border:1px solid var(--mt-border-color);background:var(--mt-bg-surface);color:var(--mt-text-muted);display:inline-grid;place-items:center;cursor:pointer}.mt-refresh:hover{color:var(--mt-color-primary);border-color:var(--mt-color-primary)}.mt-refresh:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}.mt-refresh:disabled{cursor:wait;opacity:1}.mt-refresh svg{width:18px;height:18px}.is-spinning{animation:refresh-spin 1s linear infinite}@keyframes refresh-spin{to{transform:rotate(360deg)}}@media(prefers-reduced-motion:reduce){.is-spinning{animation:none}}
</style>
