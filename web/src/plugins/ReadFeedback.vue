<script setup lang="ts">
defineProps<{state:{loading:boolean;loaded:boolean;error:string};caption:string;errorTitle?:string}>()
defineEmits<{retry:[]}>()
</script>
<template>
 <p v-if="state.loading" role="status" aria-live="polite">{{caption}}加载中…{{state.loaded?'（显示上次成功结果）':''}}</p>
 <template v-if="state.error">
  <el-alert :title="(errorTitle??caption+'读取失败')+'；'+state.error+(state.loaded?'。显示上次成功结果，请重试核对。':'。结果未知，未作为空列表处理。')" type="error" :closable="false" role="alert"/>
  <el-button :disabled="state.loading" @click="$emit('retry')">重试读取{{caption}}</el-button>
 </template>
</template>
<style scoped>p{line-height:1.7;overflow-wrap:anywhere}.el-button{min-height:44px;margin-top:8px;max-width:100%;white-space:normal}</style>
