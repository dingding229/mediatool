<script setup lang="ts">
import { computed, ref } from 'vue'
import { request, post } from '../api/client'
import { scanPercent } from '../files/workbench'
import StatusBadge from './StatusBadge.vue'
export interface Scan {id:string;root:string;state:string;files:number;directories:number;completedDirectories?:number;currentDirectory?:string;generation?:number;revision?:number;message:string;updatedAt?:string}
const props=defineProps<{scans:Scan[]}>(), emit=defineEmits<{changed:[]}>()
const page=ref(1), detail=ref<{scan:Scan;directories:{path:string;state:string}[]}|null>(null), dialog=ref(false), busy=ref(''), error=ref(''), cursorPage=ref(1)
const shown=computed(()=>props.scans.slice((page.value-1)*8,page.value*8))
async function control(scan:Scan, action:string){busy.value=scan.id;error.value='';try{await post(`/library/scans/${scan.id}/${action}`,{});emit('changed')}catch(e){error.value=e instanceof Error?e.message:'扫描控制失败'}finally{busy.value=''}}
async function inspect(scan:Scan, reset=true){if(reset)cursorPage.value=1;dialog.value=true;error.value='';busy.value=scan.id;try{detail.value=await request(`/library/scans/${scan.id}?page=${cursorPage.value}&pageSize=25`)}catch(e){error.value=e instanceof Error?e.message:'详情读取失败'}finally{busy.value=''}}
</script>
<template>
  <section class="panel section-gap scan-workbench"><div class="panel-title"><h2>扫描进度与覆盖证据</h2><RouterLink to="/organize-history?kind=scan">全部扫描历史</RouterLink></div>
    <el-alert v-if="error" :title="error" type="error" :closable="false"/>
    <el-empty v-if="!scans.length" description="尚无扫描记录"/>
    <article v-for="scan in shown" :key="scan.id" class="scan-card">
      <div class="scan-head"><code>{{scan.root}}</code><StatusBadge :state="scan.state"/></div>
      <p class="field-hint">{{scan.files}} 个文件 · {{scan.completedDirectories??0}} / {{scan.directories}} 个已知目录 · 对账代次 {{scan.generation??1}} · 连接修订 {{scan.revision??'—'}}</p>
      <el-progress v-if="scan.directories" :percentage="scanPercent(scan.completedDirectories??0,scan.directories)" :show-text="false"/><p v-if="scan.currentDirectory" class="field-hint">当前目录：<code>{{scan.currentDirectory}}</code></p>
      <p class="field-hint">{{scan.message||'扫描中；目录总量会随递归发现增加，进度不是预先已知的全树百分比。'}}</p>
      <div class="scan-actions"><el-button :loading="busy===scan.id" @click="inspect(scan)">查看目录证据</el-button><el-button v-if="['pending','running'].includes(scan.state)" :loading="busy===scan.id" @click="control(scan,'pause')">暂停扫描</el-button><el-button v-if="['paused','failed'].includes(scan.state)" :loading="busy===scan.id" @click="control(scan,'resume')">重新完整对账</el-button><el-button v-if="['pending','running','paused','failed'].includes(scan.state)" :loading="busy===scan.id" @click="control(scan,'cancel')">取消扫描</el-button></div>
    </article>
    <el-pagination v-model:current-page="page" :page-size="8" :total="scans.length" layout="prev,pager,next,total"/>
    <p class="field-hint">暂停、取消、断线、连接修订变化或扫描期间收到云端变化，均不能作为缺失证据。恢复从根目录重新校准，复用已有稳定文件ID，不重复建资产。</p>
  </section>
  <el-dialog v-model="dialog" title="扫描目录覆盖" width="min(760px,95vw)"><el-alert v-if="error" :title="error" type="error" :closable="false"/><div v-for="row in detail?.directories??[]" :key="row.path" class="cursor-row"><code>{{row.path}}</code><StatusBadge :state="row.state"/></div><el-pagination v-if="detail" v-model:current-page="cursorPage" :page-size="25" :total="detail.scan.directories" layout="prev,pager,next" @current-change="inspect(detail!.scan,false)"/></el-dialog>
</template>
<style scoped>
.scan-card{padding:18px 0;border-bottom:1px solid var(--border)}.scan-head,.scan-actions,.cursor-row{display:flex;align-items:center;flex-wrap:wrap;gap:12px}.scan-head>code,.cursor-row>code{flex:1;min-width:0;overflow-wrap:anywhere;white-space:normal}.scan-actions .el-button{margin-left:0;min-height:44px}.cursor-row{padding:14px 0;border-bottom:1px solid var(--border)}.el-pagination{overflow:auto;margin-top:20px}.scan-workbench code{overflow-wrap:anywhere}@media(max-width:680px){.scan-head>code{flex-basis:100%}}
</style>
