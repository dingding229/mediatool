<script setup lang="ts">
import {computed,onUnmounted,ref} from 'vue';
import {request,post} from '../api/client';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import StateView from '../ui/StateView.vue';
import {errorText} from './readiness';
const props=defineProps<{kind:string;id:string;state:string}>(),emit=defineEmits<{changed:[]}>();
const action=ref(''),snapshot=ref<any>(),busy=ref(false),error=ref<unknown>(null),notice=ref(''),phase=ref<'queued'|'reconciling'|null>(null);
let alive=true;onUnmounted(()=>{alive=false});
const liveState=computed(()=>{const s=snapshot.value?.state||props.state;return ['uncertain','needs_reconcile'].includes(s)?'unknown':s});
async function read(){error.value=null;try{snapshot.value=await request(`/tasks/${props.kind}/${props.id}`);if(!snapshot.value.queued&&!snapshot.value.stopPending&&snapshot.value.state!=='checking')phase.value=null;notice.value=snapshot.value.reason;emit('changed')}catch(e){error.value=e}}
async function ask(value:string){if(busy.value||phase.value)return;busy.value=true;await read();busy.value=false;if(!error.value)action.value=value}
async function apply(){if(!action.value||busy.value)return;busy.value=true;error.value=null;phase.value='reconciling';const verb=action.value;action.value='';try{const out=await post<any>(`/tasks/${props.kind}/${props.id}/${verb}`,{confirm:true,revision:snapshot.value.revision});if(!alive)return;snapshot.value=out;notice.value=out.reason;if(out.state==='unknown')error.value=Error(out.reason);phase.value=out.queued||out.stopPending?'queued':null;emit('changed')}catch(e){error.value=e}finally{busy.value=false}}
</script>
<template><div class="task-controls">
<el-button v-if="['pending','retry','running','paused','failed','unknown','checking'].includes(liveState)" :disabled="busy||!!phase" @click="ask('cancel')">取消后台任务</el-button>
<el-button v-if="['failed','paused','unknown','cancelled','checking'].includes(liveState)" :disabled="busy||!!phase" @click="ask('retry')">核对后重试</el-button>
<p v-if="notice&&!error" role="status">{{notice}}</p><StateView v-if="error" state="error" title="任务状态或操作未确认" :description="errorText(error)" retry-text="核对任务状态" @retry="read"/>
<AsyncResultBanner v-if="phase" :status="phase" message="只表示请求已受理或等待核对，不代表任务完成；不能据此重复执行。" @refresh="read"/>
<ConfirmAction :visible="!!action" @update:visible="v=>{if(!v)action=''}" :title="action==='cancel'?'取消后台任务':'核对外部状态后重试'" description="仅处理当前任务；结果未知时先核对，不会盲目重放。" impact-scope="取消停止本程序后续执行，不保证撤销已发生的外部操作；核对失败将保留未知状态并显示原因。" confirm-text="确认处理当前任务" :loading="busy" @confirm="apply"/>
</div></template>
<style scoped>.task-controls{display:flex;flex-wrap:wrap;gap:var(--mt-space-2,8px);align-items:center}.task-controls p{flex-basis:100%;overflow-wrap:anywhere}</style>
