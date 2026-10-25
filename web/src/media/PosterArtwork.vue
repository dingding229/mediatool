<script setup lang="ts">
import {ref,watch} from 'vue';
import {Film} from '@element-plus/icons-vue';
const props=defineProps<{src?:string;title:string}>(),broken=ref(false);
watch(()=>props.src,()=>{broken.value=false});
</script>
<template><span class="poster-artwork"><img v-if="src&&!broken" :src="src" :alt="title+'海报'" loading="lazy" decoding="async" @error="broken=true"/><span v-else class="poster-artwork-placeholder"><Film aria-hidden="true"/><small>暂无海报</small></span></span></template>
<style scoped>
.poster-artwork{display:block;aspect-ratio:2/3;overflow:hidden;border:1px solid var(--mt-border-color);border-radius:8px;background:var(--mt-bg-surface);min-width:0}.poster-artwork img{width:100%;height:100%;object-fit:cover}.poster-artwork-placeholder{height:100%;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;color:var(--mt-text-muted)}.poster-artwork-placeholder svg{width:28px;height:28px}.poster-artwork-placeholder small{font-size:12px}
</style>
