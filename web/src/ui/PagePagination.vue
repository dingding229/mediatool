<script setup lang="ts">
import {computed,watch} from 'vue';
defineOptions({inheritAttrs:false});
const props=withDefaults(defineProps<{currentPage?:number;pageSize?:number;total?:number;hasNext?:boolean;disabled?:boolean;label?:string;pageSizes?:number[];status?:'loading'|'error'|''}>(),{currentPage:1,pageSize:25,label:'分页',hasNext:undefined,pageSizes:()=>[10,25,50,100]});
const emit=defineEmits<{'update:currentPage':[number];'current-change':[number];'update:pageSize':[number];'size-change':[number]}>();
const pages=computed(()=>props.total==null?undefined:Math.max(1,Math.ceil(props.total/props.pageSize)));
const pageLabel=computed(()=>`第 ${props.currentPage} 页${pages.value==null?'':` / 共 ${pages.value} 页`}`);
const canNext=computed(()=>props.hasNext??(pages.value!=null&&props.currentPage<pages.value));
const sizeOptions=computed(()=>props.pageSizes.includes(props.pageSize)?props.pageSizes:[props.pageSize,...props.pageSizes]);
let requested:number|undefined;
watch(()=>props.currentPage,value=>{if(value===requested){requested=undefined;return}emit('current-change',value)});
function go(n:number){if(props.disabled||n<1||n===props.currentPage||pages.value!=null&&n>pages.value||n>props.currentPage&&!canNext.value)return;requested=n;emit('update:currentPage',n);emit('current-change',n)}
function resize(value:number){if(props.disabled||value===props.pageSize||!sizeOptions.value.includes(value))return;requested=1;emit('update:pageSize',value);emit('update:currentPage',1);emit('size-change',value);emit('current-change',1)}
</script>
<template>
 <nav class="mt-pagination" :aria-label="label">
  <div class="mt-page-summary">
   <label>每页 <el-select :model-value="pageSize" :aria-label="label+'每页条数'" :disabled="disabled" @change="resize"><el-option v-for="n in sizeOptions" :key="n" :value="n" :label="String(n)"/></el-select> 条<span v-if="total!=null">，共 {{total}} 条</span></label>
   <span v-if="status" class="mt-page-count" aria-live="polite">{{status==='loading'?'读取中':'读取失败'}}{{total==null?'，数量未知':''}}</span>
  </div>
  <div class="mt-page-actions"><button type="button" :disabled="disabled||currentPage<=1" @click="go(1)">首页</button><button type="button" :disabled="disabled||currentPage<=1" @click="go(currentPage-1)">上一页</button><span>{{pageLabel}}</span><button type="button" :disabled="disabled||!canNext" @click="go(currentPage+1)">下一页</button><button type="button" :disabled="disabled||!pages||currentPage>=pages" @click="pages&&go(pages)">末页</button></div>
 </nav>
</template>
<style scoped>
.mt-pagination{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;margin:16px 0 4px;min-height:44px;color:var(--mt-text-muted);font-size:13px;font-variant-numeric:tabular-nums}.mt-page-summary{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.mt-pagination label,.mt-page-actions{display:flex;align-items:center;gap:8px}.mt-pagination .el-select{width:80px}.mt-pagination button{min-height:44px;min-width:44px;padding:6px 10px;border:1px solid var(--mt-border-color);border-radius:6px;background:var(--mt-bg-surface);color:var(--mt-text-primary);font:inherit;cursor:pointer}.mt-pagination button:hover:not(:disabled){border-color:var(--mt-color-primary);color:var(--mt-color-primary)}.mt-pagination button:disabled{color:var(--mt-text-muted);opacity:.6;cursor:not-allowed}.mt-pagination button:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}@media(max-width:600px){.mt-page-actions{width:100%;justify-content:space-between;gap:4px}.mt-page-actions button{padding-inline:6px}.mt-page-actions>span{white-space:nowrap;font-size:12px}.mt-pagination{justify-content:space-between}.mt-page-count{font-size:12px}}
</style>
