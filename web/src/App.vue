<script setup lang="ts">
import TopProgress from './ui/TopProgress.vue'
import { ElConfigProvider } from 'element-plus'
import 'element-plus/es/components/config-provider/style/css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>
<template><ElConfigProvider :locale="zhCn"><TopProgress/><RouterView /></ElConfigProvider></template>
