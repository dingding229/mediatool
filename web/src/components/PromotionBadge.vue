<script setup lang="ts">
import {computed} from 'vue'
const props=defineProps<{factor:number|null}>()
const label=computed(()=>props.factor===null?'未知':props.factor===0?'免费':props.factor===1?'普通':`${Number((props.factor*100).toFixed(2))}%DL`)
const tone=computed(()=>props.factor===null?'unknown':props.factor===0?'free':props.factor===1?'normal':'discount')
</script>
<template><span class="promotion-badge" :class="tone">{{label}}</span></template>
<style scoped>.promotion-badge{display:inline-flex;align-items:center;border:1px solid;border-radius:99px;padding:3px 9px;font-size:12px;line-height:18px;white-space:nowrap}.normal{color:#246bd1;background:#edf4ff;border-color:#91b9f4}.discount{color:#9a5800;background:#fff3db;border-color:#e9b356}.free{color:#166d42;background:#eaf7ee;border-color:#7ac89c}.unknown{color:var(--muted);background:var(--canvas);border-color:var(--border)}</style>
