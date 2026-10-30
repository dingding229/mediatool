<script setup lang="ts">
import {ref,nextTick} from 'vue';
import {ElUpload,ElButton,type UploadFile} from 'element-plus';
import 'element-plus/es/components/upload/style/css';
import 'element-plus/es/components/button/style/css';
const props=defineProps<{accept:string;label:string;disabled?:boolean;showName?:boolean;buttonText?:string}>();
const emit=defineEmits<{select:[File]}>(),name=ref(''),picker=ref<InstanceType<typeof ElUpload>>();
function choose(file:UploadFile){if(!file.raw||props.disabled)return;name.value=file.name;emit('select',file.raw);void nextTick(()=>picker.value?.clearFiles());}
</script>
<template>
 <span class="mt-file-picker">
  <ElUpload ref="picker" :accept="accept" :auto-upload="false" :show-file-list="false" :disabled="disabled" :on-change="choose">
   <ElButton :disabled="disabled" :aria-label="label">{{buttonText||label}}</ElButton>
  </ElUpload>
  <span v-if="showName" class="mt-help">{{name||'未选择文件'}}</span>
 </span>
</template>
<style scoped>.mt-file-picker{display:inline-flex;gap:12px;align-items:center;max-width:100%;min-width:0;flex-wrap:wrap}.mt-file-picker :deep(.el-upload){display:inline-flex;max-width:100%}.mt-file-picker .mt-help{overflow-wrap:anywhere}</style>
