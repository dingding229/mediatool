<script setup lang="ts">
import {onMounted,reactive,ref} from 'vue';
import {getGeoStatus,saveGeoSettings} from '../api/ip-geolocation';
import {useResource} from '../workflows/useResource';
import ResourceState from '../workflows/ResourceState.vue';
import IpAttribution from '../ui/IpAttribution.vue';
import {date,errorText} from '../workflows/readiness';
const state=reactive(useResource(getGeoStatus)),download=ref(''),decrypt=ref(''),busy=ref(false),error=ref<unknown>(),message=ref('');
async function save(){busy.value=true;error.value=undefined;message.value='';try{await saveGeoSettings(download.value,decrypt.value);download.value='';decrypt.value='';message.value='授权已保存，后台将在一分钟内尝试更新；失败保留旧库。';await state.load()}catch(e){error.value=e}finally{busy.value=false}}
onMounted(state.load);
</script>
<template>
 <section class="settings-card f7-geo"><h2>关于 · IP 归属地</h2>
 <p>纯真社区版 CZDB · 本地查询，无在线 IP 查询。未配置库时只显示公网 IP；无公网地址显示“内网访问”。</p>
 <ResourceState :stale="state.loaded" :loading="state.loading" :error="state.error" @reload="state.load"/>
 <p>授权 {{state.data?.configured?'已设置':'未设置'}} · IPv4 {{state.data?.ipv4?'可用':'无库'}} / IPv6 {{state.data?.ipv6?'可用':'无库'}} · 更新 {{date(state.data?.updatedAt||undefined)}}</p>
 <p>在纯真官网登录并提交署名页面申请授权，取得下载链接中的 key 和 CZDB 解密密钥。密钥使用现有 master key 加密保存，留空保留原值；不记录日志、不分发进镜像。每周自动更新，下载失败保留旧库。</p>
 <label class="u5-field">下载授权 key <el-input v-model="download" type="password" autocomplete="new-password" aria-label="纯真下载授权 key" placeholder="留空保留原值"/></label>
 <label class="u5-field">CZDB 解密密钥 <el-input v-model="decrypt" type="password" autocomplete="new-password" aria-label="CZDB 解密密钥" placeholder="Base64 密钥，留空保留原值"/></label>
 <p v-if="error" role="alert">{{errorText(error)}}</p><p role="status">{{message}}</p>
 <el-button :loading="busy" :disabled="!download&&!decrypt" @click="save">保存授权</el-button><IpAttribution/>
 </section>
</template>
<style scoped>.f7-geo{min-height:370px}.f7-geo>p{font-size:.85rem;line-height:1.6}.f7-geo label{margin:12px 0}</style>
