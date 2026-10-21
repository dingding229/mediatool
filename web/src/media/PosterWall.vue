<script setup lang="ts">
import { ref } from 'vue';
import { Film } from '@element-plus/icons-vue';
import { image, mediaType, type MediaItem } from './model';
defineProps<{
  items: MediaItem[];
}>();
const emit = defineEmits<{
  select: [
    MediaItem
  ];
}>(), broken = ref(new Set<string>());
</script>
<template>
  <div class="catalog-wall">
    <button v-for="item in items" :key="item.id" class="catalog-poster" type="button" :aria-label="item.title" @click="emit('select',item)">
      <span class="catalog-art">
        <img v-if="image(item.poster||item.image)&&!broken.has(item.poster||item.image||'')" :src="image(item.poster||item.image)" :alt="item.title+'海报'" loading="lazy" @error="broken.add(item.poster||item.image||'')"/>
        <span v-else class="catalog-placeholder">
          <Film aria-hidden="true"/>
          <small>暂无海报</small>
        </span>
      </span>
      <strong>{{item.title||'—'}}</strong>
      <small>{{item.year||'—'}} · {{mediaType(item.mediaType||item.type)}}</small>
      <small v-if="item.season!=null">
        第
        {{item.season}}
        季
        <span v-if="item.episode!=null"> · 第 {{item.episode}} 集</span>
      </small>
    </button>
  </div>
</template>
