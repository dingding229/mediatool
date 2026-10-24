<script setup lang="ts">
import {computed} from 'vue';
import {Marked} from 'marked';
import DOMPurify from 'dompurify';
const props=withDefaults(defineProps<{notes:string;tag:string;expanded?:boolean}>(),{expanded:true});
// Never pass source HTML or images to the DOM, even before sanitization.
const markdown=new Marked({renderer:{html:()=>'',image:()=>''}});
const html=computed(()=>{
 const fragment=DOMPurify.sanitize(markdown.parse(props.notes,{async:false}),{
  ALLOWED_TAGS:['h1','h2','h3','h4','h5','h6','p','ul','ol','li','strong','em','del','code','pre','blockquote','a','br','hr'],
  ALLOWED_ATTR:['href','title'],ALLOW_DATA_ATTR:false,ALLOW_ARIA_ATTR:false,RETURN_DOM_FRAGMENT:true,
 });
 for(const link of fragment.querySelectorAll('a')){
  const href=link.getAttribute('href');
  try{if(href&&!['http:','https:'].includes(new URL(href,window.location.href).protocol))link.removeAttribute('href')}catch{link.removeAttribute('href')}
  link.setAttribute('target','_blank');link.setAttribute('rel','noopener noreferrer');
 }
 const container=document.createElement('div');container.append(fragment);return container.innerHTML;
});
</script>
<template>
 <details class="release-notes" :open="expanded">
  <summary>变更说明 · {{tag}}</summary>
  <div class="release-notes-body" v-html="html"/>
 </details>
</template>
<style scoped>
.release-notes{margin-top:16px;border:1px solid var(--mt-border-color);border-radius:8px;overflow-wrap:anywhere}
.release-notes summary{display:list-item;list-style:disclosure-closed inside;min-height:44px;box-sizing:border-box;padding:12px 16px;cursor:pointer;font-weight:600;line-height:1.5}
.release-notes[open]>summary{list-style-type:disclosure-open}
.release-notes summary:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}
.release-notes-body{padding:0 16px 16px;line-height:1.7;color:var(--mt-text-primary)}
.release-notes-body :deep(:first-child){margin-top:0}.release-notes-body :deep(:last-child){margin-bottom:0}
.release-notes-body :deep(h1),.release-notes-body :deep(h2),.release-notes-body :deep(h3),.release-notes-body :deep(h4),.release-notes-body :deep(h5),.release-notes-body :deep(h6){font-size:16px;line-height:1.5;margin:16px 0 8px}
.release-notes-body :deep(ul),.release-notes-body :deep(ol){padding-left:24px}.release-notes-body :deep(a){color:var(--mt-color-primary);text-decoration:underline}
.release-notes-body :deep(code){background:var(--mt-bg-canvas);border-radius:4px;padding:2px 4px;font-size:.95em;white-space:pre-wrap}
.release-notes-body :deep(pre){padding:12px;background:var(--mt-bg-canvas);border-radius:6px;white-space:pre-wrap;overflow-wrap:anywhere}
.release-notes-body :deep(blockquote){margin-inline:0;padding-left:12px;border-left:3px solid var(--mt-border-color);color:var(--mt-text-muted)}
</style>
