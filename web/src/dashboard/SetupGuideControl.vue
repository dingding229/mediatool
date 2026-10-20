<script setup lang="ts">
import { useRouter } from 'vue-router';
import { writeGuideHidden } from '../workflows/readiness';
const router = useRouter();
function open() {
  writeGuideHidden(false);
  window.dispatchEvent(new Event('media-tool:show-setup'));
  void router.push('/dashboard');
}
</script>
<template>
<section class="panel">
  <h2>首次使用引导</h2>
  <p>重新查看连接云盘、Emby、站点和分类规则的配置进度。</p>
  <el-button style="min-height:44px" @click="open">重新打开配置引导</el-button>
</section>
</template>
