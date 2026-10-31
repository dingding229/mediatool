<script setup lang="ts">
import {computed,nextTick,onBeforeUnmount,onMounted,ref,watch} from 'vue';
import {ElTooltip,ElMessage} from 'element-plus';
import {CopyDocument} from '@element-plus/icons-vue';
import 'element-plus/es/components/message/style/css';
import 'element-plus/es/components/tooltip/style/css';
import {middleText,displayPath,displayFilenamePath} from './middle-ellipsis';
const props=withDefaults(defineProps<{text?:string|null;path?:string|null;full?:boolean;copy?:boolean;embedded?:boolean;filenameOnly?:boolean;}>(),{full:false,copy:true,embedded:false});
const frame=ref<HTMLElement>();
const original=computed(()=>(props.path??props.text)||'—'),shown=ref(original.value),host=ref<HTMLElement>(),copying=ref(false),notice=ref('');
let observer:ResizeObserver|undefined,fontObserver:MutationObserver|undefined,alive=true;
function measure(){
 if(!host.value||props.full){shown.value=original.value;return;}
 // Measure the stable container, so a shorter rendered string cannot shrink its own next measurement.
 const width=(frame.value?.clientWidth||host.value.clientWidth)-(props.copy?44:0);if(!width)return;
 const context=document.createElement('canvas').getContext('2d');if(!context)return;
 context.font=getComputedStyle(host.value).font;
 const pixels=(value:string)=>context.measureText(value).width;
 shown.value=props.path?(props.filenameOnly?displayFilenamePath(original.value,width,pixels):displayPath(original.value,width,pixels)):middleText(original.value,width,pixels);
}
async function copyFull(){
 if(copying.value)return;copying.value=true;
 try{await navigator.clipboard.writeText(original.value);notice.value='已复制';ElMessage.success(notice.value);}
 catch{notice.value='复制失败，请在全文提示中手动复制';ElMessage.warning(notice.value);}
 finally{copying.value=false;}
}
onMounted(()=>{
 observer=new ResizeObserver(measure);if(frame.value)observer.observe(frame.value);
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
 <span ref="frame" class="middle-ellipsis" :class="{'middle-ellipsis--full':full,'middle-ellipsis--copy':copy}">
  <ElTooltip :disabled="embedded||full" :content="original" :trigger="['hover','focus']" :show-after="200" placement="top" popper-class="middle-ellipsis-tooltip">
   <span ref="host" class="middle-ellipsis-text" :tabindex="embedded?undefined:0" :aria-label="original"><span aria-hidden="true">{{full?original:shown}}</span></span>
  </ElTooltip>
  <button v-if="copy" type="button" class="middle-copy" aria-label="复制全文" title="复制全文" :disabled="copying" @click.stop.prevent="copyFull"><CopyDocument aria-hidden="true"/></button>
  <span v-if="copy" class="middle-copy-notice" role="status">{{notice}}</span>
 </span>
</template>
<style scoped>
.middle-ellipsis{display:flex;position:relative;align-items:center;min-width:0;width:100%;max-width:100%;line-height:1.6;text-align:start;box-sizing:border-box}.middle-ellipsis-text{display:block;flex:0 1 auto;min-width:0;overflow:hidden;white-space:nowrap}.middle-ellipsis-text:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:2px}.middle-copy{display:inline-flex;align-items:center;justify-content:center;flex:none;width:44px;height:44px;min-width:44px;min-height:44px;border:0;background:transparent;color:var(--mt-text-muted);padding:12px;margin-block:-11px;cursor:pointer;vertical-align:middle}.middle-copy:hover{color:var(--mt-color-primary)}.middle-copy svg{width:16px;height:16px}.middle-ellipsis--full{display:block}.middle-ellipsis--full .middle-copy{margin-block:0}.middle-ellipsis--full .middle-ellipsis-text{display:inline;white-space:normal;overflow-wrap:anywhere}.middle-copy-notice{position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%)}
</style>
<style>
.middle-ellipsis-tooltip{max-width:min(560px,calc(100vw - 32px));white-space:normal;overflow-wrap:anywhere}
</style>
