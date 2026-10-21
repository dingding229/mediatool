<script setup lang="ts">
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import AuthShell from '../auth/AuthShell.vue';
import PasswordForm from '../auth/PasswordForm.vue';
const auth = useAuthStore(), router = useRouter(), error = ref('');
async function logout() {
  try {
    await auth.logout();
    await router.replace('/login');
  }
  catch (e) {
    error.value = e instanceof Error ? e.message : '退出失败';
  }
}
</script>
<template>
  <AuthShell title="必须设置新密码" description="请设置新密码以继续使用。完成后，请使用新密码重新登录。">
    <PasswordForm required/>
    <el-button @click="logout">退出</el-button>
    <p v-if="error" role="alert" class="auth-error">{{error}}</p>
  </AuthShell>
</template>
