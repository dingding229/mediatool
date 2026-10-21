<script setup lang="ts">
import{ref,onMounted}from'vue'
import ReadFeedback from './ReadFeedback.vue'
import {useReadState} from './read-state'
import{ElMessageBox}from'element-plus'
import 'element-plus/es/components/message-box/style/css'
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
 <el-alert title="账号绑定属于高风险操作。目前不支持通过外部账号登录，不影响本地管理员密码登录。" type="warning" :closable="false"/>
 <el-alert v-if="error" :title="error" type="error" :closable="false" role="alert"/><el-alert v-if="notice" :title="notice.includes('unsupported')?'已绑定；目前不支持通过外部账号登录，不会签发会话。':notice" type="success" :closable="false" role="status"/>
 <ReadFeedback :state="listRead" caption="认证状态" @retry="load"/><p v-if="listRead.loaded&&!rows.length">此插件不支持账号绑定</p><ul><li v-for="(r,index) in rows" :key="r.id"><strong>认证方式 {{index+1}}</strong> · {{({ready:'可用',unavailable:'不可用',unsupported:'不支持'} as Record<string,string>)[r.state]||'待核对'}} · 有效归属绑定 {{r.bound}} · 失效/需重绑 {{r.stale??0}}<p>{{r.error?'认证暂不可用，请在技术详情查看原因。':'无待处理问题'}}</p></li></ul>
 <p>绑定前请核对外部账号与本地账号。已有但无法确认归属的绑定不会算作本插件的有效绑定。</p>
 <label for="provider-id">认证方式</label><el-select id="provider-id" v-model="provider" aria-label="认证方式"><el-option v-for="(r,index) in rows" :key="r.id" :label="`认证方式 ${index+1}`" :value="r.id"/></el-select>
 <label for="provider-subject">外部账号标识</label><el-input id="provider-subject" v-model="subject" :maxlength="200" autocomplete="off"/>
 <label for="provider-account">本地账号编号（不是用户名或密码）</label><el-input id="provider-account" v-model="account" :maxlength="36" autocomplete="off"/>
 <el-button type="danger" plain :disabled="!provider||!subject||!account||busy||listRead.loading||!listRead.loaded" :loading="busy" @click="bind">绑定账号</el-button>
<details><summary>技术详情</summary><pre class="technical-json">{{rows}}</pre></details>
</template>
<style scoped>p,li{overflow-wrap:anywhere;line-height:1.6}label{display:block;margin:12px 0 6px}.el-select{width:100%}.el-button{min-height:44px;margin-top:16px}</style>
