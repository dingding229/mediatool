<script setup lang="ts">
import {onUnmounted,ref,watch} from 'vue';
import {pendingRequests} from '../api/activity';
const visible=ref(false);let show:ReturnType<typeof setTimeout>,hide:ReturnType<typeof setTimeout>;
watch(pendingRequests,n=>{clearTimeout(show);clearTimeout(hide);if(n>0){if(!visible.value)show=setTimeout(()=>visible.value=true,120)}else hide=setTimeout(()=>visible.value=false,180)},{immediate:true});
onUnmounted(()=>{clearTimeout(show);clearTimeout(hide)});
</script>
<template><div class="mt-top-progress" :class="{'is-active':visible}" role="progressbar" aria-label="页面正在更新" :aria-hidden="!visible"><i/></div></template>
<style scoped>
.mt-top-progress{position:fixed;inset:0 0 auto;z-index:10000;height:2px;pointer-events:none;opacity:0;transition:opacity .15s;background:transparent}.mt-top-progress.is-active{opacity:1}.mt-top-progress i{display:block;height:100%;width:35%;background:var(--mt-color-primary,#249f8b);box-shadow:0 0 8px currentColor;animation:mt-load 1.3s ease-in-out infinite}@keyframes mt-load{from{transform:translateX(-100%)}to{transform:translateX(390%)}}@media(prefers-reduced-motion:reduce){.mt-top-progress{transition:none}.mt-top-progress i{animation:none;width:100%}}
</style>
