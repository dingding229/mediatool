<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { post } from '../api/client'
interface Authorization { sessionId:string; state:string; qrCodeImage?:string; expiresAt:string }
const props=defineProps<{ modelValue:boolean; integrationId:string }>()
const emit=defineEmits<{ 'update:modelValue':[boolean]; authorized:[] }>()
const session=ref<Authorization|null>(null), busy=ref(false), error=ref(''), status=ref('')
let generation=0, timer:ReturnType<typeof setTimeout>|undefined
const labels:Record<string,string>={waiting:'等待115 App扫码',scanned:'已扫码，请在115 App中确认授权',exchanging:'已确认，正在加密保存授权结果',authorized:'授权已加密保存并自动验证，请显式启用官方直链',expired:'二维码已过期，请重新生成',cancelled:'已取消授权',failed:'授权未完成，请重新生成二维码'}
function message(e:unknown){return e instanceof Error?e.message:'115官方授权暂不可用'}
async function poll(id:string, version:number){
 if(version!==generation || !props.modelValue)return
 try{
  const next=await post<Authorization>(`/integrations/${id}/115/auth/poll`,{sessionId:session.value?.sessionId})
  if(version!==generation)return
  session.value={...session.value,...next}
  if(next.state!=='waiting'&&next.state!=='scanned')session.value.qrCodeImage=undefined
  status.value=labels[next.state]??'等待授权'
  if(next.state==='authorized'){emit('authorized');return}
  if(next.state==='waiting'||next.state==='scanned'||next.state==='exchanging')timer=setTimeout(()=>void poll(id,version),1500)
 }catch(e){if(version===generation){error.value=message(e);status.value='轮询已停止；可重新生成二维码'}}
}
async function start(){
 const version=++generation;if(timer)clearTimeout(timer)
 busy.value=true;error.value='';status.value='正在请求115官方授权二维码';session.value=null
 try{
  const next=await post<Authorization>(`/integrations/${props.integrationId}/115/auth/start`,{})
  if(version!==generation){void cancel(next.sessionId,props.integrationId);return}
  session.value=next;status.value=labels[next.state]??'等待扫码';void poll(props.integrationId,version)
 }catch(e){if(version===generation){error.value=message(e);status.value='请求失败'}}
 finally{if(version===generation)busy.value=false}
}
async function cancel(sessionId:string,id:string){try{await post(`/integrations/${id}/115/auth/cancel`,{sessionId})}catch{/* Expired/replaced session or logout: local polling must still stop. */}}
function close(){
 generation++;if(timer)clearTimeout(timer)
 const old=session.value;session.value=null;busy.value=false;status.value=''
 if(old&&(old.state==='waiting'||old.state==='scanned'||old.state==='exchanging'))void cancel(old.sessionId,props.integrationId)
 emit('update:modelValue',false)
}
watch(()=>props.modelValue,open=>{if(open)void start();else{generation++;if(timer)clearTimeout(timer);session.value=null;busy.value=false}})
onUnmounted(close)
</script>
<template>
 <el-dialog :model-value="modelValue" title="115 官方扫码授权" width="min(520px, 92vw)" :close-on-click-modal="false" @close="close">
  <el-alert title="使用你自己应用的 AppID；不要借用 CloudDrive2 或他人的应用凭据。授权不会自动启用，也不会修改已有网盘账号。" type="info" :closable="false"/>
  <div class="authorization-content" v-loading="busy">
   <img v-if="session?.qrCodeImage" :src="session.qrCodeImage" width="256" height="256" alt="115官方扫码授权二维码" class="authorization-qr"/>
   <p role="status" aria-live="polite">{{status}}</p>
   <p v-if="session" class="field-hint">有效期至 {{new Date(session.expiresAt).toLocaleTimeString()}}。请在115 App中核对应用名称和授权范围后确认。</p>
   <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  </div>
  <template #footer><el-button @click="close">关闭</el-button><el-button type="primary" :loading="busy" @click="start">重新生成二维码</el-button></template>
 </el-dialog>
</template>
<style scoped>
.authorization-content{display:flex;flex-direction:column;align-items:center;gap:12px;padding:20px 0;min-height:160px;text-align:center}
.authorization-qr{max-width:100%;height:auto;background:#fff;padding:8px;border-radius:8px}
</style>
