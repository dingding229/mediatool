<script setup lang="ts">
import { ref } from 'vue';
import PageHeader from '../ui/PageHeader.vue';
import StateView from '../ui/StateView.vue';
import DetailDrawer from '../ui/DetailDrawer.vue';
import { useCatalog } from '../media/useCatalog';
import { image, originalSTRM, mediaType, cloudState, sourceState } from '../media/model';
import { errorText } from '../workflows/readiness';
import '../media/media.css';
const { source, view, page, query, kind, parentId, user, pluginKind, items, total, home, groups, hasMore, truncated, loading, error, detail, detailOpen, detailLoading, detailError, load, search, chooseSource, all, browse, inspect, homeItem } = useCatalog();
const broken = ref(new Set<string>());
</script>
<template>
  <div class="media-workspace">
    <PageHeader title="媒体库" description="按来源查找已入库的影视内容，并选择可用的播放方式。">
      <template #actions>
        <RouterLink to="/connections?tab=emby">Emby 配置</RouterLink>
        <el-button type="primary" :loading="loading" @click="load">刷新媒体库</el-button>
      </template>
    </PageHeader>
    <nav class="media-source-switch" aria-label="媒体库来源">
      <button v-for="s in [{id:'managed',name:'本程序入库'},{id:'emby',name:'Emby'},{id:'plugins',name:'插件来源'}]" :key="s.id" type="button" :aria-pressed="source===s.id" @click="chooseSource(s.id)">{{ s.name }}</button>
    </nav>
    <section class="media-surface">
      <header class="media-section-heading">
        <h2>{{ source==='managed'?'本程序入库':source==='plugins'?'插件提供的发现与推荐':'Emby 媒体库' }}</h2>
        <div v-if="source==='emby'" class="media-actions">
          <el-button :aria-pressed="view==='home'" @click="view='home';load()">Emby 首页</el-button>
          <el-button :aria-pressed="view==='all'" @click="all">全部媒体</el-button>
        </div>
      </header>
      <form v-if="source!=='emby'||view==='all'" class="media-filter-row" @submit.prevent="search">
        <template v-if="source!=='plugins'">
          <label>
            标题
            <el-input v-model="query" aria-label="搜索媒体标题" clearable/>
          </label>
          <label>
            类型
            <el-select v-model="kind" aria-label="媒体类型筛选" @change="search">
              <el-option value="" label="全部类型"/>
              <el-option value="movie" label="电影"/>
              <el-option value="series" label="电视剧"/>
            </el-select>
          </label>
          <el-button native-type="submit">搜索</el-button>
        </template>
        <label v-else>
          插件条目范围
          <el-select v-model="pluginKind" aria-label="插件来源范围" @change="search">
            <el-option value="discover.source" label="发现"/>
            <el-option value="recommend.source" label="推荐"/>
          </el-select>
        </label>
      </form>
      <div v-else-if="home" class="media-actions">
        <label>
          观看用户
          <el-select v-model="user" aria-label="Emby观看用户" @change="load">
            <el-option v-for="person in home.users" :key="person.id" :value="person.id" :label="person.name"/>
          </el-select>
        </label>
        <el-button @click="load">刷新首页</el-button>
      </div>
      <p v-if="source==='plugins'">插件条目只读，不执行插件命令，也不代表媒体已经入库。</p>
      <el-button v-if="parentId" @click="parentId='';search()">返回全部分类</el-button>
    </section>
    <StateView v-if="loading" state="loading" title="正在读取媒体来源…"/>
    <StateView v-else-if="error" :state="sourceState(error)" title="当前媒体来源暂不可用" :description="errorText(error)" @retry="load"/>
    <template v-else-if="source==='emby'&&view==='home'">
      <section v-for="group in groups" :key="group.key" class="media-surface">
        <header class="media-section-heading">
          <h2>{{ group.title }}</h2>
          <span>{{ group.items.length }} 项</span>
        </header>
        <div class="media-poster-grid">
          <button v-for="item in group.items" :key="item.id" class="media-poster-card" type="button" @click="homeItem(group.key,item)">
            <img v-if="image(item.image)&&!broken.has(item.image)" :src="image(item.image)" :alt="item.title" @error="broken.add(item.image)"/>
            <span v-else class="media-poster-empty">{{ item.title }}</span>
            <strong>{{ item.title }} {{ item.subtitle||'' }}</strong>
            <span>{{ mediaType(item.type) }}</span>
            <el-progress v-if="item.progress!=null" :percentage="Math.min(100,Math.max(0,item.progress))"/>
          </button>
        </div>
        <p v-if="!group.items.length">当前用户暂无{{ group.title }}内容</p>
      </section>
    </template>
    <section v-else class="media-surface">
      <header class="media-section-heading">
        <h2>浏览媒体</h2>
        <span>{{ total??'—' }} 项</span>
      </header>
      <div class="media-poster-grid">
        <button v-for="item in items" :key="item.source+item.id" type="button" class="media-poster-card" @click="inspect(item)">
          <img v-if="image(item.poster)&&!broken.has(item.poster!)" :src="image(item.poster)" :alt="item.title+' 海报'" @error="broken.add(item.poster!)"/>
          <span v-else class="media-poster-empty">暂无海报</span>
          <strong>{{ item.title }}</strong>
          <span>{{ item.year||'—' }} · {{ mediaType(item.mediaType) }}</span>
          <span v-if="item.season!=null">
            第
            {{ item.season }}
            季
            <span v-if="item.episode!=null"> · 第 {{ item.episode }} 集</span>
          </span>
          <span v-if="source==='managed'">{{ cloudState(item.state) }}</span>
        </button>
      </div>
      <StateView v-if="!items.length" state="empty" title="当前来源暂无匹配媒体" description="可调整筛选条件，或切换其他来源。"/>
      <p v-if="truncated" role="status">插件仅提供了部分条目，没有显示不代表没有更多内容。</p>
      <div v-if="source==='plugins'" class="media-actions">
        <el-button :disabled="page===1" @click="page--;load()">上一页</el-button>
        <span>第 {{ page }} 页</span>
        <el-button :disabled="!hasMore" @click="page++;load()">下一页</el-button>
      </div>
      <el-pagination v-else v-model:current-page="page" :page-size="24" :total="total||0" layout="prev,pager,next,total" @current-change="load"/>
    </section>
    <DetailDrawer :key="detail?.id" v-model:visible="detailOpen" :title="detail?.title||'媒体详情'" :technical-data="detail" class="media-detail">
      <StateView v-if="detailLoading" state="loading" title="正在读取详情…"/>
      <StateView v-if="detailError" :state="sourceState(detailError)" title="详情暂不可用" :description="errorText(detailError)"/>
      <p>{{ detail?.year||'—' }} · {{ mediaType(detail?.mediaType) }}</p>
      <p>{{ detail?.overview||'暂无简介' }}</p>
      <p v-if="detail?.filePath">文件位置：{{ detail.filePath }}</p>
      <a v-if="originalSTRM(detail)" :href="originalSTRM(detail)" download>下载原始 STRM 播放</a>
      <p v-if="originalSTRM(detail)">用兼容播放器打开下载的 STRM；保持原始固定链接，由服务端按播放器实际请求跳转，不提供转码、中转或 HLS。文件包含该媒体的播放权限，请勿公开分享。</p>
      <StateView v-else state="unsupported" title="此条目未提供可用播放入口" description="媒体信息不等于播放授权；这里不拼接媒体服务流地址，也不代替播放器登录。"/>
      <RouterLink v-if="detail?.instanceId" :to="{path:'/plugins',query:{instance:detail.instanceId}}">查看来源插件</RouterLink>
      <RouterLink v-if="detail?.filePath" to="/files?tab=browser">打开文件管理</RouterLink>
      <RouterLink v-if="source==='emby'" to="/connections?tab=emby">检查 Emby 连接与播放网关</RouterLink>
    </DetailDrawer>
  </div>
</template>
