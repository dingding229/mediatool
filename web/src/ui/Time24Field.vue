<script setup lang="ts">
import {computed,ref} from 'vue';
import {ElInput,ElPopover,ElSelect,ElOption} from 'element-plus';
import {Clock} from '@element-plus/icons-vue';
import 'element-plus/es/components/popover/style/css';
import 'element-plus/es/components/input/style/css';
import 'element-plus/es/components/select/style/css';
import 'element-plus/es/components/option/style/css';
const props=defineProps<{modelValue:string;label:string;disabled?:boolean;id?:string}>();
const emit=defineEmits<{'update:modelValue':[string]}>(),open=ref(false);
// A wall-clock string is not an instant: never parse it as Date or convert browser timezones.
const parts=computed(()=>/^([01]\d|2[0-3]):[0-5]\d$/.test(props.modelValue)?props.modelValue.split(':'):['00','00']);
const hours=Array.from({length:24},(_,i)=>String(i).padStart(2,'0')),minutes=Array.from({length:60},(_,i)=>String(i).padStart(2,'0'));
function input(value:string){emit('update:modelValue',value)}
function hour(value:string){emit('update:modelValue',value+':'+parts.value[1])}
function minute(value:string){emit('update:modelValue',parts.value[0]+':'+value)}
</script>
<template>
 <div class="mt-time24-field">
  <ElInput :id="id" :model-value="modelValue" :aria-label="label" :disabled="disabled" inputmode="numeric" autocomplete="off" placeholder="HH:mm" @update:model-value="input"/>
  <ElPopover v-model:visible="open" trigger="click" placement="bottom-end" :width="240" :disabled="disabled">
   <template #reference><button type="button" class="clock-trigger" :aria-label="label+'选择时间'" :disabled="disabled"><Clock aria-hidden="true"/></button></template>
   <div class="clock-options"><label>小时<ElSelect :model-value="parts[0]" :aria-label="label+'小时'" @update:model-value="hour"><ElOption v-for="value in hours" :key="value" :value="value" :label="value"/></ElSelect></label><label>分钟<ElSelect :model-value="parts[1]" :aria-label="label+'分钟'" @update:model-value="minute"><ElOption v-for="value in minutes" :key="value" :value="value" :label="value"/></ElSelect></label></div>
   <div class="clock-done"><el-button size="small" @click="open=false">完成</el-button></div>
  </ElPopover>
 </div>
</template>
<style scoped>
.mt-time24-field{position:relative;display:block;width:100%;max-width:100%;min-width:0}.mt-time24-field :deep(.el-input){width:100%;max-width:100%;min-width:0}.mt-time24-field :deep(.el-input__wrapper){height:var(--mt-form-control-height);min-height:var(--mt-form-control-height);box-sizing:border-box;padding-right:44px}.clock-trigger{position:absolute;right:0;top:0;display:grid;place-items:center;width:44px;height:44px;min-height:44px;padding:0;border:0;background:transparent;cursor:pointer;color:var(--mt-text-muted)}.clock-trigger:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:-3px}.clock-trigger svg{width:16px;height:16px}.clock-options{display:grid;grid-template-columns:1fr 1fr;gap:var(--mt-form-field-gap,8px)}.clock-options label{display:grid;gap:var(--mt-form-field-gap,8px);font-size:var(--mt-form-label-size);font-weight:var(--mt-form-label-weight);min-width:0}.clock-done{display:flex;justify-content:flex-end;margin-top:8px}
</style>
