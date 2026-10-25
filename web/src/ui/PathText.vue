<script setup lang="ts">
import {computed} from 'vue';
import {splitDisplayPath} from './path-text';
const props=defineProps<{path?:string|null}>();
const full=computed(()=>props.path||'—'),parts=computed(()=>splitDisplayPath(full.value));
</script>
<template>
 <span class="mt-path" :title="full"><span class="mt-path-full">{{full}}</span><span class="mt-path-head" aria-hidden="true">{{parts[0]}}</span><span class="mt-path-tail" aria-hidden="true"><span>{{parts[1]}}</span></span></span>
</template>
<style scoped>
.mt-path-full{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
.mt-path{display:flex;min-width:0;max-width:100%;line-height:1.6;text-align:start}.mt-path-head{flex:0 1 auto;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.mt-path-tail{display:flex;justify-content:flex-end;flex:0 1 auto;min-width:0;max-width:55%;overflow:hidden}.mt-path-tail>span{flex:none;white-space:nowrap}
</style>
