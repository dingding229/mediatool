<script setup lang="ts">
import { pageResult } from './model';
import { computed, onMounted, reactive, ref } from 'vue';
import { request } from '../api/client';
import { useResource } from '../workflows/useResource';
import { copyOriginal } from './model';
import ResourceState from '../workflows/ResourceState.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
const source = ref('postgres'), table = ref(''), page = ref(1), pattern = ref('*'), cursor = ref('0'), cursorHistory = ref<string[]>([]), filter = ref(''), simple = ref(false), open = ref(false), detail = ref<any>(null), copyOpen = ref(false), copyContent = ref(''), copied = ref(''), tables = reactive(useResource<any[]>(() => request('/data/postgres/tables'), [])), data = reactive(useResource<any>(async () => {
  if (source.value === 'postgres') {
    if (!tables.data.length)
      await tables.load();
    if (tables.error)
      throw tables.error;
    if (!table.value)
      table.value = tables.data[0]?.name || '';
    if (!table.value)
      return { items: [], total: 0 };
    return request(`/data/postgres/tables/${encodeURIComponent(table.value)}?page=${page.value}&pageSize=25`).then(pageResult);
  }
  return request(`/data/redis/keys?cursor=${cursor.value}&pattern=${encodeURIComponent(pattern.value)}`).then(pageResult);
})), detailState = reactive(useResource<any>(() => request(`/data/redis/value?key=${encodeURIComponent(String(detail.value?.key))}`)));
const visible = computed(() => data.error || data.loading ? [] : (data.data?.items || []).filter((r: any) => JSON.stringify(r).toLowerCase().includes(filter.value.toLowerCase())));
function load() {
  return data.load();
}
function change() {
  open.value = false;
  detail.value = null;
  page.value = 1;
  cursor.value = '0';
  cursorHistory.value = [];
  filter.value = '';
  data.data = null;
  void load();
}
async function inspect(row: any) {
  detail.value = row;
  open.value = true;
  if (source.value === 'redis')
    await detailState.load();
}
async function copy() {
  copied.value = await copyOriginal(visible.value.map((r: any) => JSON.stringify(r)).join('\n'), s => {
    copyContent.value = s;
    copyOpen.value = true;
  });
}
function next() {
  cursorHistory.value.push(cursor.value);
  cursor.value = data.data.cursor;
  void load();
}
function previous() {
  cursor.value = cursorHistory.value.pop() || '0';
  void load();
}
onMounted(load);
defineExpose({ load, change, source, table, page, cursor, cursorHistory, pattern, filter, simple, visible, data, tables, inspect, detailState, copy, next, previous });
</script>
<template>
  <section class="media-surface">
    <header class="media-section-heading">
      <h2>只读数据浏览</h2>
      <div class="media-actions">
        <el-button @click="load">刷新数据</el-button>
        <el-button :disabled="!visible.length" @click="copy">复制当前页</el-button>
      </div>
    </header>
    <p>仅管理员可读，显示完整原始值（包括密文、密码哈希和 URL）；没有执行语句或删除入口，请勿公开分享。</p>
    <div class="u5-filters">
      <label>
        数据库来源
        <select v-model="source" aria-label="数据库来源" @change="change">
          <option value="postgres">PostgreSQL</option>
          <option value="redis">Redis</option>
        </select>
      </label>
      <label v-if="source==='postgres'">
        数据表
        <select v-model="table" aria-label="PostgreSQL数据表" @change="change">
          <option v-for="t in tables.data" :key="t.name" :value="t.name">{{ t.name }} · 约 {{ t.estimatedRows??'—' }} 行</option>
        </select>
      </label>
      <label v-else>
        键模式
        <input v-model="pattern" aria-label="Redis键筛选" @keyup.enter="change"/>
      </label>
      <label>
        筛选当前页
        <input v-model="filter" aria-label="筛选当前页数据"/>
      </label>
      <el-button v-if="source==='redis'" @click="change">筛选键</el-button>
    </div>
    <label class="u5-check">
      <input v-model="simple" type="checkbox" aria-label="简约模式"/>
      简约模式（点击查看完整记录）
    </label>
    <ResourceState :loading="data.loading" :error="data.error" :empty="!data.loading&&!data.error&&!!data.data&&!visible.length" empty-title="当前页没有符合条件的数据" @reload="load"/>
    <p role="status">{{ copied }}</p>
    <div aria-label="数据库内容">
      <button v-for="(row,index) in visible" :key="index" class="u6-data-row" @click="inspect(row)">
        <strong>{{ source==='redis'?row.key:'第 '+((page-1)*25+Number(index)+1)+' 行' }}</strong>
        <pre class="u6-raw">{{ JSON.stringify(simple?source==='redis'?{key:row.key,type:row.type,ttlSeconds:row.ttlSeconds}:Object.fromEntries(Object.entries(row).slice(0,4)):row,null,2) }}</pre>
      </button>
    </div>
    <p v-if="source==='postgres'">{{ table||'尚无数据表' }} · 共 {{ data.loading||data.error?'—':data.data?.total??'—' }} 行 · 每页25行；实时变化可能影响页序。</p>
    <p v-else>当前游标返回 {{ data.loading||data.error?'—':data.data?.items.length??'—' }} 个键；数据库共 {{ data.data?.total??'—' }} 个键（非筛选匹配总数）。游标扫描可能返回空页或重复键。</p>
    <el-pagination v-if="source==='postgres'&&data.data&&!data.error" v-model:current-page="page" :page-size="25" :total="data.data.total" layout="prev,pager,next" @current-change="load"/>
    <div v-else-if="source==='redis'" class="media-actions">
      <el-button :disabled="!cursorHistory.length||data.loading" @click="previous">上一游标页</el-button>
      <el-button :disabled="!data.data||data.data.cursor==='0'||data.loading||!!data.error" @click="next">下一游标页</el-button>
    </div>
    <DetailDrawer v-model:visible="open" title="只读格式化数据" class="media-detail">
      <ResourceState v-if="source==='redis'" :loading="detailState.loading" :error="detailState.error" @reload="detailState.load"/>
      <pre v-if="source==='postgres'||!detailState.error&&!detailState.loading" class="u6-raw">{{ JSON.stringify(source==='redis'?detailState.data:detail,null,2) }}</pre>
    </DetailDrawer>
    <el-dialog v-model="copyOpen" title="复制原始数据" width="min(740px,94vw)" class="media-dialog u5-dialog">
      <p>复制失败，请手动选择下方完整原始数据。</p>
      <textarea :value="copyContent" rows="14" readonly aria-label="待复制数据" class="u6-raw"/>
      <template #footer>
        <el-button @click="copyOpen=false">关闭</el-button>
      </template>
    </el-dialog>
  </section>
</template>
