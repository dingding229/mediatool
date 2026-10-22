<script setup lang="ts">
import { usePageRefresh } from '../ui/page-refresh';
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request } from '../api/client';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import StateView from '../ui/StateView.vue';
import ConnectionStatus from '../connections/ConnectionStatus.vue';
import { pageById } from '../navigation/pages';
import { bytes, date, services, stateFor, errorText, type Readiness } from '../workflows/readiness';
import ConnectionManager from '../connections/ConnectionManager.vue';
import ProxySettings from '../connections/ProxySettings.vue';
import GatewayDiagnostics from '../connections/GatewayDiagnostics.vue';
import NotificationHistory from '../connections/NotificationHistory.vue';
import '../connections/connections.css';
const route = useRoute(), tabs = pageById('connections').tabs;
const tab = computed(() => tabs.some(t => t.id === route.query.tab) ? String(route.query.tab) : 'storage');
const statusServices = computed(() => services.filter(s => tab.value === 'storage' ? s.tab === 'storage' : tab.value === 'emby' ? s.tab === 'emby' : false));
const readiness = ref<Readiness | null>(null), loading = ref(false), error = ref<unknown>(null), account = ref<any>(null), accountBusy = ref(false), accountError = ref<unknown>(null);
async function load() {
  loading.value = !readiness.value;
  error.value = null;
  try {
    readiness.value = await request('/readiness');
  }
  catch (e) {
    error.value = e;
  }
  finally {
    loading.value = false;
  }
}
async function loadAccount() {
  accountBusy.value = true;
  accountError.value = null;
  try {
    account.value = await request('/cloud/accounts/info');
  }
  catch (e) {
    accountError.value = e;
  }
  finally {
    accountBusy.value = false;
  }
}
watch(tab, value => {
  if (value === 'storage')
    void loadAccount();
});
onMounted(() => {
  void load();
  if (tab.value === 'storage')
    void loadAccount();
});
usePageRefresh(() => Promise.all([load(), ...(tab.value === 'storage' ? [loadAccount()] : [])]), '*');
</script>
<template>
  <div class="connections-workspace">
    <PageHeader title="连接" description="把存储、播放和通知接好；密钥默认隐藏，需要时可在编辑中主动查看。">
      <template #actions>

    </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="storage" />
    <ConnectionStatus v-if="statusServices.length" :services="statusServices" :value="readiness" :loading="loading" :error="error" @retry="load"/>
    <template v-if="tab==='storage'">
      <ConnectionManager :allowed-kinds="['clouddrive2','open115']" title="云盘连接与115授权" />
      <section class="connection-manager" aria-label="云盘空间和会员">
        <header class="connection-section-heading">
          <h2>空间与会员</h2>
        </header>
        <StateView v-if="accountBusy" state="loading" title="正在读取官方账号信息…" />
        <StateView v-else-if="accountError" :state="stateFor(accountError)" title="账号信息暂不可用" :description="errorText(accountError)" @retry="loadAccount"/>
        <template v-else-if="account?.info">
          <dl class="gateway-facts">
            <div>
              <dt>总容量</dt>
              <dd>{{ bytes(account.info.space?.total) }}</dd>
            </div>
            <div>
              <dt>已使用</dt>
              <dd>{{ bytes(account.info.space?.used) }}</dd>
            </div>
            <div>
              <dt>可用空间</dt>
              <dd>{{ bytes(account.info.space?.free) }}</dd>
            </div>
          </dl>
          <p>{{ account.info.userName||'—' }} · {{ date(account.info.checkedAt) }}</p>
          <p v-if="account.info.spaceError">空间信息暂不可用</p>
          <p v-for="(member,index) in account.info.memberships||[]" :key="index">{{ member.identity||'—' }} {{ member.level||'—' }} · 到期 {{ date(member.expiresAt) }}</p>
          <p v-if="!account.info.memberships?.length">会员信息：—</p>
        </template>
        <p v-else>尚未取得账号信息，请先完成 115 授权。</p>
      </section>
    </template>
    <template v-else-if="tab==='emby'">
      <ConnectionManager :allowed-kinds="['emby']" title="Emby媒体库连接" />
      <GatewayDiagnostics />
      <nav class="connection-actions" aria-label="播放相关管理">
        <RouterLink to="/playback?tab=settings">本程序播放服务</RouterLink>
        <RouterLink to="/playback?tab=players">播放器准入</RouterLink>
        <RouterLink to="/playback?tab=users">本程序媒体用户</RouterLink>
      </nav>
    </template>
    <template v-else-if="tab==='downloader'">
      <ConnectionManager :allowed-kinds="['qbittorrent']" title="下载器配置"/>
    </template>
    <template v-else-if="tab==='notifications'">
      <ConnectionManager :allowed-kinds="['telegram']" title="通知渠道" />
      <NotificationHistory />
    </template>
    <template v-else-if="tab==='aux'">
      <ConnectionManager :allowed-kinds="['tmdb']" title="TMDB配置" />
      <ProxySettings />
      <RouterLink class="connection-link" to="/sites?tab=accounts">管理站点账号</RouterLink>
    </template>
  </div>
</template>
<style scoped>
.connection-actions{display:flex;gap:16px;flex-wrap:wrap}.connection-actions a,.connection-link{display:inline-flex;align-items:center;min-height:44px;color:var(--mt-color-primary);overflow-wrap:anywhere}
</style>
