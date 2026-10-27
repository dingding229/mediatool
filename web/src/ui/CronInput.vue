<script setup lang="ts">
import {computed,reactive,ref,watch} from 'vue';
import type {CronPurpose} from '../api/scheduling';
import DetailDrawer from './DetailDrawer.vue';
import {cronModes,decodeCron,draftError,generateCron} from './cron-generator';
import {useCronValidation} from './cron-validation';
const props=withDefaults(defineProps<{modelValue:string;purpose:CronPurpose;label?:string;disabled?:boolean}>(),{label:'Cron 表达式'});
const emit=defineEmits<{'update:modelValue':[string];valid:[boolean]}>();
const {valid,pending,error,preview}=useCronValidation(()=>props.modelValue,()=>props.purpose);
watch(valid,value=>emit('valid',value),{immediate:true,flush:'sync'});
const generatorOpen=ref(false),draft=reactive(decodeCron('')),base=ref(''),basePurpose=ref<CronPurpose>(props.purpose);
const expression=computed(()=>generateCron(draft));
const contextChanged=computed(()=>base.value!==props.modelValue||basePurpose.value!==props.purpose);
const draftValidation=reactive(useCronValidation(()=>expression.value,()=>props.purpose,()=>draftError(draft),()=>generatorOpen.value));
const canConfirm=computed(()=>generatorOpen.value&&!props.disabled&&!contextChanged.value&&draftValidation.valid&&!draftValidation.pending);
function input(value:unknown){const text=String(value);emit('valid',text===props.modelValue&&valid.value);emit('update:modelValue',text)}
function openGenerator(){
 if(props.disabled)return;
 Object.assign(draft,decodeCron(props.modelValue));base.value=props.modelValue;basePurpose.value=props.purpose;generatorOpen.value=true;
}
function confirm(){if(!canConfirm.value)return;input(expression.value);generatorOpen.value=false}
watch(()=>props.disabled,value=>{if(value)generatorOpen.value=false});
</script>
<template>
 <div class="cron-input" :aria-busy="pending">
  <div class="cron-main">
   <label class="cron-expression">{{label}}<el-input :model-value="modelValue" @update:model-value="input" :aria-label="label" :disabled="disabled" :aria-invalid="!!error"/></label>
   <el-button class="cron-generate" :aria-label="label+'生成'" :disabled="disabled" @click="openGenerator">生成</el-button>
  </div>
  <small>5位：分 时 日 月 周；6位增加秒；7位再增加年（1970～2199）。时区：上海。步长在相应时间字段内循环。</small>
  <div class="cron-feedback" aria-live="polite">
   <p v-if="error" role="alert" class="cron-error">{{error}}</p>
   <p v-else-if="pending" role="status">正在校验计划…</p>
   <p v-else-if="preview?.exhausted" role="status">计划已结束，没有后续执行时间</p>
   <p v-else-if="valid" role="status">校验通过</p>
   <small v-if="preview?.minimumIntervalSeconds">此业务最小间隔：{{preview.minimumIntervalSeconds}} 秒</small>
  </div>
  <DetailDrawer v-model:visible="generatorOpen" :title="label+'生成器'" size="small" append-to-body>
   <div class="cron-generator">
    <div class="cron-generator-fields">
     <label>生成方式<el-select v-model="draft.mode" :aria-label="label+'生成方式'"><el-option v-for="[value,text] in cronModes" :key="value" :value="value" :label="text"/></el-select></label>
     <template v-if="draft.mode!=='custom'">
      <label v-if="['seconds','minutes','hours'].includes(draft.mode)">间隔<el-input-number v-model="draft.interval" :controls="false" :min="1" :max="draft.mode==='hours'?23:59" :step="1" step-strictly :aria-label="label+'间隔'"/></label>
      <label v-else-if="draft.mode==='hourly'">每小时第几分钟<el-input-number v-model="draft.minute" :controls="false" :min="0" :max="59" :step="1" step-strictly :aria-label="label+'分钟'"/></label>
      <label v-else>执行时刻<el-input v-model="draft.clock" placeholder="09:00" :aria-label="label+'执行时刻'"/></label>
      <label v-if="draft.mode==='weekly'">星期<el-select v-model="draft.weekdays" multiple :aria-label="label+'星期'"><el-option v-for="(name,n) in ['星期日','星期一','星期二','星期三','星期四','星期五','星期六']" :key="n" :value="n" :label="name"/></el-select></label>
      <label v-if="draft.mode==='monthly'">日期<el-select v-model="draft.monthdays" multiple :aria-label="label+'日期'"><el-option v-for="n in 31" :key="n" :value="n" :label="n+'日'"/></el-select></label>
      <label>年份<el-input v-model="draft.year" placeholder="* 或 2030-2040" :aria-label="label+'年份'"/></label>
     </template>
    </div>
    <label v-if="draft.mode==='custom'">自定义表达式<el-input v-model="draft.custom" :aria-label="label+'自定义表达式'" :aria-invalid="!!draftValidation.error"/></label>
    <label v-else>生成的表达式<el-input :model-value="expression" readonly :aria-label="label+'生成表达式'"/></label>
    <div class="cron-feedback" aria-live="polite">
     <p v-if="contextChanged" role="alert" class="cron-error">原表达式已变化，请取消后重新打开生成器。</p>
     <p v-else-if="draftValidation.error" role="alert" class="cron-error">{{draftValidation.error}}</p>
     <p v-else-if="draftValidation.pending" role="status">正在校验计划…</p>
     <p v-else-if="draftValidation.preview?.exhausted" role="status">计划已结束，没有后续执行时间</p>
     <p v-else-if="draftValidation.valid" role="status">校验通过</p>
     <small v-if="draftValidation.preview?.minimumIntervalSeconds">此业务最小间隔：{{draftValidation.preview.minimumIntervalSeconds}} 秒</small>
    </div>
    <small>支持5、6、7位表达式和1970～2199年份；年份支持星号、列表、范围及步长。按上海时区校验，确定只填回表单，不直接保存或执行计划。</small>
   </div>
   <template #footer><el-button class="cron-generator-action" @click="generatorOpen=false">取消</el-button><el-button class="cron-generator-action" type="primary" :disabled="!canConfirm" @click="confirm">确定</el-button></template>
  </DetailDrawer>
 </div>
</template>
<style scoped>
.cron-input{display:grid;gap:8px;min-width:0}.cron-main{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:end;gap:8px}.cron-expression,.cron-generator label{display:grid;gap:6px;min-width:0}.cron-main :deep(.el-input__wrapper),.cron-generator :deep(.el-input__wrapper),.cron-generator :deep(.el-select__wrapper),.cron-generate{min-height:44px;box-sizing:border-box}.cron-generator{display:grid;gap:16px;min-width:0}.cron-generator-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.cron-generator :deep(.el-input-number){width:100%}.cron-generator-action{min-height:44px;margin-left:0}.cron-feedback{min-height:44px;font-size:13px;line-height:1.6;overflow-wrap:anywhere}.cron-feedback p{margin:0}.cron-error{color:var(--el-color-danger)}.cron-input small,.cron-generator small{color:var(--mt-text-muted);line-height:1.6;overflow-wrap:anywhere}@media(max-width:599.98px){.cron-generator-fields{grid-template-columns:minmax(0,1fr)}}
</style>
