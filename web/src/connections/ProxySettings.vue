<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import {computed,onMounted,onUnmounted,reactive,ref} from 'vue';
import {getNetworkProxy,saveNetworkProxy,type ProxySettings} from '../api/networkProxy';
import {isConflict} from './model';
const saved=ref<ProxySettings|null>(null),conflict=ref<ProxySettings|null>(null);
const form=reactive({protocol:'http' as 'http'|'socks5',host:'',port:7890,username:'',password:'',tmdb:false,sites:false,clearPassword:false,clear:false});
const busy=ref(false),blocked=ref(false),error=ref(''),notice=ref('');let disposed=false;
const changedIdentity=computed(()=>!!saved.value&&['protocol','host','port','username'].some(k=>(form as any)[k]!==((saved.value as any)[k]||(k==='protocol'?'http':k==='port'?7890:''))));
const dirty=computed(()=>changedIdentity.value||!!form.password||form.clear||form.clearPassword||form.tmdb!==saved.value?.tmdb||form.sites!==saved.value?.sites);
const validation=computed(()=>{
 if(form.clear)return form.tmdb||form.sites?'清除代理前请关闭两个代理路由。':'';
 if(!form.host&&!saved.value?.configured&&!form.username&&!form.password&&!form.tmdb&&!form.sites)return '';
 if(!form.host||/[\s/@?#\\]/.test(form.host)||!Number.isInteger(form.port)||form.port<1||form.port>65535)return '请填写合法主机和 1–65535 的端口。';
 if(form.clearPassword&&form.password)return '清除原密码时请勿同时填写新密码。';
 if(!form.username&&form.password)return '填写密码时请同时填写用户名。';
 if(!form.password&&!form.clearPassword&&(saved.value?.needsInput||saved.value?.hasPassword&&changedIdentity.value))return '更换代理端点或用户名时，请重填密码或明确清除原密码。';
 return '';
});
function accept(out:ProxySettings){
 saved.value=out;Object.assign(form,{protocol:out.protocol||'http',host:out.host||'',port:out.port||7890,username:out.username||'',password:'',tmdb:out.tmdb,sites:out.sites,clearPassword:false,clear:false});
 conflict.value=null;blocked.value=false;error.value='';
}
async function load(preserve=false){
 try{const out=await getNetworkProxy();if(disposed)return;
  if(preserve&&saved.value&&dirty.value){conflict.value=out;notice.value='已刷新配置，你的输入已保留，请对照后确认。';return}accept(out);
 }catch{error.value='代理设置读取失败，请重新读取。'}
}
async function save(){
 if(!saved.value||busy.value||conflict.value||blocked.value||validation.value)return;
 busy.value=true;error.value='';notice.value='';
 try{const out=await saveNetworkProxy({...form,port:form.host||form.username||form.password?form.port:0,host:form.host.trim(),revision:saved.value.revision});if(!disposed){accept(out);notice.value='代理设置已保存，新的请求使用所选路由。'}}
 catch(e:any){
  if(disposed)return;
  if(isConflict(e)){try{conflict.value=await getNetworkProxy();error.value='配置已被修改，已重新读取。你的代理字段与路由选择已保留，请对照后确认。'}catch{error.value='配置冲突后读取失败，输入已保留。';blocked.value=true}}
  else if(e?.status>=400&&e.status<500)error.value=e.message;
  else{error.value='保存未确认，请重新读取核对；输入已保留。';blocked.value=true}
 }finally{busy.value=false}
}
function adopt(){if(conflict.value){saved.value=conflict.value;conflict.value=null;blocked.value=false;error.value='已采用最新修订，你的输入未变。'}}
onMounted(()=>load());onUnmounted(()=>{disposed=true;form.password='';saved.value=null;conflict.value=null});
usePageRefresh(()=>load(true));
</script>
<template>
<section class="connection-manager proxy-settings" aria-label="网络代理">
 <header class="connection-section-heading"><div><h2>网络代理</h2><p>代理失效时明确报错，不静默回退直连。</p></div></header>
 <div class="f5-proxy-form">
 <form v-if="saved" class="connection-editor" @submit.prevent="save">
  <el-alert v-if="saved.needsInput" title="代理配置需重新填写：旧格式或无法识别的配置已保留，已选择的代理请求不会回退直连。请重填全部字段，并填写或明确清除原密码。" type="warning" :closable="false"/>
  <p v-else>{{saved.configured?'代理已配置；已保存的密码不会回传，留空保留。':'尚未设置代理。'}}</p>
  <div class="proxy-fields">
   <label>协议<el-select v-model="form.protocol" aria-label="代理协议" :disabled="busy||form.clear"><el-option label="HTTP" value="http"/><el-option label="SOCKS5" value="socks5"/></el-select></label>
   <label>IP 或主机<el-input v-model="form.host" aria-label="代理主机" :disabled="busy||form.clear" placeholder="IP 或主机名，不含协议和路径"/></label>
   <label>端口<el-input-number v-model="form.port" aria-label="代理端口" :min="1" :max="65535" :controls="false" :disabled="busy||form.clear"/></label>
   <label>用户名<el-input v-model="form.username" aria-label="代理用户名" autocomplete="off" :disabled="busy||form.clear"/></label>
   <div class="connection-field proxy-password"><span>密码</span><el-input v-model="form.password" type="password" show-password aria-label="代理密码" autocomplete="new-password" :disabled="busy||form.clear||form.clearPassword" placeholder="留空保留原密码"/>
   </div>
  </div>
  <div class="proxy-options">
   <el-checkbox v-if="saved.hasPassword||saved.needsInput" v-model="form.clearPassword" class="mt-form-checkbox" :disabled="busy||form.clear" @change="form.clearPassword&&(form.password='')">明确清除已保存的代理密码</el-checkbox>
   <el-checkbox v-model="form.clear" class="mt-form-checkbox" :disabled="busy">明确清除已保存的代理地址</el-checkbox>
   <el-checkbox v-model="form.tmdb" class="mt-form-checkbox" :disabled="busy">TMDB查询与海报使用代理</el-checkbox>
   <el-checkbox v-model="form.sites" class="mt-form-checkbox" :disabled="busy">PT站点搜索、RSS与站点请求使用代理</el-checkbox>
   <p class="mt-form-checkbox-help">不会影响 115、CloudDrive2 或下载器。关闭两个路由即可停用代理。</p>
  </div>
  <el-alert v-if="conflict" :title="`服务端最新路由：TMDB ${conflict.tmdb?'代理':'直连'}；PT ${conflict.sites?'代理':'直连'}。代理${conflict.configured?'已设置':'未设置'}。`" type="info" :closable="false"/>
  <el-button v-if="conflict" @click="adopt">保留输入并采用最新修订</el-button>
  <p v-if="validation" role="status">{{validation}}</p>
  <div><el-button type="primary" native-type="submit" :loading="busy" :disabled="!!conflict||blocked||!!validation" class="mt-save-button">保存代理设置</el-button></div>
  <p v-if="notice" role="status">{{notice}}</p>
 </form>
 <el-alert v-if="error" :title="error" type="warning" :closable="false"/>
 </div>
</section>
</template>
<style scoped>
.proxy-fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.proxy-password{grid-column:1/-1}.proxy-fields .el-input-number{width:100%}.proxy-options{display:grid;gap:0}.proxy-settings .proxy-options>p{margin-block:0 8px;font-size:12px;line-height:1.7;color:var(--mt-text-muted)}.proxy-fields .el-select{width:100%}
@media(max-width:600px){.proxy-fields{grid-template-columns:minmax(0,1fr)}}
</style>
