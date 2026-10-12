<script setup lang="ts">
import { computed, useId,ref,onMounted } from 'vue'
import {request}from'../api/client'
import { useRoute } from 'vue-router'
import { ArrowRight } from '@element-plus/icons-vue'
import { navigationGroups, navigationLocation } from '../navigation/items'
import { toggleNavigationGroup } from '../navigation/expansion'
const plugins=ref<any[]>([])
onMounted(async()=>{try{plugins.value=(await request<any[]>('/plugins')).filter(x=>x.enabled&&x.manifest.pages?.length)}catch{}})
const route = useRoute(), instanceId = useId()
const expanded = defineModel<string[]>('expanded', { default: () => [] })
const current = computed(() => navigationLocation(route.path))
defineEmits<{ navigate: [] }>()
</script>
<template>
  <nav class="navigation-groups" aria-label="功能导航">
    <section v-for="group in navigationGroups" :key="group.id" class="nav-group" :aria-label="group.name">
      <h2 class="nav-group-label" :aria-label="group.name">
        <button type="button" class="nav-group-toggle" :class="{ 'has-current': current?.group === group.name }"
          :aria-label="group.name" :aria-description="current?.group === group.name ? '包含当前页面' : undefined"
          :aria-expanded="expanded.includes(group.id)" :aria-controls="`${instanceId}-nav-${group.id}`"
          @click="expanded=toggleNavigationGroup(expanded,group.id)">
          <span>{{ group.name }}</span><span v-if="current?.group === group.name" class="nav-group-current" aria-hidden="true">当前</span>
          <ArrowRight class="nav-group-chevron" :class="{ 'is-expanded': expanded.includes(group.id) }" aria-hidden="true" />
        </button>
      </h2>
      <div :id="`${instanceId}-nav-${group.id}`" class="nav-group-items" :hidden="!expanded.includes(group.id)">
        <RouterLink v-for="item in group.items" :key="item.id" :to="`/${item.id}`" class="nav-item"
          :class="{ 'is-active': current?.id === item.id }" @click="$emit('navigate')">
          <component :is="item.icon" aria-hidden="true" /><span>{{ item.name }}</span>
          <span v-if="current?.id === item.id" class="nav-active-marker" aria-hidden="true"></span>
        </RouterLink>
      </div>
    </section>
    <section v-if="plugins.length" class="nav-group" aria-label="插件应用"><h2 class="nav-group-label"><button class="nav-group-toggle" type="button" :aria-expanded="expanded.includes('plugin-apps')" :aria-controls="`${instanceId}-plugin-apps`" @click="expanded=toggleNavigationGroup(expanded,'plugin-apps')"><span>插件应用</span><ArrowRight class="nav-group-chevron" :class="{'is-expanded':expanded.includes('plugin-apps')}" aria-hidden="true"/></button></h2><div :id="`${instanceId}-plugin-apps`" class="nav-group-items" :hidden="!expanded.includes('plugin-apps')"><template v-for="plugin in plugins" :key="plugin.manifest.id"><RouterLink v-for="page in plugin.manifest.pages" :key="page.id" :to="`/plugins/${plugin.manifest.id}/${page.id}`" class="nav-item" @click="$emit('navigate')">{{plugin.manifest.name}} · {{page.title}}</RouterLink></template></div></section>
  </nav>
</template>
