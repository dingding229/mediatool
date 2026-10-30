import {expect,it} from 'vitest';
import {createSSRApp,h} from 'vue';
import {renderToString} from '@vue/server-renderer';
import Grid from '../ui/DescriptionGrid.vue';
import {downloadDetailGroups} from './download-details';
it('groups real download facts by meaning, with no placeholder-only media group',async()=>{
 const groups=downloadDetailGroups({state:'imported',progress:1,message:'',seedingState:'seeding',qbState:'uploading',endpoint:'http://qb.test',cleanupState:'disabled',media:{id:1,mediaType:'movie',title:'',year:''}},true,[{kind:'qbittorrent',name:'家庭下载器',settings:{endpoint:'http://qb.test'}}]);
 expect(groups.map(g=>g.key)).toEqual(['status','downloader','cleanup']);
 const html=await renderToString(createSSRApp({render:()=>h('main',groups.map(group=>h(Grid,{items:group.items})))}));
 for(const label of ['年份','同步时间','说明','下载速度','清理说明'])expect(html).not.toContain('>'+label+'<');
 expect(html).toContain('家庭下载器');expect(html).toContain('正在做种');expect(html).toContain('未启用自动清理');expect(html).not.toContain('>—<');
});
it('all four groups exist when populated, keeps zero connections and zero progress as data',()=>{
 const groups=downloadDetailGroups({state:'downloading',progress:0,media:{id:1,mediaType:'series',title:'作品',year:'2026'},seeds:0,peers:0,cleanupState:'blocked',cleanupMessage:'做种要求尚未满足'},true);
 expect(groups.map(g=>g.title)).toEqual(['状态','媒体','下载器与做种','清理条件']);expect(groups[0]!.items.find(i=>i.key==='progress')!.value).toBe('0%');expect(groups[2]!.items.find(i=>i.key==='seeds')!.value).toBe(0);
});
it('upload completion facts use the completed stage, do not invent a downloader or network byte count',()=>{const groups=downloadDetailGroups({state:'verified',processingProgress:90},false);expect(groups).toHaveLength(1);expect(groups[0]!.items.find(i=>i.key==='progress')).toMatchObject({label:'上传进度',value:'100%'});expect(downloadDetailGroups(null,false)).toEqual([])});
