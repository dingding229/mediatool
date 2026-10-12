<script setup lang="ts">
import { useRoute } from 'vue-router'
import { navigationGroups, navigationLocation } from '../navigation/items'
const route = useRoute()
defineEmits<{ navigate: [] }>()
</script>
<template>
  <nav class="navigation-groups" aria-label="功能导航">
    <section v-for="group in navigationGroups" :key="group.id" class="nav-group" :aria-label="group.name">
      <h2 class="nav-group-label">{{ group.name }}</h2>
      <RouterLink v-for="item in group.items" :key="item.id" :to="`/${item.id}`" class="nav-item"
        :class="{ 'is-active': navigationLocation(route.path)?.id === item.id }" @click="$emit('navigate')">
        <component :is="item.icon" aria-hidden="true" /><span>{{ item.name }}</span>
        <span v-if="navigationLocation(route.path)?.id === item.id" class="nav-active-marker" aria-hidden="true"></span>
      </RouterLink>
    </section>
  </nav>
</template>
