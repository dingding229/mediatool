<script setup lang="ts">
import MiddleEllipsis from '../ui/MiddleEllipsis.vue';
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
    <el-tooltip v-for="item in items" :key="item.id" :content="item.title" :trigger="['hover','focus']" :trigger-keys="[]" popper-class="middle-ellipsis-tooltip"><button class="catalog-poster" type="button" :aria-label="item.title" @click="emit('select',item)">
      <PosterArtwork class="catalog-art" :src="image(item.poster||item.image)" :title="item.title"/>
      <strong><MiddleEllipsis :text="item.title" :copy="false" embedded/></strong>
      <small>{{item.year||'—'}} · {{item.source==='managed'?(item.category||mediaType(item.mediaType||item.type)):mediaType(item.mediaType||item.type)}}</small>
      <small v-if="item.season!=null">
        第
        {{item.season}}
        季
        <span v-if="item.episode!=null"> · 第 {{item.episode}} 集</span>
      </small>
    </button></el-tooltip>
  </div>
</template>
