<script setup lang="ts">
defineProps<{excluded:number;showAll:boolean}>();
defineEmits<{toggle:[]}>();
</script>
<template>
 <div v-if="excluded>0||showAll" class="resource-identity-notice" role="note">
  <p v-if="!showAll">另有 {{excluded}} 条因与所选媒体身份不匹配被排除</p>
  <p v-else>已显示全部资源，其中 {{excluded}} 条与所选媒体身份不匹配；投递仍须通过身份校验。</p>
  <el-button @click="$emit('toggle')">{{showAll?'恢复媒体身份过滤':'显示全部（不按媒体身份过滤）'}}</el-button>
 </div>
</template>

<style scoped>
.resource-identity-notice{margin-block:12px 16px}
</style>
