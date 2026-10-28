<script setup lang="ts">
import PathText from '../ui/PathText.vue';
import '../organize/workspaces.css';
import {usePageRefresh} from '../ui/page-refresh';
import { computed, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ElDropdown, ElDropdownItem, ElDropdownMenu } from 'element-plus';
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import { APIError, post, request } from '../api/client';
import PageHeader from '../ui/PageHeader.vue';
import PageTabs from '../ui/PageTabs.vue';
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import IdentityEditor from '../organize/IdentityEditor.vue';
import ScanPanel from '../organize/ScanPanel.vue';
import HistoryPanel from '../organize/HistoryPanel.vue';
import { useOrganize } from '../organize/useOrganize';
import { label, selectable, rows } from '../organize/model';
import { sourceState,image } from '../media/model';
import PosterArtwork from '../media/PosterArtwork.vue';
import ImageEditor from '../media/ImageEditor.vue';
import '../media/catalog.css';
import { errorText } from '../workflows/readiness';
import { readIntent, writeIntent, size } from '../acquisition/model';
import '../media/media.css';
const route = useRoute(), router = useRouter(), state = reactive(useOrganize()), tabs = [{ id: 'candidates', label: '待确认' }, { id: 'scans', label: '扫描' }, { id: 'assets', label: '已入库' }, { id: 'history', label: '整理历史' }], tab = computed(() => tabs.some(t => t.id === route.query.tab) ? String(route.query.tab) : 'candidates');
const editor = ref<InstanceType<typeof IdentityEditor>>(), detailsOpen = ref(false), assetDetail = ref<any>(null), outputs = ref<any>(null), operations = ref<any[]>([]), detailError = ref<unknown>(null), confirmOpen = ref(false), action = ref<any>(null), busy = ref(false), phase = ref<'queued' | 'reconciling' | null>(null), actionError = ref<unknown>(null);
const assetView=ref<'wall'|'list'>(readAssetView());
function readAssetView():'wall'|'list'{try{return localStorage.getItem('media-tool:asset-view:v1')==='list'?'list':'wall'}catch{return 'wall'}}
function setAssetView(view:'wall'|'list'){assetView.value=view;try{localStorage.setItem('media-tool:asset-view:v1',view)}catch{/* View preference is optional. */}}
let detailVersion = 0;
function edit(files: any[], asset: any = null) {
  void editor.value?.open(files, asset, { page: state.page.candidates, state: state.filter });
}
function editing(value: boolean) {
  state.editing = value;
  if (!value)
    state.changed();
}
function pageChange(kind: 'candidates' | 'assets') {
  state.selected[kind] = [];
  void state.load(kind);
}
async function inspect(asset: any) {
  const version = ++detailVersion;
  assetDetail.value = asset;
  outputs.value = null;
  operations.value = [];
  detailsOpen.value = true;
  detailError.value = null;
  try {
    const [out, history] = await Promise.allSettled([request(`/library/outputs?page=1&pageSize=50&q=${encodeURIComponent(asset.path)}`), request(`/library/activity?page=1&pageSize=25&kind=file&q=${encodeURIComponent(asset.path.split('/').pop() || '')}`)]);
    if (version === detailVersion) {
      if (out.status === 'fulfilled')
        outputs.value = rows((out.value as any).items).find(r => r.id === asset.id) || null;
      else
        detailError.value = out.reason;
      if (history.status === 'fulfilled')
        operations.value = rows((history.value as any).items);
      else
        detailError.value = history.reason;
    }
  }
  catch (e) {
    if (version === detailVersion)
      detailError.value = e;
  }
}
function ask(row: any, type: string) {
  action.value = { row, type };
  actionError.value = null;
  phase.value = readIntent(`organize:${type}:${row.id}:${row.revision || row.state}`)?.phase || null;
  confirmOpen.value = !phase.value;
}
async function apply() {
  if (!action.value || busy.value || phase.value)
    return;
  const { row, type } = action.value, key = `organize:${type}:${row.id}:${row.revision || row.state}`;
  busy.value = true;
  phase.value = 'reconciling';
  writeIntent(key, { phase: 'reconciling' });
  try {
    if (type === 'rotate')
      await post(`/media/assets/${row.id}/grant/rotate`, {});
    else
      await post(`/media/assets/${row.id}/grant/revoke`, {});
    phase.value = 'queued';
    writeIntent(key, { phase: 'queued' });
    confirmOpen.value = false;
    state.changed();
  }
  catch (e) {
    actionError.value = e;
    confirmOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      phase.value = null;
      writeIntent(key, null);
    }
  }
  finally {
    busy.value = false;
  }
}
state.assetId = String(route.query.assetId || '');
watch(() => route.query.assetId, v => {
  state.assetId = String(v || '');
  state.page.assets = 1;
  void state.load('assets');
});
watch(detailsOpen, v => {
  if (!v)
    detailVersion++;
});
usePageRefresh(state.refresh,'*');
</script>
<template>
  <div class="media-workspace">
    <PageHeader title="入库整理" description="先确认影片和处理目录，再开始入库；遇到问题时可查看记录并继续处理。">
      <template #actions>

      </template>
    </PageHeader>
    <PageTabs :tabs="tabs" default-tab="candidates"/>
    <section v-show="tab==='candidates'" class="media-surface">
      <header class="media-section-heading">
        <div>
          <h2>待确认文件</h2>
          <p>可同时核对多个文件；已有入库记录的文件，请先查看处理结果。</p>
        </div>
        <el-select v-model="state.filter" aria-label="候选状态" @change="state.page.candidates=1;pageChange('candidates')">
          <el-option value="needs_confirmation" label="需要确认或继续处理"/>
          <el-option value="" label="全部状态"/>
          <el-option value="confirmed" label="已确认"/>
          <el-option value="artifacts_pending" label="输出待恢复"/>
        </el-select>
      </header>
      <div class="media-actions">
        <el-button :disabled="!state.selected.candidates.length||state.selected.candidates.length>20" @click="edit(state.selected.candidates)">批量确认作品 · {{ state.selected.candidates.length }}</el-button>
        <RouterLink v-if="state.selected.candidates.length>0&&state.selected.candidates.length<=20" :to="{path:'/files',query:{tab:'browser',selectionKind:'candidates',selection:state.selected.candidates.map((x:any)=>x.id)}}">在文件页批量整理 · {{state.selected.candidates.length}}</RouterLink>
      </div>
      <StateView v-if="state.loading.candidates&&!state.data.candidates.length" state="loading" title="正在读取候选文件…"/>
      <StateView v-else-if="state.errors.candidates" :state="sourceState(state.errors.candidates)" title="候选读取失败，旧数据待核对" :description="errorText(state.errors.candidates)" @retry="state.load('candidates')"/>
      <StateView v-else-if="!state.data.candidates.length" state="empty" title="暂无待确认文件" description="先在扫描标签选择目录并完成扫描。"/>
      <article v-for="candidate in state.data.candidates" :key="candidate.id" class="organize-record" :class="{highlighted:route.query.candidateId===candidate.id}">
        <label class="media-consent">
          <input type="checkbox" :checked="state.selected.candidates.some(r=>r.id===candidate.id)" :disabled="!selectable(candidate)||state.selected.candidates.length>=20&&!state.selected.candidates.some(r=>r.id===candidate.id)" @change="state.toggle('candidates',candidate,($event.target as HTMLInputElement).checked)"/>
          {{ candidate.name||'—' }}
        </label>
        <p>{{ candidate.path }}</p>
        <p>{{ label(candidate.state) }} · 云端：{{ label(candidate.cloudState) }} · {{ size(candidate.size) }}</p>
        <p v-if="selectable(candidate)">尚未绑定准确作品，先核对识别候选，再确认入库。</p>
        <el-button :disabled="!selectable(candidate)" @click="edit([candidate])">确认作品</el-button>
        <RouterLink v-if="candidate.assetId" :to="{path:'/organize',query:{tab:'assets',assetId:candidate.assetId}}">查看已有资产与输出</RouterLink>
      </article>
      <PagePagination v-model:current-page="state.page.candidates" v-model:page-size="state.sizes.candidates" :total="state.total.candidates" layout="prev,pager,next,total" @current-change="pageChange('candidates')"/>
    </section>
    <div v-show="tab==='scans'" class="media-stack">
      <ScanPanel :page="state.page.scans" :page-size="state.sizes.scans" @size="n=>state.sizes.scans=n" :total="state.total.scans" @page="(n:number)=>{state.page.scans=n;state.load('scans')}" :scans="state.data.scans" :schedules="state.data.schedules" :scan-error="state.errors.scans" :schedule-error="state.errors.schedules" :loading="state.loading.scans" :schedule-loading="state.loading.schedules" @changed="state.refresh"/>
    </div>
    <section v-show="tab==='assets'" class="media-surface">
      <header class="media-section-heading">
        <h2>已入库资产</h2>
 <nav class="mt-segments" aria-label="已入库视图"><button :aria-pressed="assetView==='wall'" @click="setAssetView('wall')">海报墙</button><button :aria-pressed="assetView==='list'" @click="setAssetView('list')">列表视图</button></nav>
        <RouterLink v-if="state.selected.assets.length>0&&state.selected.assets.length<=20" :to="{path:'/files',query:{tab:'browser',selectionKind:'assets',selection:state.selected.assets.map((x:any)=>x.id)}}">在文件页批量整理 · {{state.selected.assets.length}}</RouterLink>
      </header>
      <p v-if="state.assetId">
        当前仅显示所选资产。
        <el-button @click="router.replace({query:{tab:'assets'}})">显示全部资产</el-button>
      </p>
      <StateView v-if="state.loading.assets&&!state.data.assets.length" state="loading" title="正在读取资产…"/>
      <StateView v-else-if="state.errors.assets" :state="sourceState(state.errors.assets)" title="资产读取失败，旧数据待核对" :description="errorText(state.errors.assets)" @retry="state.load('assets')"/>
      <StateView v-else-if="!state.data.assets.length" state="empty" title="尚无已确认资产"/>
      <div class="asset-collection" :class="'asset-collection-'+assetView">
 <article v-for="asset in state.data.assets" :key="asset.id" class="organize-record asset-card">
 <button class="asset-poster-button" :aria-label="'查看'+(asset.title||'资产')+'详情'" @click="inspect(asset)"><PosterArtwork :src="image(asset.poster)" :title="asset.title||'资产'"/></button>
 <div class="asset-card-body">
        <label class="media-consent">
          <input type="checkbox" :checked="state.selected.assets.some(r=>r.id===asset.id)" :disabled="state.selected.assets.length>=20&&!state.selected.assets.some(r=>r.id===asset.id)" @change="state.toggle('assets',asset,($event.target as HTMLInputElement).checked)"/>
          <span class="asset-title" :title="asset.title||'—'">{{ asset.title||'—' }}</span>
        </label>
        <p class="asset-path" :title="asset.path">{{ asset.path }}</p>
        <p>{{ label(asset.state) }} · {{ label(asset.artifactsState) }} · {{ asset.revoked?'文件播放已撤销':'文件播放未撤销' }}</p>
        <div class="media-actions">
          <el-button @click="inspect(asset)">查看资产详情</el-button>
          <el-button @click="edit([],asset)">修正身份 / 季集</el-button>
          <ElDropdown trigger="click" popper-class="media-dropdown">
            <el-button>更多</el-button>
            <template #dropdown>
              <ElDropdownMenu>
                <ElDropdownItem :disabled="!asset.fileScoped" @click="ask(asset,'revoke')">撤销文件播放</ElDropdownItem>
                <ElDropdownItem @click="ask(asset,'rotate')">轮换播放凭据</ElDropdownItem>
              </ElDropdownMenu>
            </template>
          </ElDropdown>
        </div>
 </div>
 </article>
 </div>
      <PagePagination v-model:current-page="state.page.assets" v-model:page-size="state.sizes.assets" :total="state.total.assets" layout="prev,pager,next,total" @current-change="pageChange('assets')"/>
    </section>
    <div v-show="tab==='history'" class="organize-history-host">
      <HistoryPanel/>
    </div>
    <el-alert v-if="actionError" :title="errorText(actionError)" type="error" :closable="false"/>
    <AsyncResultBanner v-if="phase" :status="phase" message="操作结果需重新读取核对，不重复提交同一操作。" @refresh="state.refresh"/>
    <IdentityEditor ref="editor" @changed="state.changed" @editing="editing"/>
    <ConfirmAction v-model:visible="confirmOpen" :title="action?.type==='rotate'?'轮换播放凭据':'撤销文件播放'" :description="action?.type==='rotate'?'旧凭据将失效，STRM 重写通过恢复日志执行，不覆盖手工文件。':'固定链接将失效；此前已签发的临时 CDN 链接可能短期仍有效。'" impact-scope="不删除云端媒体或本地做种源；后续结果以恢复记录和最新资产状态为准。" :loading="busy" class="media-dialog" @confirm="apply"/>
    <DetailDrawer :key="assetDetail?.id" v-model:visible="detailsOpen" title="资产与输出详情" :technical-data="{asset:assetDetail,outputs,operations}" size="large" class="media-detail">
      <h3>{{ assetDetail?.title||'—' }}</h3>
 <ImageEditor v-if="assetDetail" :item-id="assetDetail.id" @saved="state.load('assets')"/>
      <p>{{ assetDetail?.path }}</p>
      <StateView v-if="detailError" :state="sourceState(detailError)" title="部分输出或历史未取得" :description="errorText(detailError)"/>
      <h3>当前输出与 STRM</h3>
      <p v-for="item in outputs?.outputs||[]" :key="item.path">{{ item.kind==='strm'?'STRM':item.kind==='nfo'?'NFO':'输出文件' }} · {{ item.path }}</p>
      <p v-if="!outputs">尚未取得匹配的输出清单，不据此推断输出已删除。</p>
      <h3>按当前文件名检索的文件操作记录</h3>
      <p v-for="item in operations" :key="item.id">{{ label(item.state) }} · {{ item.source }} → {{ item.destination||'—' }}</p>
      <RouterLink :to="{path:'/organize',query:{tab:'history',kind:'file',q:assetDetail?.path?.split('/').pop()}}" @click="detailsOpen=false">查看重命名与整理历史</RouterLink>
    </DetailDrawer>
  </div>
