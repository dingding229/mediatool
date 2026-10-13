<script setup lang="ts">
import { ref } from "vue";
import SiteMetrics from "../components/SiteMetrics.vue";
import PTAccountDialog from "../components/PTAccountDialog.vue";
const open = ref(false),
  editingID = ref(""),
  refreshKey = ref(0);
function edit(id: string) {
  editingID.value = id;
  open.value = true;
}
</script>
<template>
  <section class="page-heading">
    <div>
      <span class="page-kicker">ACQUIRE / 站点账号</span>
      <h1>PT 站点</h1>
      <p>集中管理站点账号、数据同步与认证状态。做种统计仅展示已确认结果。</p>
    </div>
    <el-button
      type="primary"
      @click="
        editingID = '';
        open = true;
      "
      >添加站点</el-button
    >
  </section>
  <SiteMetrics :key="refreshKey" @edit="edit" /><PTAccountDialog
    v-model="open"
    :editing-id="editingID"
    @saved="refreshKey++"
  />
</template>
