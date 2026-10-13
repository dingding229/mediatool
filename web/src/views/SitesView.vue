<script setup lang="ts">
import{ref}from'vue';import SiteMetrics from'../components/SiteMetrics.vue';import PTAccountDialog from'../components/PTAccountDialog.vue'
const open=ref(false),editingID=ref(''),refreshKey=ref(0)
function edit(id:string){editingID.value=id;open.value=true}
</script>
<template><section class="page-heading"><div><h1>PT 站点</h1><p>站点账号、上传下载和做种统计。</p></div><el-button type="primary" @click="editingID='';open=true">添加站点</el-button></section><SiteMetrics :key="refreshKey" @edit="edit"/><PTAccountDialog v-model="open" :editing-id="editingID" @saved="refreshKey++"/></template>