</template>

<style scoped>
.asset-collection{display:grid;gap:20px;margin-block:20px}.asset-collection-wall{grid-template-columns:repeat(auto-fill,minmax(200px,1fr))}.asset-card{min-width:0;margin:0;display:flex;flex-direction:column;gap:12px}.asset-poster-button{padding:0;border:0;background:none;cursor:pointer;width:100%;min-height:44px;color:inherit}.asset-poster-button:focus-visible{outline:2px solid var(--mt-color-primary);outline-offset:3px}.asset-card-body{min-width:0}.asset-title{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}.asset-path{overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;color:var(--mt-text-muted)}.asset-card .media-actions{gap:8px}.asset-card .media-actions .el-button{margin:0;min-height:44px}.asset-collection-list{grid-template-columns:minmax(0,1fr)}.asset-collection-list .asset-card{display:grid;grid-template-columns:76px minmax(0,1fr);align-items:start}.asset-card .media-consent{min-height:44px}.asset-card .media-consent input{flex-shrink:0}@media(max-width:680px){.asset-collection-wall{grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}.asset-collection-wall .asset-card{padding:12px}.asset-collection-wall .media-actions{display:grid;grid-template-columns:minmax(0,1fr)}.asset-collection-wall .media-actions>.el-dropdown{display:block}.asset-collection-wall .media-actions .el-button{width:100%;padding-inline:8px}.asset-collection-list .asset-card{grid-template-columns:60px minmax(0,1fr)}}
</style>
