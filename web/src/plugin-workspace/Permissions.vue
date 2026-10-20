<script setup lang="ts">
import { dangerousScope } from '../api/plugins';
import { permissionName } from './model';
defineProps<{
  permissions: Array<{
    scope: string;
    resources?: Record<string, string[]>;
  }>;
}>();
</script>
<template>
  <ul class="u6-permissions">
    <li v-for="p in permissions" :key="p.scope">
      <strong>{{ permissionName(p.scope) }}</strong>
      <span v-if="dangerousScope(p.scope)"> · 高风险权限</span>
      <code>{{ p.scope }}</code>
      <div v-for="(values,key) in p.resources||{}" :key="key">
        <span>{{ key }}：</span>
        {{ values.join('、') }}
      </div>
    </li>
  </ul>
  <p v-if="!permissions.length">没有申请额外权限。</p>
</template>
