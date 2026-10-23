<script setup lang="ts">
import {usePageNumber} from '../ui/page-query';
import {beijingTime} from '../presentation/time';
import {usePageRefresh} from '../ui/page-refresh';
import './connections.css';
import { computed, ref, watch } from 'vue';
import { ElMessageBox,ElDropdown,ElDropdownMenu,ElDropdownItem } from 'element-plus';
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import StateView from '../ui/StateView.vue';
import ConfirmAction from '../ui/ConfirmAction.vue';
import AsyncResultBanner from '../ui/AsyncResultBanner.vue';
import StatusBadge from '../components/StatusBadge.vue';
import Official115AuthDialog from '../components/Official115AuthDialog.vue';
import { useConnections } from './useConnections';
import { useSecretReveal } from './useSecretReveal';
import { stateFor, errorText } from '../workflows/readiness';
import { kinds, safeSettings } from './model';
const props = defineProps<{
  allowedKinds: string[];
  title: string;
}>();
const { rows, loading, loadError, error, editing, saving, notice, conflict, saved, blocked, form, load, create, edit, save, useRevision, test, testState, testing, reconcile, authorize, authOpen, authId, askRemove, removeOpen, removeTarget, removing, remove } = useConnections();
const reveal = useSecretReveal(() => ({ id: form.id, revision: form.revision }));
watch(() => [editing.value, form.id, form.revision, saving.value], () => reveal.hideAll());
const connectionPage=usePageNumber('connectionPage');
const selectedKind = ref(props.allowedKinds[0] ?? 'emby'), visible = computed(() => rows.value.filter(r => props.allowedKinds.includes(r.kind)));
async function approveRevision() {
  try {
    await ElMessageBox.confirm('将保留你的输入，并基于刚读取的最新修订保存。请先核对差异。', '确认采用最新修订', { confirmButtonText: '保留输入并确认', cancelButtonText: '取消', type: 'warning' });
    useRevision();
  }
  catch {
  }
}
usePageRefresh(load);
</script>
<template>
<section class="connection-manager" :aria-label="title">
  <header class="connection-section-heading">
    <div>
      <h2>{{ title }}</h2>
      <p>保存前验证连接；密钥默认隐藏，可主动查看已保存值；留空保留原值。</p>
    </div>
    <div class="connection-buttons">

      <el-select v-if="allowedKinds.length>1" v-model="selectedKind" aria-label="新增服务类型">
        <el-option v-for="kind in allowedKinds" :key="kind" :value="kind" :label="kinds[kind]?.name" />
      </el-select>
      <el-button v-if="!(selectedKind==='tmdb'&&visible.length)" @click="create(selectedKind)">添加连接</el-button>
    </div>
  </header>
  <el-alert v-if="notice" :title="notice" type="info" :closable="false" />
  <el-alert v-if="error&&!editing&&!loadError" :title="error" type="error" :closable="false" />
  <AsyncResultBanner v-if="testState" :status="testState" :message="notice" @refresh="reconcile" />
<div class="f5-connection-rows">
  <StateView v-if="loading" state="loading" title="正在读取已保存连接…" />
  <StateView v-else-if="loadError" :state="stateFor(loadError)" title="连接列表暂不可用" :description="errorText(loadError)" @retry="load" />
  <StateView v-else-if="!visible.length&&!error" state="empty" title="尚未配置服务连接" description="先添加配置，完成检测和授权后再明确启用。" />
  <article v-for="row in visible.slice((connectionPage-1)*10,connectionPage*10)" :key="row.id" class="saved-connection">
    <div>
      <h3>{{ row.name }}</h3>
      <p>{{ kinds[row.kind]?.name }} · {{ row.enabled?'已启用':'未启用' }}</p>
      <StatusBadge :state="row.readiness?.state||row.lastCheckState" />
      <p v-if="row.kind==='open115'">上次健康检测：{{beijingTime(row.lastCheckedAt)}} · {{row.lastCheckMessage||'尚未检测'}}</p>
      <p>{{ row.secretFields?.length?'凭据已设置':'凭据未设置' }}</p>
    </div>
    <div class="connection-buttons">
      <el-button @click="edit(row)">{{ row.kind==='tmdb'?'编辑TMDB Token':'编辑' }}</el-button>
      <el-button :disabled="!!testState" :loading="testing===row.id" @click="test(row)">测试连接</el-button>
      <el-button v-if="row.kind==='open115'" @click="authorize(row)">115扫码授权</el-button>
      <el-button type="danger" plain @click="askRemove(row)">删除</el-button>
    </div>
  </article>
<PagePagination label="服务连接分页" v-model:current-page="connectionPage" :page-size="10" :total="!loading?visible.length:undefined"/>

