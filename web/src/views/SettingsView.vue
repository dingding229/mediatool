<script setup lang="ts">
import { computed, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { passwordPolicyError } from "../auth/password-policy";
import SetupGuideControl from "../dashboard/SetupGuideControl.vue";
import ApplicationSettings from "../components/ApplicationSettings.vue";
import TMDBSettings from "../components/TMDBSettings.vue";
import NetworkProxySettings from "../components/NetworkProxySettings.vue";
import PWASettings from "../components/PWASettings.vue";
import AccessSettings from "../components/AccessSettings.vue";
import {
  Link,
  FolderChecked,
  Timer,
  Connection,
  Film,
  User,
  Cellphone,
} from "@element-plus/icons-vue";
const auth = useAuthStore(),
  route = useRoute(),
  router = useRouter();
const tabs = [
  {
    id: "access",
    label: "访问与域名",
    icon: Link,
    hint: "管理入口、播放域名与独立媒体端口",
  },
  {
    id: "library",
    label: "命名与刮削",
    icon: FolderChecked,
    hint: "文件命名、刮削输出与上传后清理",
  },
  {
    id: "automation",
    label: "搜索与订阅",
    icon: Timer,
    hint: "搜索缓存和统一Cron调度",
  },
  {
    id: "proxy",
    label: "网络代理",
    icon: Connection,
    hint: "TMDB与PT站点的请求路由",
  },
  { id: "tmdb", label: "TMDB", icon: Film, hint: "元数据服务凭据与配置" },
  {
    id: "account",
    label: "管理员账号",
    icon: User,
    hint: "修改管理员密码与恢复说明",
  },
  {
    id: "mobile",
    label: "移动端应用",
    icon: Cellphone,
    hint: "PWA安装及公开资源更新",
  },
];
const active = computed(() =>
  tabs.some((tab) => tab.id === route.query.tab)
    ? String(route.query.tab)
    : route.query.connectionKind === "tmdb"
      ? "tmdb"
      : "access",
);
const selected = computed(() => tabs.find((tab) => tab.id === active.value)!);
function select(id: string) {
  void router.replace({ path: "/settings", query: { tab: id } });
}
const current = ref(""),
  next = ref(""),
  confirm = ref(""),
  busy = ref(false),
  error = ref("");
async function change() {
  error.value = "";
  const issue = passwordPolicyError(next.value);
  if (issue) {
    error.value = issue;
    return;
  }
  if (next.value !== confirm.value) {
    error.value = "两次新密码不一致";
    return;
  }
  busy.value = true;
  try {
    await auth.changePassword(current.value, next.value, confirm.value);
    current.value = "";
    next.value = "";
    confirm.value = "";
    await router.replace("/login");
  } catch (e) {
    error.value = e instanceof Error ? e.message : "修改失败";
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">SYSTEM / 设置</span>
      <h1>系统设置</h1>
      <p>
        按功能管理系统配置。保存范围在各表单中独立说明，切换分组不会自动保存。
      </p>
    </div>
  </section>
  <SetupGuideControl />
  <div class="settings-layout">
    <nav class="settings-navigation" aria-label="系统设置分类">
      <button
        v-for="tab in tabs"
        :key="tab.id"
        :aria-pressed="active === tab.id"
        :class="{ active: active === tab.id }"
        @click="select(tab.id)"
      >
        <component :is="tab.icon" aria-hidden="true" /><span>{{
          tab.label
        }}</span>
      </button>
    </nav>
    <div class="settings-content" :aria-label="selected.label">
      <AccessSettings v-show="active === 'access'" />
      <ApplicationSettings
        v-show="['access', 'library', 'automation'].includes(active)"
        :section="active"
      />
      <div v-show="active === 'proxy'"><NetworkProxySettings /></div>
      <TMDBSettings v-show="active === 'tmdb'" />
      <section v-show="active === 'account'" class="panel settings-panel">
        <div class="panel-title">
          <div>
            <h2>管理员账号</h2>
            <p class="field-hint">当前账号：{{ auth.account?.username }}</p>
          </div>
        </div>
        <el-alert v-if="error" :title="error" type="error" :closable="false" />
        <form class="settings-form password-form" @submit.prevent="change">
          <div class="settings-field">
            <label for="current-password">当前密码</label
            ><el-input
              id="current-password"
              v-model="current"
              type="password"
              show-password
              autocomplete="current-password"
              :maxlength="512"
            />
          </div>
          <div class="settings-field">
            <label for="new-password">新密码</label
            ><el-input
              id="new-password"
              v-model="next"
              type="password"
              show-password
              autocomplete="new-password"
              :maxlength="20"
            />
            <p class="field-hint">6–20个字符；修改后撤销所有会话。</p>
          </div>
          <div class="settings-field">
            <label for="confirm-new-password">确认新密码</label
            ><el-input
              id="confirm-new-password"
              v-model="confirm"
              type="password"
              show-password
              autocomplete="new-password"
              :maxlength="20"
            />
          </div>
          <div class="settings-actions">
            <el-button
              type="primary"
              native-type="submit"
              :loading="busy"
              :disabled="!current || !next || !confirm"
              >修改密码并重新登录</el-button
            >
          </div>
        </form>
        <div class="settings-notice">
          <strong>账号恢复由服务器所有者操作</strong>
          <p>
            在主程序的数据目录创建空 password 文件并重启
            app，启动控制台仅输出一次30分钟有效的临时密码。登录后必须设置新密码，后台不提供找回入口。
          </p>
        </div>
      </section>
      <div v-show="active === 'mobile'"><PWASettings /></div>
    </div>
  </div>
</template>
<style scoped>
.settings-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 24px;
  align-items: start;
}
.settings-navigation {
  display: flex;
  flex-direction: row;
  flex-wrap: wrap;
  gap: 8px;
  padding: 0 0 16px;
  border-bottom: 1px solid var(--border);
}
.settings-navigation button {
  display: flex;
  align-items: center;
  gap: 10px;
  min-height: 46px;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel-bg);
  color: var(--text);
  text-align: left;
  font: inherit;
  font-size: 14px;
  cursor: pointer;
}
.settings-navigation button.active {
  background: var(--el-color-primary-light-9);
  color: var(--el-color-primary);
  border-color: var(--border);
}
.settings-navigation button:focus-visible {
  outline: 2px solid var(--el-color-primary);
}
.settings-navigation svg {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}
.settings-content {
  display: grid;
  gap: 24px;
  min-width: 0;
  width: 100%;
}
.settings-section-heading {
  margin-bottom: 16px;
}
.settings-section-title {
  display: block;
  font-size: 20px;
  margin: 0 0 8px;
}
.settings-section-heading p {
  font-size: 13px;
  color: var(--muted);
  margin: 0;
}
.password-form {
  max-width: 560px;
}
.settings-tmdb :deep(.service-tabs) {
  display: none;
}
.settings-tmdb :deep(.page-heading) {
  padding: 20px;
  background: var(--surface, #fff);
  border: 1px solid var(--border);
  border-radius: 10px;
  margin-bottom: 16px;
}
.settings-tmdb :deep(.el-alert--info) {
  display: none;
}
@media (max-width: 1000px) {
  .settings-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .settings-navigation {
    position: static;
    flex-direction: row;
    flex-wrap: nowrap;
    overflow-x: auto;
    max-width: 100%;
    padding-bottom: 6px;
  }
  .settings-navigation button {
    flex-shrink: 0;
  }
  .settings-section-heading {
    margin-bottom: 12px;
  }
}
</style>

<style scoped>
.settings-content :deep(.panel) {
  margin: 0;
  padding: 24px;
}
.settings-content :deep(.panel-title) {
  gap: 16px;
  flex-wrap: wrap;
}
.settings-content :deep(.settings-panel) {
  max-width: none;
}
.settings-navigation button {
  transition:
    background 0.16s,
    color 0.16s;
}
.settings-navigation button.active {
  font-weight: 600;
}
.settings-content > div {
  min-width: 0;
}
@media (max-width: 680px) {
  .settings-content {
    gap: 16px;
  }
  .settings-content :deep(.panel) {
    padding: 16px;
  }
}
</style>
