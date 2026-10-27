<script setup lang="ts">
import {reactive,ref} from 'vue';
import {listIPBlocks,checkIPBlock,saveIPBlock,type IPBlockRule} from '../api/ip-blocklist';
import {APIError} from '../api/client';
import {errorText} from '../workflows/readiness';
const emit=defineEmits<{saved:[]}>(),visible=ref(false),confirm=ref(false),busy=ref(false),error=ref(''),unknown=ref(false),impact=ref(false),ack=ref(false),currentIp=ref(''),canonical=ref('');
const form=reactive({id:'',network:'',note:'',revision:0});
let generation=0;
async function open(network='',rule?:IPBlockRule,expectedRevision?:number){
 const ticket=++generation;Object.assign(form,{id:rule?.id||'',network:rule?.network||network,note:rule?.note||'',revision:0});error.value='';unknown.value=false;ack.value=false;confirm.value=false;visible.value=true;busy.value=true;
 try{const p=await listIPBlocks();if(ticket!==generation)return;form.revision=p.revision;currentIp.value=p.currentIp;if(rule&&expectedRevision!==undefined&&p.revision!==expectedRevision){form.revision=0;error.value='规则已变更，请关闭并刷新列表后重新编辑';return}if(!p.status.available)error.value=p.status.message||'IP 拉黑策略暂不可用，正在自动重试'}catch(e){if(ticket===generation)error.value=errorText(e)}finally{if(ticket===generation)busy.value=false}
}
async function prepare(){
 error.value='';if(!form.network.trim()){error.value='请填写 IP 或 CIDR 网段';return}if(Array.from(form.note.trim()).length>500){error.value='备注不能超过 500 个字符';return}
 busy.value=true;try{const p=await checkIPBlock(currentIp.value||'203.0.113.9',form.network.trim());canonical.value=p.candidateNetwork||form.network.trim();impact.value=p.affectsCurrent;ack.value=false;confirm.value=true}catch(e){error.value=errorText(e)}finally{busy.value=false}
}
async function save(){
 busy.value=true;error.value='';try{await saveIPBlock({network:canonical.value,note:form.note.trim(),revision:form.revision,confirm:true,confirmSelf:ack.value},form.id||undefined);confirm.value=false;visible.value=false;emit('saved')}
 catch(e){confirm.value=false;error.value=errorText(e);if(e instanceof APIError&&e.status===409){form.revision=0;error.value+='；请重新读取后核对再保存'}else if(!(e instanceof APIError)||e.status>=500){unknown.value=true;error.value+='；结果待核对，请重新读取列表，勿重复提交'}}finally{busy.value=false}
}
defineExpose({open});
</script>
<template>
 <el-dialog v-model="visible" :title="form.id?'编辑 IP 拉黑规则':'添加 IP 拉黑规则'" width="min(560px,94vw)" class="media-dialog" append-to-body :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy">
  <div class="ip-block-form">
   <label>IP 或 CIDR <el-input v-model="form.network" @input="error=''" aria-label="拉黑 IP 或 CIDR" maxlength="128" :disabled="busy||unknown" placeholder="例如 203.0.113.9 或 2001:db8::/32"/></label>
   <label>备注 <el-input v-model="form.note" @input="error=''" aria-label="IP 拉黑备注" type="textarea" maxlength="500" show-word-limit :disabled="busy||unknown"/></label>
   <p>规则对全部媒体用户生效，非仅当前用户；仅限制媒体服务 API 和 STRM 播放入口，不限制管理后台或健康检查。网段必须明确填写网络地址，不能使用 /0。</p>
   <el-alert v-if="error" :title="error" type="error" :closable="false"/>
  </div>
  <template #footer><el-button :disabled="busy" @click="visible=false">取消</el-button><el-button v-if="unknown||!form.revision" :disabled="busy" @click="visible=false;emit('saved')">关闭并核对列表</el-button><el-button v-else type="primary" :loading="busy" :disabled="unknown||!!error" @click="prepare">核对并继续</el-button></template>
 </el-dialog>
 <el-dialog v-model="confirm" title="确认 IP 拉黑范围" width="min(520px,94vw)" class="media-dialog" append-to-body :close-on-click-modal="false" :show-close="!busy" :close-on-press-escape="!busy">
  <p>将{{form.id?'修改':'添加'}}规则：<strong class="ip-block-network">{{canonical}}</strong></p>
  <p>全部媒体用户从此 IP 或网段访问媒体均返回 403；管理后台仍可删除规则。此操作不改账号、网盘文件或 STRM。</p>
  <el-alert v-if="impact" title="此规则覆盖当前管理员访问 IP，也会限制你从此地址访问媒体服务。" type="warning" :closable="false"/>
  <el-checkbox class="mt-form-checkbox" v-if="impact" v-model="ack">我已确认自身媒体访问将受限，仍要拉黑</el-checkbox>
  <template #footer><el-button :disabled="busy" @click="confirm=false">返回修改</el-button><el-button type="danger" :loading="busy" :disabled="impact&&!ack" @click="save">确认保存拉黑规则</el-button></template>
 </el-dialog>
</template>
<style scoped>.ip-block-form{display:grid;gap:16px}.ip-block-form label{display:grid;gap:8px}.ip-block-network{overflow-wrap:anywhere}.ip-block-form :deep(.el-input__wrapper){min-height:44px;box-sizing:border-box}</style>
