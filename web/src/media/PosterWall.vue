<script setup lang="ts">
import PosterArtwork from './PosterArtwork.vue';
import { image, mediaType, type MediaItem } from './model';
defineProps<{
  items: MediaItem[];
}>();
const emit = defineEmits<{
  select: [
    MediaItem
  ];
}>();
</script>
<template>
  <div class="catalog-wall">
    <button v-for="item in items" :key="item.id" class="catalog-poster" type="button" :aria-label="item.title" @click="emit('select',item)">
      <PosterArtwork class="catalog-art" :src="image(item.poster||item.image)" :title="item.title"/>
      <strong>{{item.title||'—'}}</strong>
      <small>{{item.year||'—'}} · {{item.source==='managed'?(item.category||mediaType(item.mediaType||item.type)):mediaType(item.mediaType||item.type)}}</small>
      <small v-if="item.season!=null">
        第
        {{item.season}}
        季
        <span v-if="item.episode!=null"> · 第 {{item.episode}} 集</span>
      </small>
    </button>
  </div>
</template>
