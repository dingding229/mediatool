<script setup lang="ts">
import {computed,onBeforeUnmount,ref,watch} from 'vue';
import {previewCron,type CronPreview,type CronPurpose} from '../api/scheduling';
import {errorText} from '../workflows/readiness';
const props=withDefaults(defineProps<{modelValue:string;purpose:CronPurpose;label?:string;disabled?:boolean}>(),{label:'Cron 表达式'}),emit=defineEmits<{'update:modelValue':[string];valid:[boolean]}>();
const mode=ref('manual'),interval=ref<number|null>(1),clock=ref('09:00'),weekdays=ref<number[]>([1]),monthdays=ref<number[]>([1]),year=ref('*'),error=ref(''),pending=ref(false),preview=ref<CronPreview|null>(null);
let lastGenerated='';
const modes=[['manual','手写表达式'],['seconds','每 N 秒'],['minutes','每 N 分钟'],['hours','每 N 小时'],['daily','每天'],['weekly','每周'],['monthly','每月']];
const generatedError=computed(()=>{
 if(mode.value==='manual')return '';
 if(['seconds','minutes','hours'].includes(mode.value)){const max=mode.value==='hours'?23:59;if(!Number.isInteger(interval.value)||interval.value!<1||interval.value!>max)return `间隔请输入1～${max}的整数`}
 else if(!/^([01]\d|2[0-3]):[0-5]\d$/.test(clock.value))return '时刻请输入有效的 HH:mm（00:00～23:59）';
 if(mode.value==='weekly'&&!weekdays.value.length)return '请至少选择一个星期';if(mode.value==='monthly'&&!monthdays.value.length)return '请至少选择一个日期';return '';
});
watch([mode,interval,clock,weekdays,monthdays,year],()=>{
 if(mode.value==='manual')return;
 if(generatedError.value){emit('valid',false);return}
 const [h,m]=clock.value.split(':').map(Number),n=interval.value;let fields:string[];
 switch(mode.value){case 'seconds':fields=[`*/${n}`,'*','*','*','*','*'];break;case 'minutes':fields=['0',`*/${n}`,'*','*','*','*'];break;case 'hours':fields=['0','0',`*/${n}`,'*','*','*'];break;default:fields=['0',String(m),String(h),mode.value==='monthly'?[...monthdays.value].sort((a,b)=>a-b).join(','):'*','*',mode.value==='weekly'?[...weekdays.value].sort((a,b)=>a-b).join(','):'*']}
 lastGenerated=[...fields,year.value.trim()||'*'].join(' ');emit('update:modelValue',lastGenerated);
},{deep:true});
watch(()=>props.modelValue,value=>{if(mode.value!=='manual'&&value!==lastGenerated)mode.value='manual'});
let timer:ReturnType<typeof setTimeout>|undefined,controller:AbortController|undefined,sequence=0;
watch(()=>[props.modelValue,props.purpose,generatedError.value],()=>{
 clearTimeout(timer);controller?.abort();const current=++sequence;preview.value=null;error.value='';pending.value=false;emit('valid',false);
 const text=props.modelValue?.trim()||'';
 if(generatedError.value){error.value=generatedError.value;return}
 if(new TextEncoder().encode(text).length>128||![5,6,7].includes(text.split(/\s+/).length)){error.value='请输入最多128字节的5、6或7字段 Cron 表达式';return}
 pending.value=true;timer=setTimeout(async()=>{controller=new AbortController();try{const value=await previewCron(text,props.purpose,controller.signal);if(current!==sequence)return;preview.value=value;emit('valid',!value.exhausted&&value.next.length>0)}catch(e){if(current===sequence)error.value=errorText(e)}finally{if(current===sequence)pending.value=false}},250);
},{immediate:true});
onBeforeUnmount(()=>{sequence++;clearTimeout(timer);controller?.abort()});
const format=(value:string)=>new Intl.DateTimeFormat('zh-CN',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hourCycle:'h23'}).format(new Date(value));
</script>
<template>
 <div class="cron-input" :aria-busy="pending">
  <div class="cron-fields">
   <label>生成方式<el-select v-model="mode" :disabled="disabled" :aria-label="label+'生成方式'"><el-option v-for="[value,text] in modes" :key="value" :value="value!" :label="text"/></el-select></label>
   <template v-if="mode!=='manual'">
    <label v-if="['seconds','minutes','hours'].includes(mode)">间隔<el-input-number v-model="interval" :controls="false" :min="1" :max="mode==='hours'?23:59" :step="1" step-strictly :disabled="disabled" :aria-label="label+'间隔'"/></label>
    <label v-else>执行时刻<el-input v-model="clock" placeholder="09:00" :disabled="disabled" :aria-label="label+'执行时刻'"/></label>
    <label v-if="mode==='weekly'">星期<el-select v-model="weekdays" multiple :disabled="disabled" :aria-label="label+'星期'"><el-option v-for="(name,n) in ['星期日','星期一','星期二','星期三','星期四','星期五','星期六']" :key="n" :value="n" :label="name"/></el-select></label>
    <label v-if="mode==='monthly'">日期<el-select v-model="monthdays" multiple :disabled="disabled" :aria-label="label+'日期'"><el-option v-for="n in 31" :key="n" :value="n" :label="n+'日'"/></el-select></label>
    <label>年份<el-input v-model="year" placeholder="* 或 2030-2040" :disabled="disabled" :aria-label="label+'年份'"/></label>
   </template>
  </div>
  <label class="cron-expression">{{label}}<el-input :model-value="modelValue" @update:model-value="emit('update:modelValue',String($event));mode='manual'" :aria-label="label" :disabled="disabled" :aria-invalid="!!error"/></label>
  <small>5位：分 时 日 月 周；6位增加秒；7位再增加年（1970～2199）。时区：上海。步长在相应时间字段内循环。</small>
  <p v-if="error" role="alert" class="cron-error">{{error}}</p>
  <div class="cron-preview" aria-live="polite">
   <p v-if="pending" role="status">正在校验并计算执行时间…</p>
   <template v-else-if="preview"><p v-if="preview.exhausted" role="status">计划已结束，没有后续执行时间</p><template v-else><p>接下来 {{preview.next.length}} 次执行时间</p><ol><li v-for="next in preview.next" :key="next">{{format(next)}}</li></ol></template><small v-if="preview.minimumIntervalSeconds">此业务最小间隔：{{preview.minimumIntervalSeconds}} 秒</small></template>
  </div>
 </div>
</template>
<style scoped>
.cron-input{display:grid;gap:10px;min-width:0}.cron-fields{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr));gap:12px}.cron-fields label,.cron-expression{display:grid;gap:6px;min-width:0}.cron-fields .el-input-number{width:100%}.cron-preview{min-height:150px;padding:12px;background:var(--mt-bg-subtle,var(--mt-bg-surface));border-radius:8px;font-size:13px}.cron-preview p{margin:0 0 8px}.cron-preview ol{margin:0;padding-left:24px;font-variant-numeric:tabular-nums}.cron-error{color:var(--el-color-danger);margin:0}.cron-input small{color:var(--mt-text-secondary);overflow-wrap:anywhere}
</style>
