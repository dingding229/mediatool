<script setup lang="ts">
import { computed } from 'vue'
import { Warning, CircleClose, InfoFilled, Lock } from '@element-plus/icons-vue'

export type StateKind = 'loading' | 'empty' | 'error' | 'unsupported' | 'unauthorized'

const props = withDefaults(defineProps<{
  compact?: boolean
  state: StateKind
  title?: string
  description?: string
  retryText?: string
}>(), {
  retryText: '重试'
})

const emit = defineEmits<{
  (e: 'retry'): void
}>()

const defaultTitles: Record<StateKind, string> = {
  loading: '正在载入数据…',
  empty: '暂无数据',
  error: '读取数据时发生错误',
  unsupported: '当前环境或服务不支持此功能',
  unauthorized: '暂无权限访问此内容'
}

const displayTitle = computed(() => props.title || defaultTitles[props.state])
</script>

<template>
  <div class="state-view" :class="[`state-${state}`,{'is-compact':compact}]" role="region" aria-live="polite">
    <div v-if="compact&&state!=='loading'&&state!=='empty'" class="state-inline-error"><span><strong>{{displayTitle}}</strong>{{description?'：'+description:''}}</span><el-button @click="emit('retry')">{{retryText}}</el-button></div>
    <div v-else-if="state === 'loading'" class="state-loading">
      <el-skeleton :rows="4" />
      <p class="state-hint">{{ displayTitle }}</p>
    </div>

    <div v-else-if="state === 'empty'" class="state-empty">
      <el-empty :image-size="80" :description="displayTitle">
        <template v-if="description" #default>
          <p class="state-desc">{{ description }}</p>
        </template>
      </el-empty>
      <div v-if="$slots.action" class="state-action">
        <slot name="action" />
      </div>
    </div>

    <div v-else-if="state === 'error'" class="state-error">
      <el-alert
        :title="displayTitle"
        :description="description"
        type="error"
        :closable="false"
        show-icon
      />
      <div class="state-action">
        <el-button plain @click="emit('retry')">{{ retryText }}</el-button>
        <slot name="action" />
      </div>
    </div>

    <div v-else-if="state === 'unsupported'" class="state-unsupported">
      <div class="state-icon-box">
        <Warning aria-hidden="true" />
      </div>
      <h3>{{ displayTitle }}</h3>
      <p v-if="description" class="state-desc">{{ description }}</p>
      <div v-if="$slots.action" class="state-action">
        <slot name="action" />
      </div>
    </div>

    <div v-else-if="state === 'unauthorized'" class="state-unauthorized">
      <div class="state-icon-box">
        <Lock aria-hidden="true" />
      </div>
      <h3>{{ displayTitle }}</h3>
      <p v-if="description" class="state-desc">{{ description }}</p>
      <div v-if="$slots.action" class="state-action">
        <slot name="action" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.state-view {
  padding: 24px 0;
  box-sizing: border-box;
  min-height: 300px;
  width: 100%;
}
.state-empty :deep(.el-empty){padding:16px 0}
.state-view.is-compact{min-height:0;padding:0}.state-inline-error{display:flex;align-items:center;gap:8px;font-size:12px;min-height:44px}.state-inline-error>span{flex:1;min-width:0;overflow-wrap:anywhere}.state-inline-error strong{color:var(--el-color-danger)}
.state-hint {
  margin-top: 12px;
  font-size: 13px;
  color: var(--muted);
  text-align: center;
}
.state-desc {
  font-size: 13px;
  color: var(--muted);
  margin-top: 8px;
  line-height: 1.6;
}
.state-action {
  margin-top: 16px;
  display: flex;
  justify-content: center;
  gap: 12px;
}
.state-unsupported, .state-unauthorized {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 16px;
  text-align: center;
  background: var(--panel-bg);
  border: 1px dashed var(--border);
  border-radius: var(--radius);
}
.state-icon-box {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: var(--mt-color-primary-light);
  color: var(--mt-color-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 16px;
}
.state-icon-box svg {
  width: 24px;
  height: 24px;
}
.state-unsupported h3, .state-unauthorized h3 {
  font-size: 16px;
  color: var(--text);
  margin-bottom: 8px;
}
</style>
