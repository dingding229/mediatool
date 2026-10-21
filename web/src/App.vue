<script setup lang="ts">
import { ElConfigProvider } from 'element-plus'
import 'element-plus/es/components/config-provider/style/css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
</script>
<template><ElConfigProvider :locale="zhCn"><RouterView /></ElConfigProvider></template>
