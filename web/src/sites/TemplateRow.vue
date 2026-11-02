<script setup lang="ts">
import {ref} from 'vue';
const props=withDefaults(defineProps<{site:any;selectable?:boolean;selected?:boolean;disabled?:boolean;used?:boolean}>(),{selectable:false,selected:false,disabled:false,used:false});
const emit=defineEmits<{select:[boolean];details:[]}>();const broken=ref(false);
</script>
<template><article class="u5-template" role="listitem">
 <component :is="selectable?'label':'div'" class="template-name u5-check"><input v-if="selectable" type="checkbox" :checked="selected" :aria-label="'选择模板 '+site.name" :disabled="disabled" @change="emit('select',($event.target as HTMLInputElement).checked)"/><img v-if="site.iconUrl?.startsWith('/api/v1/pt/templates/')&&!broken" :src="site.iconUrl" alt="" width="32" height="32" loading="lazy" referrerpolicy="no-referrer" @error="broken=true"/><span v-else class="f5-template-icon" aria-hidden="true">{{Array.from(site.name)[0]}}</span><strong>{{site.name}}</strong></component>
 <a v-if="/^https?:\/\//.test(site.domain)" :href="site.domain" target="_blank" rel="noopener noreferrer">{{site.domain}}</a><p v-else>{{site.domain||'未提供站点地址'}}</p>
 <p>{{used?site.unavailable?'使用中 · 当前内置列表未提供':'使用中':site.loaded?'已加载':'未加载'}}</p><el-button @click="emit('details')">模板详情</el-button>
</article></template>
<style scoped>.u5-template{display:grid;grid-template-columns:minmax(160px,1fr) minmax(150px,1.5fr) minmax(90px,auto) auto;align-items:center;gap:20px;border:0;border-bottom:1px solid var(--mt-border-color);border-radius:0;padding:14px 0;min-width:0}.u5-template:last-child{border-bottom:0}.template-name{min-width:0;gap:10px}.template-name strong{overflow-wrap:anywhere}.template-name input{flex:none}.template-name img,.f5-template-icon{width:32px;height:32px;flex:none}.f5-template-icon{display:grid;place-items:center;background:var(--mt-bg-canvas);border-radius:6px}a{overflow-wrap:anywhere}p{margin:0;font-size:13px;color:var(--mt-text-muted)}@media(max-width:700px){.u5-template{grid-template-columns:minmax(0,1fr) auto;gap:12px}a{grid-column:1/-1;grid-row:2}.u5-template>p{grid-column:1}.el-button{grid-column:2;grid-row:1}}</style>
