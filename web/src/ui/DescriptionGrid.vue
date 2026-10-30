<script setup lang="ts">
import {computed} from 'vue';
export interface DescriptionItem {key:string;label:string;value?:unknown}
const props=withDefaults(defineProps<{items:DescriptionItem[];columns?:2|3}>(),{columns:3});
const visible=computed(()=>props.items.filter(item=>item.value!==null&&item.value!==undefined&&String(item.value).trim()!==''&&String(item.value).trim()!=='—'));
</script>
<template>
 <dl v-if="visible.length" class="mt-description-grid" :style="{'--description-columns':columns}">
  <div v-for="item in visible" :key="item.key" class="mt-description-item">
   <dt>{{item.label}}</dt><dd><slot :name="item.key" :item="item">{{item.value}}</slot></dd>
  </div>
 </dl>
</template>
<style scoped>
.mt-description-grid{display:grid;grid-template-columns:repeat(var(--description-columns),minmax(0,1fr));gap:20px 24px;margin:20px 0}.mt-description-item{min-width:0}.mt-description-item dt{font-size:13px;font-weight:600;color:var(--mt-text-muted);margin-bottom:6px}.mt-description-item dd{margin:0;min-width:0;overflow-wrap:anywhere;font-size:14px;line-height:1.6}@media(max-width:900px){.mt-description-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:480px){.mt-description-grid{grid-template-columns:minmax(0,1fr)}}
</style>
