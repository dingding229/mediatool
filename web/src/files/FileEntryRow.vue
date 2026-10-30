<script setup lang="ts">
import {FolderOpened,Document} from '@element-plus/icons-vue';import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
defineProps<{name:string;directory?:boolean;description?:string;disabled?:boolean}>();const emit=defineEmits<{navigate:[]}>();
</script>
<template><li class="files-entry"><slot name="selection"/><component :is="directory?FolderOpened:Document" aria-hidden="true" class="files-entry-icon"/><div class="files-entry-copy"><el-tooltip v-if="directory" :content="name" :trigger="['hover','focus']" popper-class="middle-ellipsis-tooltip"><button :aria-label="name" class="files-directory" :disabled="disabled" @click="emit('navigate')"><MiddleEllipsis :text="name" :copy="false" embedded/></button></el-tooltip><strong v-else><MiddleEllipsis :text="name" :copy="false"/></strong><span v-if="description">{{description}}</span></div><slot name="actions"/></li></template>