</div>
  <el-dialog v-model="editing" :title="form.id?'编辑连接':'添加连接'" width="min(760px,94vw)" :close-on-click-modal="false" class="u2-connection-dialog" :show-close="!saving" :close-on-press-escape="!saving">
    <form class="connection-editor" @submit.prevent="save()">
      <p>{{ kinds[form.kind]?.name }} · 密钥留空保留；填写新值才替换。关闭窗口会清空未提交的密钥。</p>
      <label>
        连接名称
        <el-input v-model="form.name" aria-label="连接名称" />
      </label>
      <div v-for="field in kinds[form.kind]?.fields" :key="field.key" class="connection-field">
        {{ field.label }}
        <template v-if="field.secret">
          <small>{{ saved?.secretFields.includes(field.key)?'已设置（默认隐藏）':'未设置' }}</small>
          <el-input v-model="form.secrets[field.key]" :type="'password'" :aria-label="field.label" autocomplete="new-password" placeholder="留空保留；填写新值替换" />
          <div v-if="saved?.secretFields.includes(field.key)" class="saved-secret-controls">
            <el-button v-if="!(field.key in reveal.values)" :loading="reveal.busy[field.key]" :aria-label="`显示${field.label}`" @click="reveal.show(field.key)">显示</el-button>
            <template v-else>
              <el-input :model-value="reveal.values[field.key]" :aria-label="`${field.label}已保存值`" readonly />
              <el-button :aria-label="`隐藏${field.label}`" @click="reveal.hide(field.key)">隐藏</el-button>
              <el-button :aria-label="`复制${field.label}`" @click="reveal.copy(field.key)">复制</el-button>
            </template>
          </div>
          <el-checkbox v-if="saved?.secretFields.includes(field.key)" v-model="form.clear" :value="field.key">明确清除{{ field.label }}</el-checkbox>
        </template>
        <el-input v-else v-model="form.settings[field.key]" :aria-label="field.label" />
        <small v-if="field.hint">{{ field.hint }}</small>
      </div>
      <div v-if="form.kind==='emby'">
        <el-checkbox v-model="form.settings.gatewayEnabled">通过网关直接播放</el-checkbox>
        <el-checkbox :model-value="true" disabled>只使用影片原格式，不中转或转码</el-checkbox>
        <p>播放器需要支持文件的原始格式；服务器不会转换视频或音频格式。</p>
      </div>
      <el-checkbox v-model="form.enabled">启用连接</el-checkbox>
      <p v-if="form.kind==='open115'">扫码授权不会自动启用；授权后可回到编辑窗口明确启用。</p>
      <p v-if="reveal.message.value" role="status">{{ reveal.message.value }}</p>
      <el-alert v-if="error" :title="error" type="warning" :closable="false" />
      <section v-if="conflict" class="connection-conflict">
        <h3>当前已保存配置（供对照）</h3>
        <p>{{ conflict.name }} · {{ conflict.enabled?'已启用':'未启用' }}</p>
        <dl>
          <template v-for="(value,key) in safeSettings(conflict.settings)" :key="key">
            <dt>{{ kinds[form.kind]?.fields.find(field=>field.key===key)?.label || '其他设置' }}</dt>
            <dd>{{ value }}</dd>
          </template>
        </dl>
        <p>凭据：{{ conflict.secretFields.length?'已设置（默认隐藏）':'未设置' }}</p>
        <el-button @click="approveRevision">保留输入并采用最新修订</el-button>
      </section>
      <div class="connection-buttons">
        <el-button :disabled="saving||blocked||!!conflict" :loading="saving" native-type="submit" type="primary" class="mt-save-button">{{ form.kind==='tmdb'?'保存TMDB Token':'加密保存' }}</el-button>
        <ElDropdown v-if="form.kind==='open115'" trigger="click">
          <el-button :disabled="saving||blocked||!!conflict">更多保存方式</el-button>
          <template #dropdown><ElDropdownMenu><ElDropdownItem :disabled="saving||blocked||!!conflict" @click="save(true)">保存并115扫码授权</ElDropdownItem></ElDropdownMenu></template>
        </ElDropdown>
        <el-button v-if="blocked" @click="load">核对结果</el-button>
      </div>
    </form>
  </el-dialog>
  <ConfirmAction v-model:visible="removeOpen" title="删除连接" :description="`将移除「${removeTarget?.name||'—'}」的连接配置。`" impact-scope="依赖此连接的服务将不可用；不会删除网盘文件、媒体或其他账号。" danger-level="danger" confirm-text="删除连接" :loading="removing" @confirm="remove" />
  <Official115AuthDialog v-model="authOpen" :integration-id="authId" @authorized="load" />
</section>
</template>
