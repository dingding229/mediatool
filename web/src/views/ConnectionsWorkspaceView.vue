<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { request } from '../api/client';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import StateView from '../ui/StateView.vue';
import ServiceReadiness from '../workflows/ServiceReadiness.vue';
import { bytes, date, type Readiness } from '../workflows/readiness';
import ConnectionManager from '../connections/ConnectionManager.vue';
import ProxySettings from '../connections/ProxySettings.vue';
import GatewayDiagnostics from '../connections/GatewayDiagnostics.vue';
import NotificationHistory from '../connections/NotificationHistory.vue';
import '../connections/connections.css';
const route = useRoute(), tabs = [{ id: 'overview', label: '总览' }, { id: 'storage', label: '115/CD2' }, { id: 'emby', label: 'Emby' }, { id: 'notifications', label: '通知渠道' }, { id: 'aux', label: 'TMDB/代理' }];
const tab = computed(() => tabs.some(t => t.id === route.query.tab) ? String(route.query.tab) : 'overview');
const readiness = ref<Readiness | null>(null), loading = ref(false), error = ref<unknown>(null), account = ref<any>(null), accountBusy = ref(false), accountError = ref('');
async function load() {
  loading.value = true;
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
  accountError.value = '';
  try {
    account.value = await request('/cloud/accounts/info');
  }
  catch {
    accountError.value = '空间或会员信息暂不可用，不推断配额或会员有效期。';
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
</script>
<template>
<div class="connections-workspace">
  <PageHeader title="服务连接" description="把存储、播放和通知接好；密钥只显示设置状态，不回显已保存的明文。">
    <template #actions>
      <el-button type="primary" :loading="loading" @click="load">刷新状态</el-button>
    </template>
  </PageHeader>
  <PageTabs :tabs="tabs" default-tab="overview" />
  <ServiceReadiness v-if="tab==='overview'" :value="readiness" :loading="loading" :error="error" @retry="load" />
  <template v-else-if="tab==='storage'">
    <ConnectionManager :allowed-kinds="['clouddrive2','open115']" title="云盘连接与115授权" />
    <section class="connection-manager" aria-label="云盘空间和会员">
      <header class="connection-section-heading">
        <h2>空间与会员</h2>
        <el-button :loading="accountBusy" @click="loadAccount">刷新空间与会员</el-button>
      </header>
      <StateView v-if="accountBusy" state="loading" title="正在读取官方账号信息…" />
      <el-alert v-else-if="accountError" :title="accountError" type="warning" :closable="false" />
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
        <p>{{ account.info.userName||account.info.userId||'—' }} · {{ date(account.info.checkedAt) }}</p>
        <p v-if="account.info.spaceError">空间信息暂不可用</p>
        <p v-for="(member,index) in account.info.memberships||[]" :key="index">{{ member.identity||'—' }} {{ member.level||'—' }} · 到期 {{ date(member.expiresAt) }}</p>
        <p v-if="!account.info.memberships?.length">会员信息：—</p>
        <p>{{ account.note||'—' }}</p>
      </template>
    </section>
  </template>
  <template v-else-if="tab==='emby'">
    <ConnectionManager :allowed-kinds="['emby']" title="Emby媒体库连接" />
    <GatewayDiagnostics />
  </template>
  <template v-else-if="tab==='notifications'">
    <ConnectionManager :allowed-kinds="['telegram']" title="通知渠道" />
    <NotificationHistory />
  </template>
  <template v-else-if="tab==='aux'">
    <ConnectionManager :allowed-kinds="['tmdb']" title="TMDB配置" />
    <ProxySettings />
  </template>
</div>
</template>
