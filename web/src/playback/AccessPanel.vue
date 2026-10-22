<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';

import { onMounted, reactive, ref } from 'vue';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { date, errorText } from '../workflows/readiness';
const query = ref(''), filter = ref(''), page = usePageNumber(), visitPage = usePageNumber('visitPage'), embyUser = ref(''), policy = reactive(useResource<any>(() => request('/player-access/policy'))), records = reactive(useResource<any>(() => request(`/player-access/observations?${new URLSearchParams({ q: query.value, filter: filter.value, page: String(page.value), pageSize: '25' })}`))), visits = reactive(useResource<any>(() => request(`/player-access/events?page=${visitPage.value}&pageSize=25&q=${encodeURIComponent(query.value)}&embyUserId=${encodeURIComponent(embyUser.value)}`))), embyUsers = reactive(useResource<any>(() => request('/player-access/users'), [])), op = reactive(useOperation('access')), editing = ref(false), form = reactive<any>({}), base = ref<any>(null), latest = ref<any>(null), confirm = ref(false), action = ref<any>(null), details = ref<any>(null), detailOpen = ref(false), notice = ref(''), copyNotice = ref('');
const ipSource = (s: string) => ({ direct: '直连', trusted_proxy: '可信反代', proxy_without_header: '代理未提供来源', trusted_chain_without_client: '代理链没有访客地址', invalid_forwarded_chain: '代理链无效', legacy_unknown: '历史来源未核验' } as Record<string, string>)[s] || '来源待核对', reason = (s: string) => ({ restriction_disabled: '限制关闭', whitelisted: '命中白名单', ua_not_whitelisted: '未匹配白名单' } as Record<string, string>)[s] || '原因待核对';
async function load() {
  await Promise.all([policy.load(), records.load(), visits.load(), embyUsers.load()]);
  if (op.pending && !policy.error) {
    const p = JSON.parse(op.pending.key);
    if (p.action === 'clear' && !records.error)
      op.clear();
    else if (policy.data.revision !== p.revision)
      op.clear();
  }
}
function search() {
  page.value = 1;
  visitPage.value = 1;
  void records.load();
  void visits.load();
}
function edit(rule: any = null, observation: any = null) {
  op.error = null;
  latest.value = null;
  base.value = JSON.parse(JSON.stringify(policy.data));
  Object.assign(form, { id: rule?.id || '', label: rule?.label || '', matchType: rule?.matchType || 'exact', userAgent: rule?.userAgent || observation?.userAgent || '', enabled: rule?.enabled ?? true, observationId: observation?.id || '' });
  editing.value = true;
}
async function submit() {
  if (!form.userAgent.trim() || new TextEncoder().encode(form.userAgent).length > 4096) {
    op.error = Error('请填写不超过4096字节的完整 UA，空 UA 不能加入');
    return;
  }
  if (!base.value)
    return;
  const rule = { id: form.id, label: form.label, matchType: form.matchType, userAgent: form.userAgent, enabled: form.enabled }, rules = base.value.rules.filter((r: any) => r.id !== form.id);
  rules.push(rule);
  const p = { action: 'save', revision: base.value.revision };
  const out = await op.run(JSON.stringify(p), () => form.observationId ? post(`/player-access/observations/${form.observationId}/allow`, { revision: p.revision, label: form.label, confirm: true }) : request('/player-access/policy', { method: 'PUT', body: JSON.stringify({ enabled: base.value.enabled, revision: p.revision, rules }) }));
  if (out !== undefined) {
    await load();
    if (!op.pending)
      editing.value = false;
  }
  else if (op.error) {
    latest.value = await policy.load() || { revision: null, rules: [], enabled: false };
    notice.value = latest.value.revision == null ? '重新读取失败，输入已保留，请先读取当前策略。' : '配置已被修改或保存未完成，已重新读取；输入已保留。请核对后采用最新修订。';
  }
}
function ask(kind: string, row: any = null) {
  const p = JSON.parse(JSON.stringify(policy.data));
  action.value = { kind, row, policy: p };
  confirm.value = true;
}
async function apply() {
  const a = action.value, p = a.policy;
  await op.run(JSON.stringify({ action: a.kind, revision: p.revision }), () => a.kind === 'clear' ? post('/player-access/observations/clear', { confirm: true, revision: p.revision }) : request('/player-access/policy', { method: 'PUT', body: JSON.stringify({ enabled: a.kind === 'toggle' ? !p.enabled : p.enabled, revision: p.revision, rules: a.kind === 'remove' ? p.rules.filter((r: any) => r.id !== a.row.id) : p.rules }) }));
  confirm.value = false;
  await load();
}
async function copyUA() {
  try {
    await navigator.clipboard.writeText(details.value.userAgent);
    copyNotice.value = '完整 UA 已复制';
  }
  catch {
    copyNotice.value = '复制失败，请在完整 UA 字段中手动选择复制。';
  }
}
onMounted(load);
defineExpose({ load, search, edit, submit, ask, apply, policy, records, visits, query, filter, page, visitPage, embyUser, form, base, latest, action, op });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>播放器准入</h2><p>播放器标识（UA）用于识别发起请求的设备或应用。</p>
        <div class="media-actions">
          <el-button :disabled="!policy.data||!!policy.error||!!op.pending" @click="edit()">添加 UA 规则</el-button>
          <el-button :disabled="!policy.data||!!policy.error||!!op.pending" @click="ask('toggle')">{{ policy.data?.enabled?'关闭 UA 限制':'启用 UA 限制' }}</el-button>
        </div>
      </header>
      <ResourceState :stale="policy.loaded" :loading="policy.loading" :error="policy.error" @reload="load"/>
      <p v-if="policy.data">{{ policy.data.enabled?'仅允许命中白名单的播放器访问。':'当前仅记录访问，不按UA限制现有播放器。' }}</p>
      <el-alert v-if="op.error&&!editing" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
      <article v-for="rule in policy.data?.rules||[]" :key="rule.id" class="organize-record">
        <h3>{{ rule.label||'—' }}</h3>
        <p>{{ rule.matchType==='prefix'?'前缀匹配':'完整匹配' }} · {{ rule.enabled?'启用':'停用' }}</p>
        <p class="u5-original">{{ rule.userAgent }}</p>
        <div class="media-actions">
          <el-button :disabled="!!op.pending" @click="edit(rule)">编辑规则</el-button>
          <el-button :disabled="!!op.pending" @click="ask('remove',rule)">删除规则</el-button>
        </div>
      </article>
      <p v-if="policy.data&&!policy.data.rules.length">尚无 UA 规则。</p>
    </section>
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>播放器访问汇总</h2>
        <el-button :disabled="!policy.data||!!op.pending" @click="ask('clear')">清空访问记录</el-button>
      </header>
      <form class="u5-filters" @submit.prevent="search">
        <label>
          搜索 UA
          <input v-model="query" aria-label="搜索UA记录"/>
        </label>
        <label>
          白名单状态
          <select v-model="filter" aria-label="UA记录筛选" @change="search">
            <option value="">全部访问</option>
            <option value="whitelisted">已在白名单</option>
            <option value="unlisted">未在白名单</option>
          </select>
        </label>
        <el-button native-type="submit">搜索记录</el-button>
      </form>
      <ResourceState :stale="records.loaded" :loading="records.loading" :error="records.error" :empty="!records.data?.items.length" @reload="records.load"/>
      <article v-for="record in records.data?.items||[]" :key="record.id" class="organize-record u5-ua-record">
        <p class="u5-original">{{ record.userAgent||'空 UA' }}</p>
        <p>访问 {{ record.requestCount??'—' }} · 通过 {{ record.allowedCount??'—' }} · 拒绝 {{ record.deniedCount??'—' }} · {{ record.whitelisted?'已在白名单':'未在白名单' }}</p>
        <p>{{ record.lastClientIp||'—' }} · {{ date(record.lastSeenAt) }} · {{ reason(record.lastReason) }}</p>
        <div class="media-actions">
          <el-button @click="details=record;copyNotice='';detailOpen=true">查看完整 UA</el-button>
          <el-button :disabled="!record.userAgent||record.whitelisted||!!op.pending||!policy.data" @click="edit(null,record)">加入白名单</el-button>
        </div>
      </article>
      <PagePagination v-model:current-page="page" :page-size="25" :total="records.data.total" layout="prev,pager,next,total" @current-change="records.load"/>
    </section>
    <section class="media-surface">
      <h2>访问明细</h2>
      <label class="u5-field">
        Emby 用户
        <select v-model="embyUser" aria-label="Emby访问用户筛选" @change="visitPage=1;visits.load()">
          <option value="">全部 Emby 用户</option>
          <option value="unknown">未核验用户</option>
          <option v-for="user in embyUsers.data" :key="user.id" :value="user.id">{{ user.name }}</option>
        </select>
      </label>
      <ResourceState :error="embyUsers.error" @reload="embyUsers.load"/>
      <ResourceState :stale="visits.loaded" :loading="visits.loading" :error="visits.error" :empty="!visits.data?.items.length" @reload="visits.load"/>
      <article v-for="visit in visits.data?.items||[]" :key="visit.id" class="organize-record">
        <h3>{{ visit.embyVerified?visit.embyUserName||'已核验用户':'未核验用户' }}</h3>
        <p>{{ visit.allowed?'准入通过':'准入拒绝' }} · HTTP {{ visit.status??'—' }} · {{ reason(visit.reason) }}</p>
        <p>访客地址 {{ visit.clientIp||'—' }} · {{ ipSource(visit.ipSource) }} · {{ visit.ipPublic?'公网地址':'非公网或未核验' }}</p>
        <p>连接地址 {{ visit.connectionIp||'—' }} · {{ visit.method||'—' }} · {{ date(visit.createdAt) }}</p>
        <p class="u5-original">{{ visit.userAgent||'空 UA' }}</p>
        <el-button @click="details=visit;copyNotice='';detailOpen=true">访问详情</el-button>
      </article>
      <PagePagination v-model:current-page="visitPage" :page-size="25" :total="visits.data.total" layout="prev,pager,next,total" @current-change="visits.load"/>
    </section>
  </div>
  <el-dialog v-model="editing" :title="form.observationId?'从访问记录加入白名单':form.id?'编辑 UA 规则':'添加 UA 规则'" width="min(680px,94vw)" class="media-dialog u5-dialog" :close-on-click-modal="false">
    <div class="u5-form">
      <label>
        播放器备注
        <input v-model="form.label" aria-label="播放器备注" :disabled="op.busy"/>
      </label>
      <label>
        完整 User-Agent
        <textarea v-model="form.userAgent" rows="6" aria-label="完整 User-Agent" :readonly="!!form.observationId" :disabled="op.busy"/>
      </label>
      <label v-if="!form.observationId">
        匹配方式
        <select v-model="form.matchType" aria-label="UA匹配方式" :disabled="op.busy">
          <option value="exact">完整匹配</option>
          <option value="prefix">前缀匹配</option>
        </select>
      </label>
      <label class="u5-check">
        <input v-model="form.enabled" type="checkbox" :disabled="op.busy"/>
        启用此规则
      </label>
      <p v-if="notice">{{ notice }}</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <section v-if="latest">
        <el-button v-if="latest.revision==null" @click="policy.load().then(p=>latest=p||latest)">核对准入策略</el-button>
        <p>当前策略：{{ latest.enabled?'启用限制':'不限 UA' }} · {{ latest.rules.length }} 条规则</p>
        <p v-for="rule in latest.rules" :key="rule.id" class="u5-original">{{ rule.label||'—' }}：{{ rule.userAgent }}</p>

        <el-button :disabled="latest.revision==null" @click="base=JSON.parse(JSON.stringify(latest));latest=null">保留输入并采用最新修订</el-button>
      </section>
    </div>
    <template #footer>
      <el-button :disabled="op.busy" @click="editing=false">取消</el-button>
      <el-button :disabled="!!op.pending||!!latest" :loading="op.busy" @click="submit" type="primary" class="mt-save-button">保存规则</el-button>
    </template>
  </el-dialog>
  <DetailDrawer v-model:visible="detailOpen" title="完整 UA 与访问详情" :technical-data="details" class="media-detail">
    <label class="u5-field">
      实际收到的完整 UA
      <textarea :value="details?.userAgent||''" readonly rows="7" aria-label="实际收到的完整 UA"/>
    </label>
    <el-button @click="copyUA">复制完整 UA</el-button>
    <p role="status">{{ copyNotice }}</p>
    <p>访客地址 {{ details?.clientIp||details?.lastClientIp||'—' }} · 连接地址 {{ details?.connectionIp||'—' }}</p>
    <el-button v-if="details?.requestCount!=null&&!details?.whitelisted&&details?.userAgent" :disabled="!policy.data||!!op.pending" @click="detailOpen=false;edit(null,details)">从此记录加入白名单</el-button>
  </DetailDrawer>
  <ConfirmAction v-model:visible="confirm" :title="action?.kind==='remove'?'删除 UA 白名单规则':action?.kind==='clear'?'清理 UA 访问记录':action?.policy.enabled?'关闭播放器限制':'启用播放器白名单'" :description="action?.kind==='remove'?'删除后，依赖此规则的播放器可能无法访问。':action?.kind==='clear'?'只清理 UA 聚合记录，新的访问会重新记录。':'启用后，未匹配的播放器和空 UA 会被拒绝；关闭后仍检查媒体授权。'" impact-scope="不删除媒体、不删除程序日志；聚合记录清理不移除白名单，也不清理访问明细。" :confirm-text="action?.kind==='remove'?'删除规则':action?.kind==='clear'?'清理记录':action?.policy.enabled?'关闭限制':'启用限制'" :loading="op.busy" class="media-dialog" @confirm="apply"/>
</template>
