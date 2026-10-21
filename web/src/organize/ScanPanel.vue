<script setup lang="ts">
import TaskControl from '../workflows/TaskControl.vue';
import {scheduleLabel} from '../presentation/labels';
import {usePageRefresh} from '../ui/page-refresh';
import { ref, onMounted, onUnmounted, watch } from 'vue';
import { APIError, post, request } from '../api/client';
import StateView from '../ui/StateView.vue';
import PreviewApprove from '../ui/PreviewApprove.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import { errorText, date } from '../workflows/readiness';
import { sourceState } from '../media/model';
import { scanPercent } from '../files/workbench';
import { label, rows, readPrivate, savePrivate } from './model';
const props = defineProps<{
  scans: any[];
  schedules: any[];
  scanError?: unknown;
  scheduleError?: unknown;
  scheduleLoading?: boolean;
  loading?: boolean;
}>(), emit = defineEmits<{
  changed: [
  ];
}>();
const remembered = readPrivate('media-tool:scan-create:v1'), root = ref(remembered?.root || ''), useDefault = ref(!remembered?.root), roots = ref<any[]>([]), connection = ref(''), rootError = ref<unknown>(null), preview = ref<any>(null), busy = ref(false), error = ref<unknown>(null), pending = ref<any>(remembered), approveOpen = ref(false), detailOpen = ref(false), detail = ref<any>(null), cursorPage = ref(1), scanPage = ref(1), step = ref(remembered ? 3 : 1), controlTarget = ref<any>(null), controlOpen = ref(false);
const scheduleOpen = ref(false), scheduleBusy = ref(false), conflict = ref<any>(null), form = ref<any>({ id: '', root: '', enabled: false, watchChanges: false, cron: '0 * * * *', revision: 0 });
const controlPending = ref<any>(readPrivate('media-tool:scan-control:v1'));
let version = 0, detailVersion = 0, disposed = false;
watch(() => [root.value, useDefault.value, connection.value], () => {
  version++;
  preview.value = null;
  if (!pending.value)
    step.value = 1;
});
async function readRoots() {
  try {
    roots.value = rows(await request('/files/roots')).filter(r => r.kind === 'cloud');
    rootError.value = null;
  }
  catch (e) {
    rootError.value = e;
  }
}
async function rangePreview() {
  const current = ++version;
  busy.value = true;
  error.value = null;
  try {
    const chosen = roots.value.find(r => r.id === connection.value);
    let listing: any = null;
    if (chosen && !useDefault.value) {
      if (root.value !== chosen.path && !root.value.startsWith(chosen.path + '/'))
        throw Error('扫描路径不在所选云盘目录内。');
      const path = root.value === chosen.path ? '.' : root.value.slice(chosen.path.length + 1);
      listing = await request(`/files?connectionId=${encodeURIComponent(chosen.id)}&path=${encodeURIComponent(path)}&page=1&pageSize=50`);
    }
    if (current === version && !disposed) {
      preview.value = { root: useDefault.value ? '' : root.value.trim(), listing, connection: chosen };
      step.value = 2;
    }
  }
  catch (e) {
    if (current === version)
      error.value = e;
  }
  finally {
    if (current === version)
      busy.value = false;
  }
}
async function create() {
  if (!preview.value || pending.value || busy.value)
    return;
  const value = { root: preview.value.root, phase: 'reconciling', submittedAt: Date.now() };
  pending.value = value;
  savePrivate('media-tool:scan-create:v1', value);
  busy.value = true;
  try {
    const out: any = await post('/library/scans', { root: value.root });
    pending.value = { ...value, phase: 'queued', id: out.id };
    savePrivate('media-tool:scan-create:v1', pending.value);
    step.value = 3;
    approveOpen.value = false;
    emit('changed');
  }
  catch (e) {
    error.value = e;
    approveOpen.value = false;
    if (e instanceof APIError && e.status >= 400 && e.status < 500) {
      pending.value = null;
      savePrivate('media-tool:scan-create:v1', null);
    }
  }
  finally {
    busy.value = false;
  }
}
async function inspect(scan: any, reset = true) {
  const current = ++detailVersion;
  if (reset)
    cursorPage.value = 1;
  detailOpen.value = true;
  error.value = null;
  try {
    const out: any = await request(`/library/scans/${scan.id}?page=${cursorPage.value}&pageSize=25`);
    if (disposed || current !== detailVersion)
      return;
    detail.value = out;
    if (controlPending.value?.id === scan.id) {
      controlPending.value = null;
      savePrivate('media-tool:scan-control:v1', null);
    }
    if (pending.value && (pending.value.id === scan.id || !pending.value.id && Date.parse(out.scan.createdAt) >= pending.value.submittedAt - 5000 && (!pending.value.root || out.scan.root === pending.value.root))) {
      pending.value = { ...pending.value, id: scan.id, phase: 'queued' };
      savePrivate('media-tool:scan-create:v1', pending.value);
      if (out.scan.state === 'completed')
        step.value = 4;
    }
  }
  catch (e) {
    error.value = e;
  }
}
function askControl(scan: any, action: string) {
  if (controlPending.value) {
    error.value = Error('上次控制结果未知，请先查看该扫描的目录证据再操作。');
    return;
  }
  controlTarget.value = { scan, action };
  controlOpen.value = true;
}
async function control() {
  if (!controlTarget.value || busy.value)
    return;
  controlPending.value = { id: controlTarget.value.scan.id };
  savePrivate('media-tool:scan-control:v1', controlPending.value);
  busy.value = true;
  try {
    await post(`/library/scans/${controlTarget.value.scan.id}/${controlTarget.value.action}`, {});
    controlPending.value = null;
    savePrivate('media-tool:scan-control:v1', null);
    emit('changed');
    controlOpen.value = false;
  }
  catch (e) {
    error.value = e;
    controlOpen.value = false;
  }
  finally {
    busy.value = false;
  }
}
function editSchedule(row: any = null) {
  form.value = row ? { id: row.id, root: row.root, enabled: row.enabled, watchChanges: !!row.watchChanges, cron: row.cron, revision: row.revision } : { id: '', root: useDefault.value ? '' : root.value, enabled: false, watchChanges: false, cron: '0 * * * *', revision: 0 };
  conflict.value = null;
  error.value = null;
  const uncertain = readPrivate('media-tool:scan-schedule-write:v1');
  if (uncertain) {
    form.value = uncertain;
    conflict.value = { revision: null, root: '上次保存结果未知', cron: '请先核对上次保存', enabled: false };
  }
  scheduleOpen.value = true;
}
async function saveSchedule() {
  if (scheduleBusy.value || conflict.value)
    return;
  scheduleBusy.value = true;
  savePrivate('media-tool:scan-schedule-write:v1', form.value);
  error.value = null;
  try {
    await post('/library/scan-schedules', { ...form.value });
    savePrivate('media-tool:scan-schedule-write:v1', null);
    scheduleOpen.value = false;
    emit('changed');
  }
  catch (e) {
    error.value = e;
    if (e instanceof APIError && e.status >= 400 && e.status < 500 && e.status !== 409)
      savePrivate('media-tool:scan-schedule-write:v1', null);
    if (!(e instanceof APIError) || e.status >= 500 || e.status === 409 || e.code === 'schedule_invalid') {
      conflict.value = { revision: null, root: '尚未读取，请再次核对', cron: '', enabled: false };
      try {
        const all: any = await request('/library/scan-schedules');
        conflict.value = rows(all).find(s => form.value.id ? s.id === form.value.id : s.root === form.value.root && s.cron === form.value.cron) || { ...form.value, revision: null };
        if (e instanceof APIError && e.code === 'schedule_invalid' && (!conflict.value?.id || conflict.value.revision === form.value.revision))
          conflict.value = null;
        else if (e instanceof APIError && (e.status === 409 || e.code === 'schedule_invalid'))
          error.value = Error('配置已被修改，已重新读取。你的输入已保留，请对照后采用最新修订。');
      }
      catch {
        error.value = Error('配置冲突后读取失败，输入已保留，请稍后核对。');
      }
    }
  }
  finally {
    scheduleBusy.value = false;
  }
}
async function checkSchedule() {
  try {
    const all = rows(await request('/library/scan-schedules'));
    conflict.value = all.find(s => form.value.id ? s.id === form.value.id : s.root === form.value.root && s.cron === form.value.cron) || { revision: null, root: '尚未确认保存记录', cron: '保留输入，请稍后再核对', enabled: false };
  }
  catch (e) {
    error.value = e;
  }
}
onMounted(readRoots);
onUnmounted(() => {
  disposed = true;
  version++;
});
usePageRefresh(()=>{emit('changed');return readRoots()}, 'scans');
</script>
<template>
  <section class="media-surface">
    <h2>扫描目录</h2>
    <p>范围预览不计算整棵目录树的变更；真实数量与完整覆盖以扫描记录为准。</p>
    <PreviewApprove :current-step="step">
      <div class="media-form">
        <label>
          云盘目录来源
          <el-select v-model="connection" aria-label="扫描目录来源" :disabled="!!pending" @change="root=roots.find(r=>r.id===connection)?.path||'';useDefault=false">
            <el-option v-for="entry in roots" :key="entry.id" :value="entry.id" :label="entry.name+' · '+entry.path"/>
          </el-select>
        </label>
        <el-checkbox v-model="useDefault" :disabled="!!pending">使用服务端默认云盘根目录</el-checkbox>
        <label v-if="!useDefault">
          扫描路径
          <el-input v-model="root" aria-label="扫描路径" :disabled="!!pending"/>
        </label>
        <StateView v-if="rootError" :state="sourceState(rootError)" title="目录浏览暂不可用" description="可以手工输入扫描范围；不会把未取得的数据当作完整预览。" @retry="readRoots"/>
        <el-button :loading="busy" :disabled="!!pending||(!useDefault&&!root.trim())" @click="rangePreview">预览扫描范围</el-button>
        <section v-if="preview" aria-label="扫描范围预览">
          <h3>{{ preview.root||'服务端默认云盘根目录' }}</h3>
          <p>只读查看范围，不移动文件；执行时由服务端按当前连接和授权校验。</p>
          <p v-if="preview.listing">此层已取得 {{ preview.listing.items?.length??'—' }} 项 · 总数 {{ preview.listing.total??'—' }}；这不是全树文件总数。</p>
          <p v-else>未预读文件清单，不推断文件数、缺失项或完整覆盖。</p>
          <el-button type="primary" :disabled="!!pending||busy" @click="approveOpen=true">确认范围并开始扫描</el-button>
        </section>
        <AsyncResultBanner v-if="pending" :status="pending.phase" :message="pending.id?'扫描已排队，尚未确认完成。':'扫描回执未知，先核对下方记录，不重复创建。'" @refresh="emit('changed')"/>
        <el-button v-if="pending?.id" @click="inspect({id:pending.id})">核对已受理扫描</el-button>
        <el-button v-if="pending?.id&&detail?.scan?.id===pending.id" @click="pending=null;savePrivate('media-tool:scan-create:v1',null);preview=null;step=1">开始另一项扫描</el-button>
      </div>
    </PreviewApprove>
    <el-alert v-if="error&&!scheduleOpen" :title="errorText(error)" type="error" :closable="false"/>
  </section>
  <section class="media-surface">
    <h2>扫描进度与目录记录</h2>
    <StateView v-if="scanError" :state="sourceState(scanError)" title="扫描记录读取失败" @retry="emit('changed')"/>
    <p v-else-if="loading&&!scans.length">正在读取扫描记录…</p>
    <p v-else-if="!scans.length">尚无扫描记录</p>
    <article v-for="scan in scans.slice((scanPage-1)*8,scanPage*8)" :key="scan.id" class="organize-record">
      <h3>{{ scan.root }}</h3>
      <p>{{ label(scan.state) }} · {{ scan.files??'—' }} 个文件 · 已知目录 {{ scan.completedDirectories??'—' }} / {{ scan.directories??'—' }}</p>
      <el-progress v-if="scan.directories>0" :percentage="scanPercent(scan.completedDirectories||0,scan.directories)"/>
      <p>{{ scan.currentDirectory||'—' }} · {{ scan.message||'—' }}</p>
      <div class="media-actions">
        <el-button @click="inspect(scan)">查看目录记录</el-button>
        <el-button v-if="['pending','running'].includes(scan.state)" :disabled="!!scanError||busy" @click="askControl(scan,'pause')">暂停扫描</el-button>
        <el-button v-if="['paused','failed'].includes(scan.state)" :disabled="!!scanError||busy" @click="askControl(scan,'resume')">重新完整对账</el-button>
        <TaskControl kind="scans" :id="scan.id" :state="scan.state" @changed="emit('changed')"/>
      </div>
    </article>
    <PagePagination v-if="scans.length>8" v-model:current-page="scanPage" :page-size="8" :total="scans.length" layout="prev,pager,next"/>
    <p>扫描中断或云盘内容变化时，不会把未扫描到的文件直接标记为缺失；恢复后会重新检查。</p>
  </section>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>定时扫描</h2>
      <el-button @click="editSchedule()">新增定时扫描</el-button>
    </header>
    <StateView v-if="scheduleLoading&&!schedules.length" state="loading" title="正在读取扫描计划…"/>
    <StateView v-else-if="scheduleError" :state="sourceState(scheduleError)" title="扫描计划读取失败" @retry="emit('changed')"/>
    <StateView v-else-if="!schedules.length" state="empty" title="尚无定时扫描" description="新增计划默认停用，核对目录与周期后再启用。"/>
    <article v-for="schedule in schedules" :key="schedule.id" class="organize-record">
      <h3>{{ schedule.root }}</h3>
      <p>{{ scheduleLabel(schedule.cron) }} · {{ schedule.enabled?'已启用':'未启用' }} · 下次 {{ date(schedule.nextRunAt) }}</p>
      <p>文件变化推送：{{ label(schedule.watchState) }} · {{ schedule.watchMessage||'—' }}</p>
      <p>{{ schedule.lastError||'—' }}</p>
      <el-button @click="editSchedule(schedule)">编辑</el-button>
    </article>
  </section>
  <DetailDrawer v-model:visible="detailOpen" title="扫描目录覆盖" :technical-data="detail" class="media-detail">
    <p>{{ detail?.scan?.root }} · {{ label(detail?.scan?.state) }}</p>
    <p v-for="directory in detail?.directories||[]" :key="directory.path">{{ directory.path }} · {{ label(directory.state) }}</p>
    <PagePagination v-if="detail" v-model:current-page="cursorPage" :page-size="25" :total="detail.scan.directories||0" layout="prev,pager,next" @current-change="inspect(detail.scan,false)"/>
  </DetailDrawer>
  <ConfirmAction v-model:visible="approveOpen" title="开始目录扫描" :description="`将扫描「${preview?.root||'服务端默认目录'}」。`" impact-scope="更新索引、候选与缺失对账状态，不移动媒体或做种源；未完成覆盖不作为缺失证据。" confirm-text="确认开始扫描" :loading="busy" class="media-dialog" @confirm="create"/>
  <ConfirmAction v-model:visible="controlOpen" title="扫描控制" :description="controlTarget?.action==='resume'?'从根目录重新完整对账，不复用不完整覆盖。':'暂停或取消不会把未扫描目录标为缺失。'" impact-scope="只影响此扫描，不删除媒体或输出。" :loading="busy" class="media-dialog" @confirm="control"/>
  <el-dialog v-model="scheduleOpen" title="定时扫描设置" width="min(650px,94vw)" :close-on-click-modal="false" class="media-dialog">
    <div class="media-form">
      <label>
        扫描根目录
        <el-input v-model="form.root" aria-label="定时扫描根目录"/>
      </label>
      <label>
        Cron
        <el-input v-model="form.cron" aria-label="定时扫描Cron"/>
      </label>
      <el-switch v-model="form.enabled" aria-label="启用定时扫描"/>
      <span>启用定时扫描</span>
      <el-switch v-model="form.watchChanges" aria-label="启用CloudDrive2文件变化推送"/>
      <span>启用CloudDrive2文件变化推送</span>
      <p>变化事件只触发重新扫描，不直接删除媒体或输出。</p>
      <el-alert v-if="error" :title="errorText(error)" type="error" :closable="false"/>
      <section v-if="conflict">
        <el-button @click="checkSchedule">核对上次保存</el-button>
        <p>当前配置：{{ conflict.root }} · {{ scheduleLabel(conflict.cron) }} · {{ conflict.enabled?'启用':'停用' }}</p>
        <el-button :disabled="conflict.revision==null" @click="form.id=conflict.id||form.id;form.revision=conflict.revision;conflict=null;savePrivate('media-tool:scan-schedule-write:v1',null)">保留输入并采用最新修订</el-button>
      </section>
    </div>
    <template #footer>
      <el-button @click="scheduleOpen=false">取消</el-button>
      <el-button type="primary" :loading="scheduleBusy" :disabled="!!conflict" @click="saveSchedule" class="mt-save-button">保存定时扫描</el-button>
    </template>
  </el-dialog>
</template>
