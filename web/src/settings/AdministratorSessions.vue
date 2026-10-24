<script setup lang="ts">
import IpAddress from '../ui/IpAddress.vue';
import {beijingTime as date} from '../presentation/time';
import {usePageNumber} from '../ui/page-query';

import {onMounted,onUnmounted,ref} from 'vue';
import {useRouter} from 'vue-router';
import {request,post} from '../api/client';
import {useAuthStore} from '../stores/auth';
import {useResource} from '../workflows/useResource';
import {usePageRefresh} from '../ui/page-refresh';
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import {messageFor,stateFor} from './revision';
interface Session {id:string;accountId:string;username:string;createdAt:string;lastActiveAt:string;expiresAt:string;ip:string;userAgent:string;current:boolean}
const page = usePageNumber(),sessions=useResource(()=>request<{items:Session[];hasNext:boolean}>('/auth/sessions?page='+page.value),{items:[],hasNext:false}),target=ref<Session>(),busy=ref(false),error=ref<unknown>(null),notice=ref(''),router=useRouter(),auth=useAuthStore();
let alive=true;onUnmounted(()=>{alive=false});
async function revoke(){if(!target.value||busy.value)return;busy.value=true;error.value=null;const row=target.value;try{const out=await post<{revoked:boolean;loginRequired:boolean}>(`/auth/sessions/${row.id}/revoke`,{confirm:true});if(!alive)return;target.value=undefined;if(out.loginRequired){auth.forget();await router.replace('/login?sessionRevoked=1');return}notice.value='已撤销指定管理端登录；该会话的下一次请求需要重新登录。';await sessions.load()}catch(e){error.value=e;target.value=undefined;await sessions.load()}finally{busy.value=false}}
onMounted(sessions.load);usePageRefresh(sessions.load,'security');
</script>
<template>
<section class="settings-card" aria-label="管理端登录会话">
<h2>管理端登录会话</h2><p>显示有效的管理端登录，不包含媒体服务器用户登录。IP 为连接地址，播放器标识（UA）按原值显示。</p>
<p v-if="notice" role="status">{{notice}}</p>
<StateView v-if="error" :state="stateFor(error)" :description="messageFor(error)" retry-text="核对会话" @retry="sessions.load"/>
<StateView v-if="sessions.loading.value" state="loading"/>
<StateView v-else-if="sessions.error.value" :state="stateFor(sessions.error.value)" :description="messageFor(sessions.error.value)" @retry="sessions.load"/>
<StateView v-else-if="!sessions.data.value?.items.length" state="empty" title="暂无有效的管理端登录"/>
<article v-else v-for="row in sessions.data.value.items" :key="row.id" class="organize-record">
<h3>{{row.username}}<span v-if="row.current"> · 当前会话</span></h3>
<p>创建 {{date(row.createdAt)}} · 最后活动 {{date(row.lastActiveAt)}} · 到期 {{date(row.expiresAt)}}</p>
<p class="admin-session-raw">IP：<IpAddress :ip="row.ip"/></p><p class="admin-session-raw">UA：{{row.userAgent||'—'}}</p>
<el-button :disabled="busy" @click="target=row">{{row.current?'撤销当前登录':'撤销此登录'}}</el-button>
</article>
<PagePagination v-model:current-page="page" :has-next="sessions.data.value.hasNext" :disabled="sessions.loading.value" @current-change="sessions.load"/>
<ConfirmAction :visible="!!target" @update:visible="v=>{if(!v)target=undefined}" :title="target?.current?'撤销当前管理端登录':'撤销指定管理端登录'" :description="target?.current?'确认后将退出当前管理端并跳转到登录页。':'仅撤销所选登录，不影响其他管理端登录。'" impact-scope="不会撤销媒体服务器用户登录，也不会修改账号密码。撤销立即对下一次请求生效。" confirm-text="确认撤销登录" :loading="busy" @confirm="revoke"/>
</section>
</template>
<style scoped>.admin-session-raw{white-space:pre-wrap;overflow-wrap:anywhere}</style>
