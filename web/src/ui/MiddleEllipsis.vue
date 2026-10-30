<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,ref,watch} from 'vue';
import {ElTooltip,ElMessage} from 'element-plus';
import {CopyDocument} from '@element-plus/icons-vue';
import 'element-plus/es/components/message/style/css';
import 'element-plus/es/components/tooltip/style/css';
import {middleText,displayPath} from './middle-ellipsis';
const props=withDefaults(defineProps<{text?:string|null;path?:string|null;full?:boolean;copy?:boolean}>(),{full:false,copy:true});
const original=computed(()=>(props.path??props.text)||'—'),shown=ref(original.value),host=ref<HTMLElement>(),copying=ref(false),notice=ref('');
let observer:ResizeObserver|undefined,fontObserver:MutationObserver|undefined,alive=true;
function measure(){
 if(!host.value||props.full){shown.value=original.value;return;}
 const width=host.value.clientWidth;if(!width)return;
 const context=document.createElement('canvas').getContext('2d');if(!context)return;
 context.font=getComputedStyle(host.value).font;
 const pixels=(value:string)=>context.measureText(value).width;
 shown.value=props.path?displayPath(original.value,width,pixels):middleText(original.value,width,pixels);
}
async function copyFull(){
 if(copying.value)return;copying.value=true;
 try{await navigator.clipboard.writeText(original.value);notice.value='已复制';ElMessage.success(notice.value);}
 catch{notice.value='复制失败，请在全文提示中手动复制';ElMessage.warning(notice.value);}
 finally{copying.value=false;}
}
onMounted(()=>{
 observer=new ResizeObserver(measure);if(host.value)observer.observe(host.value);
 // Selected options can become bold without a width change. Remeasure inherited font/style changes too.
 if(typeof MutationObserver==='function'){
  fontObserver=new MutationObserver(measure);
  for(let node=host.value;node;node=node.parentElement||undefined)fontObserver.observe(node,{attributes:true,attributeFilter:['class','style']});
 }
 measure();void document.fonts?.ready.then(()=>{if(alive)measure()});
});
watch([original,()=>props.full],()=>{shown.value=original.value;void nextTick(measure)});
onBeforeUnmount(()=>{alive=false;observer?.disconnect();fontObserver?.disconnect()});
</script>
<template>
 <span class="middle-ellipsis" :class="{'middle-ellipsis--full':full,'middle-ellipsis--copy':copy}">
  <ElTooltip :content="original" :trigger="['hover','focus']" :show-after="200" placement="top" popper-class="middle-ellipsis-tooltip">
   <span ref="host" class="middle-ellipsis-text" tabindex="0" :aria-label="original"><span aria-hidden="true">{{full?original:shown}}</span></span>
  </ElTooltip>
  <button v-if="copy" type="button" class="middle-copy" aria-label="复制全文" title="复制全文" :disabled="copying" @click.stop.prevent="copyFull"><CopyDocument aria-hidden="true"/></button>
  <span class="middle-copy-notice" role="status">{{notice}}</span>
 </span>
</template>
<style scoped>
.middle-ellipsis{display:block;position:relative;min-width:0;max-width:100%;line-height:1.6;text-align:start;box-sizing:border-box}.middle-ellipsis--copy{padding-inline-end:44px}.middle-ellipsis-text{display:block;min-width:0;overflow:hidden;white-space:nowrap}.middle-ellipsis--full .middle-ellipsis-text{white-space:normal;overflow-wrap:anywhere}.middle-ellipsis-text:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:2px}.middle-copy{position:absolute;inset-inline-end:0;top:50%;transform:translateY(-50%);width:44px;height:44px;min-width:44px;min-height:44px;border:0;background:transparent;color:var(--mt-text-muted);padding:12px;cursor:pointer}.middle-copy:hover{color:var(--mt-color-primary)}.middle-copy svg{width:16px;height:16px}.middle-copy-notice{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
<style>
.middle-ellipsis-tooltip{max-width:min(560px,calc(100vw - 32px));white-space:normal;overflow-wrap:anywhere}
</style>
