<script setup lang="ts">
import { onUnmounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../stores/auth';
import { passwordPolicyError, PASSWORD_MAX_LENGTH, EXISTING_PASSWORD_MAX_LENGTH } from './password-policy';
const props = defineProps<{
  required?: boolean;
}>(), auth = useAuthStore(), router = useRouter(), current = ref(''), next = ref(''), confirmation = ref(''), busy = ref(false), error = ref('');
let alive = true;
function clear() {
  current.value = '';
  next.value = '';
  confirmation.value = '';
}
async function save() {
  if (busy.value)
    return;
  error.value = '';
  const issue = passwordPolicyError(next.value);
  if (issue) {
    error.value = issue;
    return;
  }
  if (next.value !== confirmation.value) {
    error.value = '两次新密码不一致';
    return;
  }
  busy.value = true;
  try {
    await auth.changePassword(current.value, next.value, confirmation.value);
    clear();
    if (alive)
      await router.replace('/login');
  }
  catch (e) {
    if (alive)
      error.value = e instanceof Error ? e.message : '密码更新失败';
  }
  finally {
    if (alive)
      busy.value = false;
  }
}
onUnmounted(() => {
  alive = false;
  clear();
});
</script>
<template>
  <form class="password-fields" @submit.prevent="save">
    <el-alert v-if="error" :title="error" type="error" :closable="false"/>
    <div>
      <label for="current-password">当前密码</label>
      <el-input id="current-password" v-model="current" type="password" show-password autocomplete="current-password" :maxlength="EXISTING_PASSWORD_MAX_LENGTH" :disabled="busy"/>
    </div>
    <div>
      <label for="new-password">新密码</label>
      <el-input id="new-password" v-model="next" type="password" show-password autocomplete="new-password" :maxlength="PASSWORD_MAX_LENGTH" :disabled="busy"/>
      <small>6–20 个字符，支持中文与空格，不去除首尾空格。</small>
    </div>
    <div>
      <label for="confirm-new-password">确认新密码</label>
      <el-input id="confirm-new-password" v-model="confirmation" type="password" show-password autocomplete="new-password" :maxlength="PASSWORD_MAX_LENGTH" :disabled="busy"/>
    </div>
    <p>修改后所有登录会话都会失效，需要重新登录。</p>
    <el-button type="primary" native-type="submit" :loading="busy" :disabled="!current||!next||!confirmation">{{props.required?'设置新密码并重新登录':'修改密码并重新登录'}}</el-button>
  </form>
</template>
<style scoped>
.password-fields{display:grid;gap:20px;max-width:580px}.password-fields label{display:block;margin-bottom:8px}.password-fields small,.password-fields p{color:var(--mt-text-muted);font-size:13px;line-height:1.7}.password-fields .el-button{min-height:44px;white-space:normal;height:auto;padding:12px}.password-fields :deep(.el-input__wrapper){min-height:44px;box-sizing:border-box}
</style>
