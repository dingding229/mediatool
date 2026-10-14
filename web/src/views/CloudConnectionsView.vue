<script setup lang="ts">
import {ref,onMounted} from 'vue'
import {request} from '../api/client'
import ConnectionPanel from '../components/ConnectionPanel.vue'
interface Account{connectionId:string;info:{userId?:string;userName?:string;path:string;source:string;checkedAt:string;space:{total:number;used:number;free:number}|null;spaceError?:string;membershipError?:string;memberships:{identity:string;level:string;expiresAt?:string}[]};note:string}
const account=ref<Account|null>(null),busy=ref(false),error=ref('')
function bytes(n:number){if(!Number.isFinite(n))return '—';if(n===0)return '0 B';const unit=Math.min(5,Math.floor(Math.log(n)/Math.log(1024)));return `${(n/1024**unit).toFixed(2)} ${['B','KiB','MiB','GiB','TiB','PiB'][unit]}`}
async function load(){busy.value=true;error.value='';try{account.value=await request<Account>('/cloud/accounts/info')}catch(e){error.value=e instanceof Error?e.message:'空间 / 会员信息读取失败'}finally{busy.value=false}}
onMounted(load)
</script>
<template>
 <section class="page-heading"><div><span class="page-kicker">CLOUD / 连接与授权</span><h1>云盘连接</h1><p>在此维护CloudDrive2文件管理连接及115官方播放授权；云盘索引只处理扫描、识别和入库。</p></div><RouterLink to="/cloudStorage" class="inline-link">进入云盘索引</RouterLink></section>
 <section class="panel cloud-account"><div class="panel-title"><div><h2>115 空间与会员</h2><p class="field-hint">直接查询当前115授权账号的官方API，不使用CloudDrive2空间数据。</p></div><el-button :loading="busy" @click="load">刷新账户信息</el-button></div><el-alert v-if="error" :title="error" type="warning" :closable="false"/>
 <template v-if="account"><div class="quota-grid"><article><span>总空间</span><strong>{{account.info.space?bytes(account.info.space.total):'未返回'}}</strong></article><article><span>已使用</span><strong>{{account.info.space?bytes(account.info.space.used):'未返回'}}</strong></article><article><span>剩余空间</span><strong>{{account.info.space?bytes(account.info.space.free):'未返回'}}</strong></article></div><el-progress v-if="account.info.space&&account.info.space.total>0" :percentage="Math.min(100,Math.round(account.info.space.used/account.info.space.total*100))"/>
 <el-alert v-if="account.info.spaceError" :title="`空间信息：${account.info.spaceError}`" type="warning" :closable="false"/><div v-for="(member,index) in account.info.memberships" :key="index" class="membership-row"><strong>{{member.identity||'会员身份未返回'}} {{member.level}}</strong><span>到期时间：{{member.expiresAt?new Date(member.expiresAt).toLocaleString():'接口未返回（不推断为永久会员）'}}</span></div><p v-if="!account.info.memberships.length" class="field-hint">{{account.info.membershipError?`会员信息：${account.info.membershipError}`:'会员信息未返回'}}</p><p class="field-hint">账号 {{account.info.userName||account.info.userId||account.info.path}} · {{account.info.source}} · {{new Date(account.info.checkedAt).toLocaleString()}}</p><p class="field-hint">{{account.note}}</p></template>
 </section>
 <ConnectionPanel :allowed-kinds="['clouddrive2','open115']" title="云盘连接与115授权"/>
</template>
<style scoped>.cloud-account{margin-bottom:24px}.quota-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:16px;margin:20px 0}.quota-grid article{padding:18px;border:1px solid var(--border);border-radius:10px}.quota-grid span{display:block;color:var(--muted);font-size:13px}.quota-grid strong{display:block;font-size:24px;margin-top:12px}.membership-row{display:flex;flex-wrap:wrap;justify-content:space-between;gap:16px;padding:20px 0;border-bottom:1px solid var(--border)}@media(max-width:680px){.quota-grid{grid-template-columns:1fr}.panel-title{align-items:flex-start;gap:16px}}</style>
