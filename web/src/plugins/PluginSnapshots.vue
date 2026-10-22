<script setup lang="ts">
import {formatBytes} from '../utils/format-bytes'
import { onMounted,ref } from 'vue'
import ReadFeedback from './ReadFeedback.vue'
import {useReadState} from './read-state'
import { ElMessageBox } from 'element-plus'
import 'element-plus/es/components/message-box/style/css'
import { inspect,stateSnapshots,stateOperation } from '../api/plugins'
import type {StateSnapshot} from '../api/plugins'
import {requestUUID} from '../utils/request-uuid'
const emit=defineEmits<{changed:[]}>()
const props=defineProps<{instanceId:string}>()
const entries=ref<StateSnapshot[]>([]),busy=ref(false),error=ref(''),notice=ref('')
const pending=new Map<string,Parameters<typeof stateOperation>[1]>()
const listRead=useReadState()
async function load(){await listRead.run(()=>stateSnapshots(props.instanceId),value=>entries.value=value)}
onMounted(load)
async function operate(kind:'create'|'restore'|'delete',snapshotId=''){
 if(busy.value)return
 try{await ElMessageBox.confirm(kind==='restore'?'仅恢复此实例配置、KV 和 data，覆盖当前实例数据。不会撤销业务副作用、回退代码或启用插件。':kind==='delete'?'永久删除此快照，不能恢复。':'保存此实例配置、KV 和 data；实例必须已停用且进程已退出。','确认实例快照操作',{type:'warning',confirmButtonText:'确认操作',cancelButtonText:'取消',customClass:'plugin-confirm-dialog'})}catch{return}
 busy.value=true;error.value='';notice.value=''
 try{const identity=kind+':'+snapshotId;let input=pending.get(identity)
 if(!input){const info=await inspect(props.instanceId);if(info.instance.desiredState!=='disabled'||!['stopped','needs_action'].includes(info.instance.state))throw new Error('请先停用实例并等待进程退出');input={kind,snapshotId,expectedRevision:info.instance.revision,fingerprint:info.fingerprint,confirm:true,operationKey:requestUUID()};pending.set(identity,input)}
 await stateOperation(props.instanceId,input);pending.delete(identity);emit('changed');notice.value='操作已完成；实例保持停用。';await load()
 }catch(e){error.value=e instanceof Error?e.message:'操作结果未知；请刷新核对，不要更换标识盲目重试'}finally{busy.value=false}}
</script>
<template>
 <p>只恢复本实例数据，不回退代码、不撤销业务副作用。创建和恢复前须完全停用。单份文件≤{{formatBytes(16*1024**2)}}、data≤{{formatBytes(64*1024**2)}}；单实例≤5份/{{formatBytes(256*1024**2)}}，全局≤64份/{{formatBytes(1024**3)}}（含密文）。不收录可执行文件或链接。超时先刷新核对；同一窗口重试沿用原操作标识。</p>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><el-alert v-if="notice" :title="notice" type="success" :closable="false" role="status"/>
 <ReadFeedback :state="listRead" caption="实例快照" @retry="load"/>
 <el-button :disabled="!listRead.loaded||listRead.loading" :loading="busy" @click="operate('create')">创建实例快照</el-button>
 <el-table v-if="listRead.loaded" :aria-busy="listRead.loading" :data="entries" empty-text="尚无实例快照"><el-table-column prop="createdAt" label="创建时间" min-width="170"/><el-table-column prop="state" label="状态" width="95"/><el-table-column label="数据格式" width="110"><template #default="{row}">{{row.configSchema}} / {{row.dataSchema}}</template></el-table-column><el-table-column label="容量" width="130"><template #default="{row}">{{formatBytes(row.bytes)}}</template></el-table-column><el-table-column label="操作" min-width="180"><template #default="{row}"><el-button :disabled="busy||row.state!=='ready'" @click="operate('restore',row.id)">恢复</el-button><el-button type="danger" plain :disabled="busy||row.state!=='ready'" @click="operate('delete',row.id)">删除</el-button></template></el-table-column></el-table>
</template>
<style scoped>p{line-height:1.7;overflow-wrap:anywhere}.el-button{min-height:44px}.el-table{margin-top:16px}</style>
