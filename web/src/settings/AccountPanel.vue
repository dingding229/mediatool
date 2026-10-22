<script setup lang="ts">
import {usePageRefresh} from '../ui/page-refresh';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { request } from '../api/client';
import { useAuthStore } from '../stores/auth';
import { useResource } from '../workflows/useResource';
import AdministratorSessions from './AdministratorSessions.vue';
import PasswordForm from '../auth/PasswordForm.vue';
import StateView from '../ui/StateView.vue';
import { readGuideHidden, writeGuideHidden } from '../workflows/readiness';
import { stateFor, messageFor } from './revision';
const auth = useAuthStore(), router = useRouter(), session = useResource(() => request<{
  authenticated: boolean;
  account: {
    username: string;
    mustChangePassword?: boolean;
  };
}>('/auth/session')), hidden = ref(readGuideHidden()), busy = ref(false), error = ref('');
function setGuide(show: boolean) {
  writeGuideHidden(!show);
  hidden.value = !show;
  if (show)
    window.dispatchEvent(new Event('media-tool:show-setup'));
}
function reopen() {
  setGuide(true);
  void router.push('/ops?tab=attention');
}
async function logout() {
  busy.value = true;
  try {
    await auth.logout();
    await router.replace('/login');
  }
  catch (e) {
    error.value = messageFor(e);
  }
  finally {
    busy.value = false;
  }
}
onMounted(session.load);
usePageRefresh(session.load, 'security');
</script>
<template>
  <div class="settings-stack">
    <section class="settings-card" aria-label="当前登录会话">
      <header>
        <div>
          <h2>当前登录会话</h2>
          <p>账号安全从妥善保管密码开始。</p>
        </div>

      </header>
      <StateView v-if="session.error.value" :state="stateFor(session.error.value)" :description="messageFor(session.error.value)" @retry="session.load"/>
      <StateView v-else-if="session.loading.value" state="loading"/>
      <template v-else-if="session.data.value">
        <p>管理员：{{session.data.value.account.username||'—'}}</p>
        <p>{{session.data.value.authenticated?'已登录':'未登录'}} · {{session.data.value.account.mustChangePassword?'需要修改密码':'密码可正常使用'}}</p>
        <p>下方可查看并撤销指定的管理端登录；修改密码会撤销该账号所有会话。</p>
        <el-button :loading="busy" @click="logout">退出当前会话</el-button>
        <p v-if="error" role="alert">{{error}}</p>
      </template>
    </section>
    <AdministratorSessions/>
    <section class="settings-card" aria-label="修改管理员密码">
      <h2>修改密码</h2>
      <PasswordForm/>
    </section>
    <section class="settings-card" aria-label="首页配置引导">
      <h2>首次使用引导</h2>
      <p>{{hidden?'引导已收起。':'引导可在首页查看。'}}重新查看云盘、播放服务、站点与分类的配置进度。</p>
      <el-switch :model-value="!hidden" aria-label="在首页显示配置引导" @update:model-value="(v:unknown)=>setGuide(Boolean(v))"/>
      <el-button @click="reopen">重新打开配置引导</el-button>
    </section>
  </div>
</template>
