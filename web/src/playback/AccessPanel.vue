<script setup lang="ts">
import IpAddress from '../ui/IpAddress.vue';
import {usePageNumber,usePageSize} from '../ui/page-query';

import { computed, onMounted, reactive, ref } from 'vue';
import { request, post } from '../api/client';
import { useResource } from '../workflows/useResource';
import { useOperation } from '../workflows/useOperation';
import ResourceState from '../workflows/ResourceState.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import {getUAPolicy,saveUAPolicy,addUAObservation,type RuleList,type AccessMode} from '../api/playerAccess';
import { date, errorText } from '../workflows/readiness';
const query = ref(''), filter = ref(''), page = usePageNumber(),pageSize=usePageSize(25), visitPage = usePageNumber('visitPage'),visitSize=usePageSize(25,'visitPage'), policy = reactive(useResource<any>(async () => {const p=await getUAPolicy();return {...p,mode:p.mode||(p.enabled?'whitelist':'off'),blacklistRules:p.blacklistRules||[]}})), records = reactive(useResource<any>(() => request(`/player-access/observations?${new URLSearchParams({ q: query.value, filter: filter.value, page: String(page.value), pageSize: String(pageSize.value) })}`),{items:[],total:0})), visits = reactive(useResource<any>(() => request(`/player-access/events?page=${visitPage.value}&pageSize=${visitSize.value}&q=${encodeURIComponent(query.value)}`),{items:[],total:0})), op = reactive(useOperation('access')), editing = ref(false), form = reactive<any>({}), base = ref<any>(null), latest = ref<any>(null), confirm = ref(false), action = ref<any>(null), details = ref<any>(null), detailOpen = ref(false), notice = ref(''), copyNotice = ref('');
const list=ref<RuleList>('rules'),modeName=(mode:string)=>({off:'关闭（全部允许）',whitelist:'白名单（仅允许匹配规则）',blacklist:'黑名单（拒绝匹配规则）'}[mode]||'待核对');
const listName=(key:RuleList)=>key==='blacklistRules'?'黑名单':'白名单';
const rules=computed(()=>policy.data?.[list.value]||[]);
const matchName=(kind:string)=>({exact:'完整匹配',prefix:'前缀匹配',contains:'包含匹配'}[kind]||'待核对');
const ipSource = (s: string) => ({ direct: '直连', trusted_proxy: '可信反代', proxy_without_header: '代理未提供来源', trusted_chain_without_client: '代理链没有访客地址', invalid_forwarded_chain: '代理链无效', legacy_unknown: '历史来源未核验' } as Record<string, string>)[s] || '来源待核对', reason = (s: string) => ({ restriction_disabled: '限制关闭', whitelisted: '命中白名单', ua_not_whitelisted: '未匹配白名单', blacklisted:'命中黑名单',not_blacklisted:'未匹配黑名单' } as Record<string, string>)[s] || '原因待核对';
async function load() {
  await Promise.all([policy.load(), records.load(), visits.load()]);
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
function edit(rule: any = null, observation: any = null, targetList:RuleList = list.value) {
  op.error = null;
  latest.value = null;
  base.value = JSON.parse(JSON.stringify(policy.data));
  Object.assign(form, { list:targetList, id: rule?.id || '', label: rule?.label || '', matchType: rule?.matchType || 'exact', userAgent: rule?.userAgent || observation?.userAgent || '', enabled: rule?.enabled ?? true, observationId: observation?.id || '' });
  editing.value = true;
}
async function submit() {
  if (!form.userAgent.trim() || new TextEncoder().encode(form.userAgent).length > 4096) {
    op.error = Error('请填写不超过4096字节的完整 UA，空 UA 不能加入');
    return;
  }
  if (!base.value)
    return;
  const rule = { id: form.id, label: form.label, matchType: form.matchType, userAgent: form.userAgent, enabled: form.enabled }, rules = (base.value[form.list]||[]).filter((r: any) => r.id !== form.id);
  rules.push(rule);
  const p = { action: 'save', revision: base.value.revision };
  const out = await op.run(JSON.stringify(p), () => form.observationId ? addUAObservation(form.observationId,form.list,p.revision,form.label) : saveUAPolicy({...base.value,[form.list]:rules}));
  if (out !== undefined) {
    await load();
    if (!op.pending)
      editing.value = false;
  }
  else if (op.error) {
    latest.value = await policy.load() || { revision: null, rules: [], blacklistRules:[],mode:'off',enabled: false };
    notice.value = latest.value.revision == null ? '重新读取失败，输入已保留，请先读取当前策略。' : '配置已被修改或保存未完成，已重新读取；输入已保留。请核对后采用最新修订。';
  }
}
function ask(kind: string, row: any = null) {
  const p = JSON.parse(JSON.stringify(policy.data));
  action.value = { kind, row, list:list.value, mode:kind==='toggle'?(p.enabled?'off':'whitelist'):row, policy: p };
  confirm.value = true;
}
function chooseMode(event:Event){const select=event.target as HTMLSelectElement;ask('mode',select.value);select.value=policy.data?.mode||'off'}
async function apply() {
  const a = action.value, p = a.policy;
  const value={...p};
  if(a.kind==='toggle'||a.kind==='mode'){value.mode=a.mode;value.enabled=a.mode!=='off'}
  if(a.kind==='remove')value[a.list]=p[a.list].filter((r:any)=>r.id!==a.row.id);
  await op.run(JSON.stringify({action:a.kind,revision:p.revision}),()=>a.kind==='clear'?post('/player-access/observations/clear',{confirm:true,revision:p.revision}):saveUAPolicy(value));
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
defineExpose({ load, search, edit, submit, ask, apply, policy, records, visits, query, filter, page, visitPage, form, base, latest, action, op, list });
</script>
<template>
  <div class="media-stack">
    <section class="media-surface">
      <header class="media-section-heading ua-policy-heading">
        <div class="ua-policy-intro"><h2>播放器准入</h2><p>播放器标识（UA）用于识别发起请求的设备或应用。</p></div>
        <div class="media-actions ua-policy-actions">
          <label class="u5-field">策略模式
            <select :value="policy.data?.mode||'off'" aria-label="UA 策略模式" :disabled="!policy.data||!!policy.error||!!op.pending" @change="chooseMode">
              <option v-for="mode in (['off','whitelist','blacklist'] as AccessMode[])" :key="mode" :value="mode">{{ modeName(mode) }}</option>
            </select>
          </label>
          <el-button :disabled="!policy.data||!!policy.error||!!op.pending" @click="edit()">添加 UA 规则</el-button>
        </div>
      </header>
<div class="f5-player-policy-rows">
      <ResourceState overlay :stale="policy.loaded" :loading="policy.loading" :error="policy.error" @reload="load"/>
      <p class="ua-policy-summary">切换模式不删除任何名单规则；UA 可由客户端伪造，不替代媒体授权。</p>
      <label class="u5-field">规则名单 <select v-model="list" aria-label="UA 规则名单"><option value="rules">白名单规则</option><option value="blacklistRules">黑名单规则</option></select></label>
      <p>{{ listName(list) }}规则只在对应模式生效。</p>
      <el-alert v-if="op.error&&!editing" :title="errorText(op.error)" type="error" :closable="false"/>
      <AsyncResultBanner v-if="op.phase" :status="op.phase" :message="op.phase==='queued'?'操作已排队，尚未核验完成。':'正在核对上次结果，不重复提交。'" @refresh="load"/>
      <article v-for="rule in rules" :key="rule.id" class="organize-record">
        <h3>{{ rule.label||'—' }}</h3>
        <p>{{ matchName(rule.matchType) }} · {{ rule.enabled?'启用':'停用' }}</p>
        <p class="u5-original">{{ rule.userAgent }}</p>
        <div class="media-actions">
          <el-button :disabled="!!op.pending" @click="edit(rule)">编辑规则</el-button>
          <el-button :disabled="!!op.pending" @click="ask('remove',rule)">删除规则</el-button>
        </div>
      </article>
      <p v-if="policy.data&&!rules.length">尚无{{ listName(list) }}规则。</p>
</div>
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
          名单状态
          <select v-model="filter" aria-label="UA记录筛选" @change="search">
            <option value="">全部访问</option>
            <option value="whitelisted">已在白名单</option>
            <option value="unlisted">未在白名单</option>
            <option value="blacklisted">已在黑名单</option>
          </select>
        </label>
        <el-button native-type="submit">搜索记录</el-button>
      </form>
      <ResourceState :stale="records.loaded" :loading="records.loading" :error="records.error" :empty="!records.data?.items.length" @reload="records.load"/>
      <article v-for="record in records.data?.items||[]" :key="record.id" class="organize-record u5-ua-record">
        <p class="u5-original">{{ record.userAgent||'空 UA' }}</p>
        <p>访问 {{ record.requestCount??'—' }} · 通过 {{ record.allowedCount??'—' }} · 拒绝 {{ record.deniedCount??'—' }} · {{ record.whitelisted?'已在白名单':'未在白名单' }} · {{ record.blacklisted?'已在黑名单':'未在黑名单' }}</p>
        <p><IpAddress :ip="record.lastClientIp"/> · {{ date(record.lastSeenAt) }} · {{ reason(record.lastReason) }}</p>
        <div class="media-actions">
          <el-button @click="details=record;copyNotice='';detailOpen=true">查看完整 UA</el-button>
          <el-button :disabled="!record.userAgent||record.whitelisted||!!op.pending||!policy.data" @click="edit(null,record,'rules')">加入白名单</el-button>
          <el-button :disabled="!record.userAgent||record.blacklisted||!!op.pending||!policy.data" @click="edit(null,record,'blacklistRules')">加入黑名单</el-button>
        </div>
      </article>
      <PagePagination v-model:current-page="page" v-model:page-size="pageSize" :total="records.data.total" layout="prev,pager,next,total" @current-change="records.load"/>
    </section>
    <section class="media-surface">
      <h2>访问明细</h2>
      <ResourceState :stale="visits.loaded" :loading="visits.loading" :error="visits.error" :empty="!visits.data?.items.length" @reload="visits.load"/>
      <article v-for="visit in visits.data?.items||[]" :key="visit.id" class="organize-record">
        <h3>访问记录</h3>
        <p>{{ visit.allowed?'准入通过':'准入拒绝' }} · HTTP {{ visit.status??'—' }} · {{ reason(visit.reason) }}</p>
        <p>访客地址 <IpAddress :ip="visit.clientIp"/> · {{ ipSource(visit.ipSource) }} · {{ visit.ipPublic?'公网地址':'非公网或未核验' }}</p>
        <p>连接地址 <IpAddress :ip="visit.connectionIp"/> · {{ visit.method||'—' }} · {{ date(visit.createdAt) }}</p>
        <p class="u5-original">{{ visit.userAgent||'空 UA' }}</p>
        <el-button @click="details=visit;copyNotice='';detailOpen=true">访问详情</el-button>
      </article>
      <PagePagination v-model:current-page="visitPage" v-model:page-size="visitSize" :total="visits.data.total" layout="prev,pager,next,total" @current-change="visits.load"/>
    </section>
  </div>
  <el-dialog v-model="editing" :title="form.observationId?'从访问记录加入'+listName(form.list):(form.id?'编辑':'添加')+listName(form.list)+'规则'" width="min(680px,94vw)" class="media-dialog u5-dialog" :close-on-click-modal="false">
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
          <option value="contains">包含匹配</option>
        </select>
      </label>
      <label v-if="!form.observationId" class="u5-check">
        <input v-model="form.enabled" type="checkbox" :disabled="op.busy"/>
        启用此规则
      </label>
      <p v-else>从访问记录加入的规则会启用，仅在对应名单模式下生效。</p>
      <p v-if="notice">{{ notice }}</p>
      <el-alert v-if="op.error" :title="errorText(op.error)" type="error" :closable="false"/>
      <section v-if="latest">
        <el-button v-if="latest.revision==null" @click="policy.load().then(p=>latest=p||latest)">核对准入策略</el-button>
        <p>当前策略：{{ modeName(latest.mode) }} · {{ (latest[form.list]||[]).length }} 条{{ listName(form.list) }}规则</p>
        <p v-for="rule in latest[form.list]||[]" :key="rule.id" class="u5-original">{{ rule.label||'—' }}：{{ rule.userAgent }}</p>

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
    <p>访客地址 <IpAddress :ip="details?.clientIp||details?.lastClientIp"/> · 连接地址 <IpAddress :ip="details?.connectionIp"/></p>
    <el-button v-if="details?.requestCount!=null&&!details?.whitelisted&&details?.userAgent" :disabled="!policy.data||!!op.pending" @click="detailOpen=false;edit(null,details,'rules')">从此记录加入白名单</el-button>
    <el-button v-if="details?.requestCount!=null&&!details?.blacklisted&&details?.userAgent" :disabled="!policy.data||!!op.pending" @click="detailOpen=false;edit(null,details,'blacklistRules')">从此记录加入黑名单</el-button>
  </DetailDrawer>
  <ConfirmAction v-model:visible="confirm" :title="action?.kind==='remove'?'删除 UA '+listName(action.list)+'规则':action?.kind==='clear'?'清理 UA 访问记录':'切换 UA 策略为'+modeName(action?.mode)" :description="action?.kind==='remove'?'删除后会改变对应名单的准入结果，请核对规则。':action?.kind==='clear'?'只清理 UA 聚合记录和明细，新的访问会重新记录。':'关闭时全部允许；白名单仅允许匹配规则；黑名单拒绝匹配规则、允许其余 UA。现有两份名单均保留。'" impact-scope="不删除媒体、不删除程序日志；名单策略同时约束 Emby API 和 302 播放入口，仍检查媒体授权。" :confirm-text="action?.kind==='remove'?'删除规则':action?.kind==='clear'?'清理记录':'确认切换'" :loading="op.busy" class="media-dialog" @confirm="apply"/>

</template>

<style scoped>
.ua-policy-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:16px;flex-wrap:wrap}
.ua-policy-intro{min-width:0;flex:1 1 260px}
.ua-policy-intro h2{margin:0 0 8px}
.ua-policy-intro p{margin:0}
.ua-policy-heading .ua-policy-actions{display:flex;align-items:flex-end;gap:12px;flex-wrap:wrap}
.ua-policy-actions .u5-field{min-width:0;margin:0}
.ua-policy-actions select,.ua-policy-actions .el-button{box-sizing:border-box;height:44px;min-height:44px;margin:0}
.u5-field select{max-width:100%;min-height:44px}
.ua-policy-summary{min-height:44px}
.u5-ua-record .media-actions{align-items:center}
@media(max-width:600px){.ua-policy-summary{min-height:88px}.ua-policy-actions{width:100%}.ua-policy-actions .u5-field{flex:1}.ua-policy-actions select{width:100%}}
</style>
