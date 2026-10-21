<script setup lang="ts">
import { reactive, ref, onMounted, watch, nextTick } from 'vue';
import { useRoute } from 'vue-router';
import { ElDropdown, ElDropdownItem, ElDropdownMenu } from 'element-plus';
import { FolderOpened, Document } from '@element-plus/icons-vue';
import { useBrowser, permission } from './useBrowser';
import FileActionDialog from './FileActionDialog.vue';
import FileBatchDialog from '../workflows/FileBatchDialog.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { roles } from './workbench';
import { readPrivate, label } from '../organize/model';
import { size } from '../acquisition/model';
import { date, errorText } from '../workflows/readiness';
import { sourceState } from '../media/model';
const route = useRoute();
const browser = reactive(useBrowser()), action = ref<InstanceType<typeof FileActionDialog>>(), batch = ref<InstanceType<typeof FileBatchDialog>>(), historyOpen = ref(false);
function allowed(operation: string, row?: any, cap?: any) {
  return !browser.error && !browser.rootError && !browser.loading && !browser.rootsLoading && permission(operation, row, cap || browser.capability);
}
function act(operation: string, row: any = null) {
  if (readPrivate('media-tool:organize-batch:v1')) {
    void batch.value?.openFiles(browser.root, browser.selection.length ? browser.selection : [row || {}]);
    return;
  }
  void action.value?.open(operation, browser.root, browser.directory, row);
}
function organize() {
  if (readPrivate('media-tool:file-operation:v1')) {
    act('organize');
    return;
  }
  void batch.value?.openFiles(browser.root, browser.selection);
}
function selectPage() {
  browser.selection = [];
  for (const row of browser.shown.filter(r => !r.directory && permission('organize', r, browser.capability)).slice(0, 20))
    browser.select(row, true);
}
function history() {
  historyOpen.value = true;
  void browser.loadHistory();
}
async function selectedLibrary() {
  const kind = route.query.selectionKind, values = route.query.selection;
  if (!['candidates', 'assets'].includes(String(kind)))
    return;
  const ids = (Array.isArray(values) ? values : [values]).filter((x): x is string => typeof x === 'string' && /^[a-zA-Z0-9_-]{1,100}$/.test(x));
  if (!ids.length || ids.length > 20)
    return;
  await nextTick();
  void batch.value?.open(kind as 'candidates' | 'assets', ids.map(id => ({ id })));
}
onMounted(selectedLibrary);
watch(() => [route.query.selectionKind, route.query.selection], selectedLibrary);
defineExpose({ refresh: browser.refresh });
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>文件浏览</h2>
      <el-button @click="history">操作记录</el-button>
    </header>
    <div class="files-browser-toolbar">
      <label>
        本地与云盘目录
        <el-select v-model="browser.connection" aria-label="浏览根目录" :disabled="browser.rootsLoading" @change="browser.changeRoot">
          <el-option v-for="root in browser.roots" :key="root.id" :label="(root.kind==='local'?'本地 · ':'云盘 · ')+root.name" :value="root.id"/>
        </el-select>
      </label>
      <label>
        筛选当前页文件
        <el-input v-model="browser.filter" aria-label="筛选当前页文件" clearable/>
      </label>
      <el-button :disabled="!browser.root||!allowed('mkdir',undefined,browser.capability)" @click="act('mkdir')">新建文件夹</el-button>
    </div>
    <p v-if="browser.root">{{ browser.root.path }} · {{ roles[browser.capability?.role||'']||'目录权限待核对' }}</p>
    <p v-if="browser.capability?.reason">{{ browser.capability.reason }}</p>
    <nav class="files-breadcrumbs" aria-label="文件路径">
      <button v-for="crumb in browser.crumbs" :key="crumb.path" :aria-current="browser.directory===crumb.path?'location':undefined" @click="browser.navigate(crumb.path)">{{ crumb.name }}</button>
      <el-button v-if="browser.directory!=='.'" @click="browser.navigate(browser.directory.includes('/')?browser.directory.slice(0,browser.directory.lastIndexOf('/')):'.')">上级目录</el-button>
      <el-button :disabled="!browser.root" @click="browser.navigate('.media-tool-trash')">查看回收目录</el-button>
    </nav>
    <div class="media-actions">
      <el-button :disabled="browser.loading||!browser.shown.length" @click="selectPage">选择本页可整理文件（最多20项）</el-button>
      <el-button :disabled="!browser.selection.length" @click="browser.selection=[]">清空选择</el-button>
      <el-button :disabled="!browser.selection.length" @click="organize">批量整理预览（{{ browser.selection.length }}）</el-button>
    </div>
    <StateView v-if="browser.rootsLoading||browser.loading&&!browser.entries.length" state="loading" title="正在读取目录…"/>
    <StateView v-else-if="browser.rootError||browser.error" :state="sourceState(browser.rootError||browser.error)" title="目录暂不可用" :description="errorText(browser.rootError||browser.error)" @retry="browser.refresh"/>
    <StateView v-else-if="!browser.roots.length" state="empty" title="尚无可浏览目录" description="先配置本地工作区或云盘连接。"/>
    <StateView v-else-if="!browser.shown.length" state="empty" title="当前页没有匹配的文件"/>
    <ul class="files-list" aria-label="文件列表">
      <li v-for="file in browser.shown" :key="file.path" class="files-entry">
        <label class="files-selection">
          <input type="checkbox" :aria-label="'选择 '+file.name" :disabled="file.directory||!allowed('organize',file,browser.capability)||browser.selection.length>=20&&!browser.selection.some(r=>r.path===file.path)" :checked="browser.selection.some(r=>r.path===file.path)" @change="browser.select(file,($event.target as HTMLInputElement).checked)"/>
        </label>
        <component :is="file.directory?FolderOpened:Document" aria-hidden="true" class="files-entry-icon"/>
        <div class="files-entry-copy">
          <button v-if="file.directory" :disabled="!!file.unsupported" @click="browser.navigate(file.path)">{{ file.name }}</button>
          <strong v-else>{{ file.name }}</strong>
          <span>{{ file.unsupported?'不支持的文件类型':file.directory?'文件夹':'文件' }} · {{ file.directory?'—':size(file.size) }}</span>
        </div>
        <el-dropdown trigger="click" popper-class="media-dropdown">
          <el-button :aria-label="file.name+' 更多'">更多</el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="file.directory" :disabled="!!file.unsupported" @click="browser.navigate(file.path)">进入目录</el-dropdown-item>
              <el-dropdown-item :disabled="!allowed('rename',file,browser.capability)" @click="act('rename',file)">重命名</el-dropdown-item>
              <el-dropdown-item :disabled="!allowed('move',file,browser.capability)" @click="act('move',file)">移动</el-dropdown-item>
              <el-dropdown-item :disabled="file.directory||!allowed('organize',file,browser.capability)" @click="act('organize',file)">影视整理</el-dropdown-item>
              <el-dropdown-item divided :disabled="!allowed('trash',file,browser.capability)" @click="act('trash',file)">删除（可恢复）</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </li>
    </ul>
    <el-pagination v-if="browser.total>50" v-model:current-page="browser.page" :page-size="50" :total="browser.total" layout="prev,pager,next,total" @current-change="browser.selection=[];browser.load()"/>
  </section>
  <DetailDrawer v-model:visible="historyOpen" title="文件操作记录" :technical-data="browser.history" class="media-detail">
    <el-button :loading="browser.historyLoading" @click="browser.loadHistory">刷新操作记录</el-button>
    <StateView v-if="browser.historyLoading" state="loading" title="正在读取操作记录…"/>
    <StateView v-else-if="browser.historyError" :state="sourceState(browser.historyError)" title="操作记录读取失败" :description="errorText(browser.historyError)" @retry="browser.loadHistory"/>
    <StateView v-else-if="!browser.history.length" state="empty" title="暂无文件操作记录"/>
    <article v-for="item in browser.history" :key="item.id" class="organize-record">
      <h3>{{ item.plan?.items?.[0]?.source?.name||'文件变更' }}</h3>
      <p>{{ label(item.state) }} · {{ date((item as any).updatedAt) }}</p>
      <p>{{ item.message||'—' }}</p>
      <el-button @click="historyOpen=false;action?.inspect(item)">查看步骤与核对</el-button>
    </article>
  </DetailDrawer>
  <FileActionDialog ref="action" @changed="browser.refresh();browser.loadHistory()"/>
  <FileBatchDialog ref="batch" @changed="browser.refresh"/>
</template>
