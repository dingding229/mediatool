<script setup lang="ts">
withDefaults(defineProps<{active?:boolean;disabled?:boolean}>(),{active:false,disabled:false});
defineEmits<{click:[]}>();
</script>
<template><button type="button" class="filter-chip" :class="{'is-active':active}" :aria-pressed="active" :disabled="disabled" @click="$emit('click')"><span><slot/></span></button></template>
<style scoped>
.filter-chip{display:inline-flex;align-items:center;justify-content:center;min-height:44px;padding:0;border:0;background:transparent;color:var(--mt-text-muted);font:inherit;cursor:pointer}.filter-chip>span{display:inline-flex;align-items:center;min-height:26px;padding:2px 10px;box-sizing:border-box;border:1px solid var(--mt-border-color);border-radius:999px;font-size:12px;line-height:18px;white-space:nowrap}.filter-chip.is-active>span{color:var(--mt-color-primary);border-color:var(--mt-color-primary);background:var(--mt-color-primary-light)}.filter-chip:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:2px;border-radius:999px}.filter-chip:disabled{opacity:.5;cursor:not-allowed}
</style>
