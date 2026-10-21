<script setup lang="ts">
import { requestUUID } from '../utils/request-uuid';
import { ElDropdown, ElDropdownMenu, ElDropdownItem } from 'element-plus';
import { computed, onMounted, ref } from 'vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import StateView from '../ui/StateView.vue';
import RulePicker from './RulePicker.vue';
import RevisionFeedback from './RevisionFeedback.vue';
import { clone, useRevision } from './revision';
import { genreChoices, countryChoices, languageChoices, languageLabel, genres } from '../classification/options';
interface Rule {
  id: string;
  name: string;
  directory: string;
  enabled: boolean;
  genreIds: number[];
  countries: string[];
  languages: string[];
}
interface Group {
  id: string;
  name: string;
  directory: string;
  enabled: boolean;
  genreIds: number[];
  mediaType: string;
  rules: Rule[];
}
const settings = useRevision<{
  groups: Group[];
}>('/classification'), data = settings.draft, dialog = ref(false), groupIndex = ref(-1), ruleIndex = ref(-1), groupMode = ref(true), removal = ref<{
  g: number;
  r: number;
}>(), error = ref('');
const form = ref({ name: '', directory: '', enabled: true, mediaType: 'movie', genreIds: [] as number[], countries: [] as string[], languages: [] as string[] });
const genreOptions = computed(() => genreChoices(groupMode.value ? form.value.mediaType : data.value?.groups[groupIndex.value]?.mediaType || 'movie', form.value.genreIds));
const typeLabel = (type: string) => ({ movie: '电影', series: '电视剧', anime: '动漫' }[type] || type || '—');
const genreLabel = (ids: number[] | null) => (ids || []).map(id => genres.find(x => x.value === id)?.label || String(id)).join('、') || '不限题材';
const countryLabel = (codes: string[] | null) => (codes || []).map(id => countryChoices(codes || []).find(x => x.value === id)?.label || id).join('、');
function edit(g: number, r = -1, group = true) {
  groupIndex.value = g;
  ruleIndex.value = r;
  groupMode.value = group;
  error.value = '';
  const item = group ? data.value?.groups[g] : data.value?.groups[g]?.rules[r];
  form.value = { name: item?.name || '', directory: item?.directory || '', enabled: item?.enabled ?? true, mediaType: (item as Group)?.mediaType || data.value?.groups[g]?.mediaType || 'movie', genreIds: [...item?.genreIds || []], countries: [...(item as Rule)?.countries || []], languages: [...(item as Rule)?.languages || []] };
  dialog.value = true;
}
function apply() {
  if (!data.value)
    return;
  if (!form.value.name.trim()) {
    error.value = '请填写分类名称。';
    return;
  }
  const f = clone(form.value), base = { id: `rule-${requestUUID()}`, name: f.name.trim(), directory: f.directory || f.name.trim(), enabled: f.enabled, genreIds: f.genreIds };
  if (groupMode.value) {
    const old = data.value.groups[groupIndex.value];
    const next = { ...base, id: old?.id || base.id, mediaType: f.mediaType, rules: old?.rules || [] };
    if (old)
      data.value.groups[groupIndex.value] = next;
    else
      data.value.groups.push(next);
  }
  else {
    const rows = (data.value.groups[groupIndex.value]!.rules ||= []), old = rows[ruleIndex.value], next = { ...base, id: old?.id || base.id, countries: f.countries, languages: f.languages };
    if (old)
      rows[ruleIndex.value] = next;
    else
      rows.push(next);
  }
  dialog.value = false;
}
function move(g: number, r: number, delta: number) {
  const list = r < 0 ? data.value?.groups : data.value?.groups[g]?.rules;
  if (!list)
    return;
  const i = r < 0 ? g : r;
  if (i + delta < 0 || i + delta >= list.length)
    return;
  const row = list.splice(i, 1)[0]!;
  list.splice(i + delta, 0, row as Group & Rule);
}
function remove() {
  if (!removal.value)
    return;
  const { g, r } = removal.value;
  if (r < 0)
    data.value?.groups.splice(g, 1);
  else
    data.value?.groups[g]?.rules.splice(r, 1);
  removal.value = undefined;
}
onMounted(settings.load);
</script>
<template>
  <section class="settings-card" aria-label="目录分类">
    <header>
      <div>
        <h2>目录分类</h2>
        <p>按从上到下的顺序匹配，先选媒体分组，再按国家、题材和语言分流。</p>
      </div>
      <div class="settings-actions">
        <el-button :disabled="settings.busy.value" @click="settings.load">重新读取分类</el-button>
        <el-button :disabled="!data" @click="edit(-1)">添加分类</el-button>
      </div>
    </header>
    <RevisionFeedback :form="settings"/>
    <StateView v-if="data&&!data.groups.length" state="empty" title="尚未设置目录分类" description="添加电影、电视剧或动漫分组后，再添加分流规则。"/>
    <div v-if="data" class="settings-stack">
      <article v-for="(group,g) in data.groups" :key="group.id" class="classification-card" :aria-label="group.name">
        <header>
          <div>
            <span class="settings-index">{{g+1}}</span>
            <h3>{{group.name}}</h3>
            <p>{{typeLabel(group.mediaType)}} · {{genreLabel(group.genreIds)}} → {{group.directory||'—'}}</p>
          </div>
          <el-switch v-model="group.enabled" :aria-label="`启用${group.name}`"/>
        </header>
        <div class="settings-actions">
          <el-button @click="edit(g)">编辑分组</el-button>
          <el-button :aria-label="`为${group.name}添加规则`" @click="edit(g,-1,false)">添加规则</el-button>
          <el-dropdown trigger="click">
            <el-button :aria-label="`${group.name}更多操作`">更多</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item :disabled="g===0" @click="move(g,-1,-1)">上移</el-dropdown-item>
                <el-dropdown-item :disabled="g===data.groups.length-1" @click="move(g,-1,1)">下移</el-dropdown-item>
                <el-dropdown-item @click="removal={g,r:-1}">删除分类</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <ol class="classification-rules">
          <li v-for="(rule,r) in group.rules" :key="rule.id">
            <div>
              <strong>{{rule.name}}</strong>
              <p>{{genreLabel(rule.genreIds)}} · {{countryLabel(rule.countries)||'不限地区'}} · {{(rule.languages||[]).map(languageLabel).join('、')||'不限语言'}}</p>
              <p>目录：{{rule.directory||'—'}}</p>
            </div>
            <div class="settings-actions">
              <el-switch v-model="rule.enabled" :aria-label="`启用${rule.name}`"/>
              <el-button :aria-label="`编辑${rule.name}`" @click="edit(g,r,false)">编辑</el-button>
              <el-dropdown trigger="click">
                <el-button :aria-label="`${rule.name}更多操作`">更多</el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :disabled="r===0" @click="move(g,r,-1)">上移</el-dropdown-item>
                    <el-dropdown-item :disabled="r===group.rules.length-1" @click="move(g,r,1)">下移</el-dropdown-item>
                    <el-dropdown-item @click="removal={g,r}">删除规则</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </li>
        </ol>
        <p v-if="!group.rules?.length">暂无细分规则，匹配后直接使用分组目录。</p>
      </article>
    </div>
    <footer>
      <p>仅影响新确认的入库，不移动或删除已有媒体。</p>
      <el-button type="primary" :loading="settings.busy.value" :disabled="settings.conflict.value||!settings.changed.value" @click="settings.save(['groups'])">保存目录分类</el-button>
    </footer>
    <el-dialog class="settings-dialog" v-model="dialog" destroy-on-close :title="groupMode?'编辑媒体分组':'编辑分流规则'" width="min(700px,94vw)">
      <form class="settings-stack" @submit.prevent="apply">
        <p v-if="error" role="alert">{{error}}</p>
        <div class="settings-field">
          <label for="classification-name">分类名称</label>
          <el-input id="classification-name" v-model="form.name"/>
        </div>
        <div class="settings-field">
          <label for="classification-directory">目标目录</label>
          <el-input id="classification-directory" v-model="form.directory" placeholder="留空使用分类名称"/>
        </div>
        <div v-if="groupMode" class="settings-field">
          <label for="classification-type">媒体类型</label>
          <el-select popper-class="settings-option-list" id="classification-type" v-model="form.mediaType">
            <el-option label="电影" value="movie"/>
            <el-option label="电视剧" value="series"/>
          </el-select>
          <el-button @click="form.mediaType='series';form.genreIds=[16]">设为动漫分组</el-button>
          <small>动漫使用电视剧类型与动画题材。</small>
        </div>
        <el-switch v-model="form.enabled" aria-label="启用分类"/>
        <RulePicker v-model="form.genreIds" label="题材" :options="genreOptions"/>
        <template v-if="!groupMode">
          <RulePicker v-model="form.countries" label="国家或地区" :options="countryChoices(form.countries)"/>
          <RulePicker v-model="form.languages" label="原始语言" :options="languageChoices(form.languages)"/>
        </template>
        <div class="settings-actions">
          <el-button @click="dialog=false">取消</el-button>
          <el-button type="primary" native-type="submit">应用到草稿</el-button>
        </div>
      </form>
    </el-dialog>
    <ConfirmAction class="settings-dialog" :visible="!!removal" title="删除分类规则" description="只从草稿移除，保存后用于新入库。不会移动或删除已有媒体。" confirm-text="从草稿删除" @update:visible="v=>{if(!v)removal=undefined}" @confirm="remove"/>
  </section>
</template>
