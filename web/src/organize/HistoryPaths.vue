<script setup lang="ts">
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';import {present} from './presentation';
defineProps<{source?:string;target?:string}>();
</script>
<template><div class="history-paths" :class="{'history-paths--single':!present(target)}"><div v-if="present(source)"><small>源文件</small><MiddleEllipsis class="mt-path" :path="source"/></div><template v-if="present(target)"><span v-if="present(source)" aria-hidden="true">→</span><div><small>目标路径</small><MiddleEllipsis class="mt-path" :path="target"/></div></template></div></template>
