<script setup lang="ts">
import Time24Field from "./Time24Field.vue";
import {ref,watch} from 'vue';
import type {CronPurpose} from '../api/scheduling';
import {beijingTime} from '../presentation/time';
import {useCronValidation} from './cron-validation';
import {cronPreset,cronPresets,presetCron,type CronPreset} from './cron-presets';
const props=withDefaults(defineProps<{modelValue:string;purpose:CronPurpose;label?:string;disabled?:boolean}>(),{label:'执行计划'});
const emit=defineEmits<{'update:modelValue':[string];valid:[boolean]}>();
const preset=ref<CronPreset>('custom'),clock=ref('09:00');
let emitted:string|undefined;
watch(()=>props.modelValue,value=>{if(value===emitted){emitted=undefined;return}emitted=undefined;const decoded=cronPreset(value);preset.value=decoded.preset;clock.value=decoded.clock},{immediate:true});
const {valid,pending,error,preview}=useCronValidation(()=>props.modelValue,()=>props.purpose);
watch(valid,value=>emit('valid',value),{immediate:true,flush:'sync'});
function update(value:string){emitted=value;emit('update:modelValue',value)}
function select(value:CronPreset){preset.value=value;if(value!=='custom')update(presetCron(value,clock.value))}
function time(value:string){clock.value=value;update(presetCron('daily',value))}
</script>
<template>
 <div class="cron-field" :aria-busy="pending">
  <div class="cron-field-controls"><label>{{label}}<el-select :model-value="preset" :aria-label="label+'预设'" :disabled="disabled" @change="select"><el-option v-for="[value,text] in cronPresets" :key="value" :value="value" :label="text"/></el-select></label>
   <label v-if="preset==='daily'">执行时刻（上海）<Time24Field :model-value="clock" :label="label+'执行时刻'" :disabled="disabled" @update:model-value="time"/></label>
   <label v-if="preset==='custom'">自定义 Cron<el-input :model-value="modelValue" :aria-label="label+'自定义 Cron'" :disabled="disabled" :aria-invalid="!!error" @update:model-value="update($event)"/></label>
  </div>
  <div class="cron-field-feedback" aria-live="polite"><p v-if="error" role="alert">{{error}}</p><small v-else-if="pending" role="status">正在校验计划…</small>
  <small v-else-if="preview?.exhausted" role="status">计划已结束，没有后续执行时间</small>
  <small v-else-if="valid" role="status">下次执行时间：{{beijingTime(preview?.next?.[0],'暂无')}}（上海）</small></div>
 </div>
</template>
<style scoped>
.cron-field{display:grid;gap:8px;min-width:0}.cron-field-controls{display:flex;align-items:end;flex-wrap:wrap;gap:12px}.cron-field label{display:grid;gap:6px;min-width:0;flex:1 1 180px;font-size:var(--mt-form-label-size);font-weight:600}.cron-field :deep(.el-select__wrapper),.cron-field :deep(.el-input__wrapper){min-height:44px;box-sizing:border-box}.cron-field-feedback{min-height:36px;line-height:1.5;overflow-wrap:anywhere}.cron-field small{font-size:12px;color:var(--mt-text-muted)}.cron-field p{color:var(--mt-color-danger);margin:0;font-size:12px}
</style>
