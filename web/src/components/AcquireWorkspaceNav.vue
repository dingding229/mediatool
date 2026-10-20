<script setup lang="ts">
import { useRoute } from "vue-router";
const route = useRoute();
const links = [
  { path: "/search", label: "搜索资源", hint: "作品与站点直搜" },
  { path: "/subscriptions", label: "管理订阅", hint: "规则与自动追踪" },
  { path: "/sites", label: "站点账号", hint: "账号与数据" },
  { path: "/site-templates", label: "模板目录", hint: "按需加载" },
  { path: "/downloads", label: "下载任务", hint: "下载与传输" },
];
</script>
<template>
  <nav class="acquire-workspace-nav" aria-label="资源获取快捷导航">
    <div class="workspace-intro">
      <span class="section-kicker">资源获取</span
      ><strong>发现资源，持续追踪</strong>
    </div>
    <div class="workspace-links">
      <RouterLink
        v-for="link in links"
        :key="link.path"
        :to="link.path"
        :aria-current="route.path === link.path ? 'page' : undefined"
        ><span>{{ link.label }}</span
        ><small>{{ link.hint }}</small></RouterLink
      >
    </div>
    <div class="workspace-cache-links">
      <RouterLink to="/tmdb-cache">媒体缓存</RouterLink
      ><RouterLink to="/search-cache">站点缓存</RouterLink>
    </div>
  </nav>
</template>
<style scoped>
.acquire-workspace-nav {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: 18px 22px;
  border: 1px solid var(--border);
  background: var(--panel-bg);
  border-radius: 14px;
  margin-bottom: 24px;
}
.workspace-intro {
  flex-shrink: 0;
}
.workspace-intro strong {
  font-size: 14px;
}
.workspace-links {
  display: flex;
  gap: 8px;
  flex: 1;
  flex-wrap: wrap;
}
.workspace-links a {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 8px 12px;
  border-radius: 10px;
  min-height: 44px;
  color: var(--muted);
}
.workspace-links a[aria-current="page"],
.workspace-links a:hover {
  background: var(--accent-soft);
  color: var(--accent);
}
.workspace-links span {
  font-size: 13px;
  font-weight: 600;
}
.workspace-links small {
  font-size: 10px;
}
.workspace-cache-links {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--accent);
}
.workspace-cache-links a {
  min-height: 44px;
  display: flex;
  align-items: center;
}
@media (max-width: 1100px) {
  .acquire-workspace-nav {
    flex-wrap: wrap;
    gap: 14px;
  }
  .workspace-intro {
    width: 100%;
  }
  .workspace-cache-links {
    margin-left: auto;
  }
}
@media (max-width: 680px) {
  .acquire-workspace-nav {
    padding: 16px;
  }
  .workspace-links {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    width: 100%;
  }
  .workspace-links a {
    padding: 8px;
  }
  .workspace-links small {
    display: none;
  }
  .workspace-cache-links {
    margin-left: 0;
  }
}
</style>
