<script setup lang="ts">
import {pendingRestore} from '../workflows/restore-session';
import { ref, onMounted, onUnmounted } from 'vue';
import { useRouter, useRoute } from 'vue-router';
import { request, APIError } from '../api/client';
import { PASSWORD_MAX_LENGTH, EXISTING_PASSWORD_MAX_LENGTH, passwordPolicyError } from '../auth/password-policy';
import { useAuthStore } from '../stores/auth';
import AuthShell from '../auth/AuthShell.vue';
let alive = true;
const username = ref(''), password = ref(''), confirmation = ref(''), loading = ref(false), checking = ref(true), error = ref(''), registrationOpen = ref(false), available = ref(false), router = useRouter(), auth = useAuthStore();
const loginRoute = useRoute();
async function loadSetup() {
  checking.value = true;
  error.value = '';
  try {
    const setup = await request<{
      registrationOpen: boolean;
    }>('/auth/setup');
    if (!alive)
      return;
    registrationOpen.value = setup.registrationOpen;
    available.value = true;
  }
  catch (e) {
    available.value = false;
    error.value = e instanceof Error ? e.message : '无法检查账号状态，请检查数据库连接';
  }
  finally {
    checking.value = false;
  }
}
async function submit() {
  if (loading.value || checking.value || !available.value)
    return;
  if (!username.value.trim() || !password.value) {
    error.value = '请输入管理员账号和密码';
    return;
  }
  ;
  if (registrationOpen.value) {
    if (!/^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$/.test(username.value.trim())) {
      error.value = '账号需3–32位英文字母、数字、下划线、短横线或点';
      return;
    }
    ;
    const passwordError = passwordPolicyError(password.value);
    if (passwordError) {
      error.value = passwordError;
      return;
    }
    ;
    if (password.value !== confirmation.value) {
      error.value = '两次输入的密码不一致';
      return;
    }
  }
  loading.value = true;
  error.value = '';
  try {
    if (registrationOpen.value)
      await auth.register(username.value.trim(), password.value, confirmation.value);
    else
      await auth.login(username.value.trim(), password.value);
    password.value = '';
    confirmation.value = '';
    if (!alive)
      return;
    await router.push(auth.account?.mustChangePassword ? '/reset-password' : pendingRestore()?{path:'/ops',query:{tab:'maintenance',restore:pendingRestore()}}:'/dashboard');
  }
  catch (e) {
    error.value = e instanceof Error ? e.message : '登录或注册失败';
    if (e instanceof APIError && e.code === 'registration_closed') {
      registrationOpen.value = false;
      password.value = '';
      confirmation.value = '';
    }
  }
  finally {
    loading.value = false;
  }
}
onMounted(loadSetup);
onUnmounted(() => {
  alive = false;
  password.value = '';
  confirmation.value = '';
});
</script>
<template>
  <AuthShell :title="registrationOpen?'注册管理员账号':'进入管理工作台'" :description="registrationOpen?'创建唯一管理员账号。完成注册后，将自动关闭公开注册。':'使用你的管理员账号和密码登录。'">
    <p v-if="pendingRestore()||loginRoute.query.restore" role="status">恢复已切换账号与会话，当前登录可能已失效。请使用备份时的管理员账号和密码登录；恢复可能已经完成，登录后在运维页查看持久化结果，不要重复执行。</p>
    <p v-if="loginRoute.query.sessionRevoked==='1'" role="status">当前管理端登录已撤销，请重新登录。</p>
    <el-alert v-if="error" :title="error" type="error" :closable="false"/>
    <p v-if="checking" role="status">正在检查账号状态…</p>
    <form @submit.prevent="submit">
      <div>
        <label for="admin-username">管理员账号</label>
        <el-input id="admin-username" v-model="username" autocomplete="username" autocapitalize="none" spellcheck="false" :maxlength="32" :disabled="loading||checking"/>
        <p v-if="registrationOpen" class="field-hint">3–32 位英文字母、数字、下划线、短横线或点。</p>
      </div>
      <div>
        <label for="admin-password">密码</label>
        <el-input id="admin-password" v-model="password" type="password" show-password :autocomplete="registrationOpen?'new-password':'current-password'" :maxlength="registrationOpen?PASSWORD_MAX_LENGTH:EXISTING_PASSWORD_MAX_LENGTH" :disabled="loading||checking"/>
        <p v-if="registrationOpen" class="field-hint">6–20 个字符，支持中文与空格，不会去除首尾空格。</p>
      </div>
      <div v-if="registrationOpen">
        <label for="admin-password-confirm">确认密码</label>
        <el-input id="admin-password-confirm" v-model="confirmation" type="password" show-password autocomplete="new-password" :maxlength="PASSWORD_MAX_LENGTH" :disabled="loading||checking"/>
      </div>
      <el-button type="primary" native-type="submit" :loading="loading" :disabled="checking||!available">{{registrationOpen?'注册并进入工作台':'进入工作台'}}</el-button>
    </form>
    <el-button v-if="!available&&!checking" @click="loadSetup">重新检查账号状态</el-button>
    <p class="auth-note">{{registrationOpen?'请先在可信环境中完成注册，不要将未初始化的管理端暴露到公网。':'密码不会写入浏览器存储。退出当前账号或修改密码后，需要重新登录。'}}</p>
  </AuthShell>
</template>
