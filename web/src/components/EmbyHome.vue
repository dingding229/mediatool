<script setup lang="ts">
import{ref,onMounted,computed}from'vue';import{request}from'../api/client';import{Film,FolderOpened}from'@element-plus/icons-vue'
interface Card{id:string;title:string;type:string;image:string;progress?:number;subtitle?:string;year?:number}
interface Home{users:Array<{id:string;name:string}>;userId:string;libraries:Card[];playing:Card[];resume:Card[];recent:Card[];latest:Card[]}
const emit=defineEmits<{browse:[id:string];selected:[id:string]}>()
const home=ref<Home|null>(null),user=ref(''),busy=ref(false),error=ref(''),broken=ref(new Set<string>())
const groups=computed(()=>[{title:'媒体库分类',kind:'libraries',items:home.value?.libraries??[]},{title:'正在播放',kind:'playing',items:home.value?.playing??[]},{title:'继续观看',kind:'resume',items:home.value?.resume??[]},{title:'近期观看',kind:'recent',items:home.value?.recent??[]},{title:'最新入库',kind:'latest',items:home.value?.latest??[]}])
async function load(){busy.value=true;error.value='';try{home.value=await request<Home>(`/media/library/emby/home?userId=${encodeURIComponent(user.value)}`);user.value=home.value.userId}catch(e){error.value=e instanceof Error?e.message:'媒体库读取失败'}finally{busy.value=false}}
function open(kind:string,item:Card){if(kind==='libraries')emit('browse',item.id);else emit('selected',item.id)}
defineExpose({refresh:load})
onMounted(load)
</script>
<template><section class="emby-home-heading"><div class="emby-symbol"><Film aria-hidden="true"/></div><div><h2>我的媒体库 <span>Emby</span></h2><p>浏览分类、正在播放、近期观看与最新入库</p></div><div class="emby-home-actions"><el-select v-if="home" v-model="user" aria-label="Emby观看用户" @change="load"><el-option v-for="u in home.users" :key="u.id" :label="u.name" :value="u.id"/></el-select><el-button :loading="busy" @click="load">刷新首页</el-button></div></section><el-alert v-if="error" :title="error" type="warning" :closable="false"/><section v-for="group in groups" :key="group.kind" v-loading="busy" class="panel emby-section"><div class="emby-section-heading"><h2>{{group.title}}</h2><p>{{busy&&!home?'—':group.items.length}} {{group.kind==='libraries'?'个媒体库':'项'}}</p></div><div class="emby-card-grid" :class="{'portrait-grid':['resume','recent','latest'].includes(group.kind)}"><button v-for="item in group.items" :key="item.id" class="emby-card" @click="open(group.kind,item)"><div class="emby-art"><img v-if="item.image&&!broken.has(item.image)" :src="item.image" :alt="item.title" loading="lazy" @error="broken.add(item.image)"/><div v-else class="emby-no-art"><FolderOpened v-if="group.kind==='libraries'" aria-hidden="true"/><Film v-else aria-hidden="true"/><span>{{item.title}}</span></div><span v-if="group.kind!=='libraries'" class="emby-type">{{item.type==='Movie'?'电影':item.type==='Episode'||item.type==='Series'?'电视剧':'媒体'}}</span><div v-if="item.progress" class="emby-progress"><i :style="{width:item.progress+'%'}"></i></div></div><h3>{{item.title}}<span v-if="item.subtitle"> {{item.subtitle}}</span></h3></button></div><p v-if="!busy&&!group.items.length" class="emby-empty">{{error?'等待有效的Emby连接':'当前用户暂无'+group.title+'内容'}}</p></section></template>
<style scoped>
.emby-home-heading { display:flex; align-items:center; gap:16px; padding:20px 24px; background:var(--panel-bg); border:1px solid var(--border); border-radius:var(--radius); margin-bottom:24px; }
.emby-symbol { width:48px; height:48px; display:grid; place-items:center; background:var(--accent-soft); border:1px solid #d5e7de; color:var(--accent); border-radius:10px; flex-shrink:0; }
.emby-symbol svg { width:24px; height:24px; }
.emby-home-heading h2 { font-size:20px; display:flex; align-items:center; gap:10px; }
.emby-home-heading h2 span { font-size:11px; padding:3px 8px; border:1px solid #c9dfd4; background:var(--accent-soft); color:var(--accent); border-radius:5px; }
.emby-home-heading p,.emby-section-heading p { color:var(--muted); font-size:13px; margin-top:6px; }
.emby-home-actions { margin-left:auto; display:flex; gap:12px; }
.emby-home-actions .el-select { width:160px; }
.emby-section { margin-bottom:24px; }
.emby-section-heading { display:flex; justify-content:space-between; align-items:center; gap:12px; margin-bottom:20px; }
.emby-card-grid { display:grid; grid-template-columns:repeat(auto-fill,minmax(210px,1fr)); gap:20px; }
.portrait-grid { grid-template-columns:repeat(auto-fill,minmax(150px,1fr)); }
.emby-card { min-width:0; text-align:left; padding:0; color:var(--text); background:none; border-radius:8px; }
.emby-art { aspect-ratio:16/10; background:#223632; border:1px solid #334c43; border-radius:9px; overflow:hidden; position:relative; }
.portrait-grid .emby-art { aspect-ratio:2/3; }
.emby-art img { width:100%; height:100%; object-fit:cover; display:block; }
.emby-no-art { display:flex; flex-direction:column; align-items:center; justify-content:center; height:100%; padding:18px; gap:12px; color:#c8ddd2; }
.emby-no-art svg { width:32px; height:32px; color:#a9c8b6; }
.emby-no-art span { font-size:14px; text-align:center; }
.emby-type { position:absolute; top:10px; left:10px; background:#152a20df; border:1px solid #526e60; color:#fff; padding:3px 8px; border-radius:5px; font-size:11px; }
.emby-progress { position:absolute; bottom:0; left:0; right:0; height:4px; background:#ffffff35; }
.emby-progress i { display:block; height:100%; background:#9dd4b9; }
.emby-card h3 { font-size:14px; font-weight:550; line-height:1.6; margin-top:10px; overflow-wrap:anywhere; }
.emby-card h3 span { color:var(--muted); font-size:12px; }
.emby-empty { color:var(--muted); font-size:13px; padding:20px; background:#f7f9f8; border:1px dashed #d9e3dd; border-radius:8px; }
@media(max-width:680px) { .emby-home-heading { flex-wrap:wrap; padding:18px; gap:12px; } .emby-home-actions { width:100%; margin-left:0; } .emby-home-actions .el-select { flex:1; min-width:0; } .emby-home-heading h2 { font-size:18px; } .emby-card-grid,.portrait-grid { grid-template-columns:repeat(2,minmax(0,1fr)); gap:16px 12px; } .emby-no-art { padding:12px; } .emby-no-art span { font-size:13px; } .emby-card h3 { font-size:13px; } }
</style>
