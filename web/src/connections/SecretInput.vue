<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {View,Hide,CopyDocument} from '@element-plus/icons-vue';
const props=withDefaults(defineProps<{id:string;label:string;modelValue?:string;hasSaved?:boolean;savedValue?:string;busy?:boolean;active?:boolean}>(),{modelValue:'',hasSaved:false,busy:false,active:true});
const emit=defineEmits<{'update:modelValue':[string];show:[];hide:[];copy:[]}>();
const draftVisible=ref(false),revealed=computed(()=>props.savedValue!==undefined),showing=computed(()=>revealed.value||draftVisible.value);
function toggle(){if(revealed.value)emit('hide');else if(props.modelValue||!props.hasSaved)draftVisible.value=!draftVisible.value;else emit('show')}
watch(()=>[props.active,props.label,props.modelValue],()=>{if(!props.active||!props.modelValue)draftVisible.value=false});
</script>
<template>
 <el-input :id="id" :model-value="revealed?savedValue:modelValue" :type="showing?'text':'password'" :readonly="revealed" :aria-label="revealed?label+'已保存值':label" autocomplete="new-password" :placeholder="hasSaved?'留空保留；填写新值替换':'未设置，可填写'" @update:model-value="!revealed&&emit('update:modelValue',$event)">
  <template v-if="hasSaved" #suffix><button v-if="hasSaved" type="button" class="secret-input-action" :aria-label="(showing?'隐藏':'显示')+label" :title="(showing?'隐藏':'显示')+label" :disabled="busy" :aria-pressed="showing" @click="toggle"><el-icon><Hide v-if="showing"/><View v-else/></el-icon></button><button v-if="revealed" type="button" class="secret-input-action" :aria-label="'复制'+label" :title="'复制'+label" @click="emit('copy')"><el-icon><CopyDocument/></el-icon></button></template>
 </el-input>
 <small v-if="revealed">已保存值只读展示，不作为替换提交；隐藏后可填写新值。</small>
</template>
<style scoped>.secret-input-action{display:inline-flex;justify-content:center;align-items:center;width:44px;height:44px;min-width:44px;min-height:44px;padding:10px;margin-block:-1px;border:0;background:transparent;color:var(--mt-text-muted);cursor:pointer}.secret-input-action:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:-2px;border-radius:var(--radius-sm)}.secret-input-action:disabled{cursor:wait;opacity:.5}.secret-input-action .el-icon{font-size:16px}small{font-size:12px;color:var(--mt-text-muted)}</style>
