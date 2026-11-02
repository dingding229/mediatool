<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {decodeDuration,durationSeconds,durationUnits,type DurationUnit} from './cache-duration';
const props=defineProps<{modelValue:number|string;label:string;id:string;max:number}>();
const emit=defineEmits<{'update:modelValue':[number|string];valid:[boolean]}>();
const unit=ref<DurationUnit>('minutes'),value=ref(''),emitted=ref<number|string>();
watch(()=>props.modelValue,v=>{if(v===emitted.value){emitted.value=undefined;return}const decoded=decodeDuration(Number(v)||0);unit.value=decoded.unit;value.value=decoded.value},{immediate:true});
const seconds=computed(()=>durationSeconds(value.value,unit.value,props.max));
watch(seconds,v=>emit('valid',v!==null),{immediate:true});
function input(v:string){value.value=v;const n=durationSeconds(v,unit.value,props.max);emit('valid',n!==null);const next=n===null?v:n;emitted.value=next;emit('update:modelValue',next)}
function changeUnit(next:DurationUnit){const current=seconds.value;unit.value=next;if(current!==null)value.value=String(current/durationUnits.find(([key])=>key===next)![2]);input(value.value)}
</script>
<template>
 <div class="settings-field cache-duration-field"><label :for="id">{{label}}</label><div class="cache-duration-controls"><el-input :id="id" :model-value="value" :aria-label="label" inputmode="decimal" :aria-invalid="seconds===null" @update:model-value="input($event)"/><el-select :model-value="unit" :aria-label="label+'单位'" @update:model-value="changeUnit"><el-option v-for="[key,name] in durationUnits" :key="key" :value="key" :label="name"/></el-select></div><small v-if="seconds===null" role="alert">{{label}}：请输入有效时长，需能换算为整数秒且不超过 {{max/86400}} 天。</small></div>
</template>
<style scoped>.cache-duration-controls{display:grid;grid-template-columns:minmax(0,1fr) 96px;gap:var(--mt-form-field-gap);min-width:0}.cache-duration-controls :deep(.el-select){min-width:0}</style>
