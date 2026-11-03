<script setup lang="ts">
import {ElTag,ElButton} from 'element-plus';
import 'element-plus/es/components/tag/style/css';
import 'element-plus/es/components/button/style/css';
import {serviceText} from '../presentation/service-text';
import type {NotificationMessage} from '../api/notifications';
import {date} from '../workflows/readiness';
defineProps<{item:NotificationMessage;blocked?:boolean}>();defineEmits<{retry:[]}>();
const labels:Record<string,string>={sent:'已发送',pending:'等待投递',retry:'等待重试',sending:'正在投递',outcome_unknown:'结果未知，需先在接收端核对',failed:'投递失败',not_configured:'仅站内通知'};
</script>
<template><article class="notification-entry"><header class="notification-entry-heading"><h3>{{serviceText(item.title)}}</h3><ElTag disable-transitions size="small" :type="item.telegramState==='sent'?'success':item.telegramState==='not_configured'?'info':'warning'">{{labels[item.telegramState]||'状态待核对'}}</ElTag></header><p v-if="item.body&&item.body!==item.title" class="notification-entry-body">{{serviceText(item.body)}}</p><small>{{date(item.createdAt)}}</small><div class="notification-entry-status"><p v-if="item.deliveryError">{{serviceText(item.deliveryError)}}</p><ElButton v-if="item.deliveryId&&['failed','outcome_unknown'].includes(item.telegramState)" :disabled="blocked" @click="$emit('retry')">人工重发</ElButton></div></article></template>
