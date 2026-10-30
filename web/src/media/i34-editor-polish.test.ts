import {afterEach,expect,it,vi} from 'vitest';
import {readFileSync} from 'node:fs';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
const read=(file:string)=>readFileSync(new URL(file,import.meta.url),'utf8');
afterEach(()=>vi.unstubAllGlobals());
it('only shows scraped hints for a locked/restoring field whose current value differs',async()=>{
 const base={title:'刮削标题',genres:['剧情','家庭']};const record={target:{mediaId:'root',kind:'movie'},base,effective:{...base,title:'人工标题'},overrides:{title:'人工标题'},matched:true,choices:[],unmatched:[]};
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({data:record})})));
 const v=setupComponent(MetadataEditor,{itemId:'item',hideTrigger:true}),s=v.state;s.open();await flush();expect(s.showOrigin('title')).toBe(true);s.draft.title='刮削标题';expect(s.showOrigin('title')).toBe(false);s.stageRestore('title');expect(s.showOrigin('title')).toBe(false);s.draft.title='恢复前待核对';expect(s.showOrigin('title')).toBe(true);
 s.draft.genres='剧情，家庭';expect(s.showOrigin('genres')).toBe(false);v.unmount();
});
it('uses a generic detail title, active segmented controls, sticky save actions and one shared chip',()=>{
 const detail=read('../views/MediaWorkspaceView.vue'),text=read('./MetadataEditor.vue'),images=read('./ImageEditor.vue');
 expect(detail).toContain('title="作品详情"');expect(detail).not.toContain(':title="detail?.title');expect(detail).toContain("'is-editing':editorOpen");expect(detail).toContain(':aria-pressed="editorOpen?');expect(text).toContain('@click="requestClose">完成编辑');expect(images).toContain("open?'完成编辑':'编辑图片'");
 expect(text).toContain('position:sticky;bottom:0');expect(text).toContain('background:var(--mt-bg-surface)');expect(text).toContain('border-top:1px solid var(--mt-border-color)');
 for(const file of [text,images]){expect(file).toContain("import './editor.css'");expect(file).toContain('class="metadata-lock"')}
 expect(images).toContain('.image-preview-poster{width:64px;height:96px;justify-self:start}');
});
