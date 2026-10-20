<script setup lang="ts">
import { computed } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import CacheView from '../views/CacheView.vue';
const route = useRoute(), router = useRouter(), kind = computed(() => route.query.cacheKind === 'search' ? 'search' : 'tmdb');
function setKind(value: string | number | boolean | undefined) {
  void router.replace({ query: { ...route.query, cacheKind: String(value) } });
}
</script>
<template>
  <div>
    <el-radio-group :model-value="kind" aria-label="缓存范围" @change="setKind">
      <el-radio-button value="tmdb">TMDB</el-radio-button>
      <el-radio-button value="search">搜索缓存</el-radio-button>
    </el-radio-group>
    <CacheView :kind="kind"/>
  </div>
</template>

<style scoped>
:deep(.el-radio-button__inner) {min-height:44px;display:inline-flex;align-items:center}
</style>
