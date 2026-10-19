<script setup lang="ts">
import{ref,onMounted}from'vue'
import ReadFeedback from './ReadFeedback.vue'
import {useReadState} from './read-state'
import{ElMessageBox}from'element-plus'
import{authProviders,bindProvider,inspect}from'../api/plugins'
import type{AuthProviderStatus}from'../api/plugins'
const props=defineProps<{instanceId:string}>(),rows=ref<AuthProviderStatus[]>([]),provider=ref(''),subject=ref(''),account=ref(''),busy=ref(false),error=ref(''),notice=ref('')
const listRead=useReadState()
async function load(){await listRead.run(()=>authProviders(props.instanceId),value=>{rows.value=value;provider.value=provider.value||value[0]?.id||''})}
onMounted(load)
async function bind(){if(busy.value||!provider.value)return
 try{await ElMessageBox.confirm('将此外部subject绑定到预先存在的本地账号？不签发会话、不授予管理员权限，不替代本地密码登录。','确认认证绑定',{type:'warning',customClass:'plugin-confirm-dialog',confirmButtonText:'确认绑定',cancelButtonText:'取消'})}catch{return}
 busy.value=true;error.value='';notice.value='';try{const info=await inspect(props.instanceId);await bindProvider({instanceId:props.instanceId,providerId:provider.value,subject:subject.value,accountId:account.value,fingerprint:info.fingerprint,confirm:true});subject.value='';notice.value='已绑定；真实登录适配器仍为 unsupported，不会签发会话';await load()}catch(e){error.value=String(e)}finally{busy.value=false}}
</script>
<template>
 <el-alert title="auth.provider 高危权限：仅状态/绑定框架。真实登录适配器仍为 unsupported，本地管理员密码登录不受影响。" type="warning" :closable="false"/>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><el-alert v-if="notice" :title="notice" type="success" :closable="false" role="status"/>
 <ReadFeedback :state="listRead" caption="认证状态" @retry="load"/><p v-if="listRead.loaded&&!rows.length">此定义没有认证 provider</p><ul><li v-for="r in rows" :key="r.id"><strong>{{r.id}}</strong> · {{r.state}} · 有效归属绑定 {{r.bound}} · 失效/需重绑 {{r.stale??0}}<p>状态/错误码：{{r.error||'无'}}</p></li></ul>
 <p>不显示外部subject、令牌或认证断言。历史上未记录实例归属的绑定不计为本实例已绑定。</p>
 <label for="provider-id">Provider</label><el-select id="provider-id" v-model="provider" aria-label="认证Provider"><el-option v-for="r in rows" :key="r.id" :label="r.id" :value="r.id"/></el-select>
 <label for="provider-subject">外部subject</label><el-input id="provider-subject" v-model="subject" :maxlength="200" autocomplete="off"/>
 <label for="provider-account">预先存在的本地账号UUID（非用户名或密码）</label><el-input id="provider-account" v-model="account" :maxlength="36" autocomplete="off"/>
 <el-button type="danger" plain :disabled="!provider||!subject||!account||busy||listRead.loading||!listRead.loaded" :loading="busy" @click="bind">绑定账号</el-button>
</template>
<style scoped>p,li{overflow-wrap:anywhere;line-height:1.6}label{display:block;margin:12px 0 6px}.el-select{width:100%}.el-button{min-height:44px;margin-top:16px}</style>
