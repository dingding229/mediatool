<script setup lang="ts">
import {computed} from 'vue';
import { dangerousScope } from '../api/plugins';
import { permissionName } from './model';
const props=defineProps<{
  permissions: Array<{
    scope: string;
    resources?: Record<string, string[]>;
  }>;
}>();
const groups=computed(()=>{const out=new Map<string,{label:string;danger:boolean;resources:Set<string>}>();for(const p of props.permissions){const label=permissionName(p.scope),danger=dangerousScope(p.scope),key=label+danger;const group=out.get(key)||{label,danger,resources:new Set<string>()};for(const values of Object.values(p.resources||{}))for(const v of values)group.resources.add(v);out.set(key,group)}return [...out.values()]});
</script>
<template>
 <ul class="u6-permissions"><li v-for="group in groups" :key="group.label+group.danger"><strong>{{group.label}}</strong><span v-if="group.danger"> · 高风险权限</span><p v-if="group.resources.size">访问范围：{{[...group.resources].join('、')}}</p></li></ul>
 <p v-if="!permissions.length">没有申请额外权限。</p>
</template>
