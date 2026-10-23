<script setup lang="ts">
import {ref,watch} from 'vue';
import {lookupIP,type IPLocation} from '../api/ip-geolocation';
const props=defineProps<{ip?:string|null}>(),location=ref<IPLocation>(),failed=ref(false);let seq=0;
watch(()=>props.ip,async raw=>{const id=++seq;location.value=undefined;failed.value=false;try{const v=await lookupIP(raw||'');if(id===seq)location.value=v}catch{if(id===seq)failed.value=true}},{immediate:true});
</script>
<template><span class="f7-ip" :aria-busy="!location&&!failed">{{failed||location?.unavailable?'IP 暂不可用':!location?'…':location.ip?(location.ip+(location.label?' · '+location.label:'')):'内网访问'}}</span></template>
<style scoped>.f7-ip{display:inline-block;min-height:1.4em;overflow-wrap:anywhere;font-variant-numeric:tabular-nums}</style>
