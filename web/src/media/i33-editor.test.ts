import {readFileSync} from 'node:fs';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import {setupComponent,flush} from '../test/setup-component';
import MetadataEditor from './MetadataEditor.vue';
import Detail from '../views/MediaWorkspaceView.vue';
import {ref} from 'vue';
const confirm=vi.hoisted(()=>vi.fn());
vi.mock('element-plus',async original=>({...await original<typeof import('element-plus')>(),ElMessageBox:{confirm}}));
vi.mock('./useCatalog',()=>({useCatalog:()=>({detail:ref({id:'movie'}),detailOpen:ref(true),refresh:vi.fn(),load:vi.fn()})}));
const read=(file:string)=>readFileSync(new URL(file,import.meta.url),'utf8');
const record={target:{mediaId:'root',kind:'movie',season:-1,episode:-1},targetKey:'root',revision:1,base:{title:'原名'},effective:{title:'原名'},overrides:{},matched:true,unmatched:[],choices:[]};
beforeEach(()=>{vi.clearAllMocks();vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({data:record})})));});
afterEach(()=>vi.unstubAllGlobals());
it('places label, input/helper and restore in one row, with an equal-width label column',()=>{
 const source=read('./MetadataEditor.vue'),row=source.slice(source.indexOf('<div v-for="key'),source.indexOf('<div class="mt-save-actions"'));
 expect(row).toMatch(/<label class="metadata-label"[^>]*:for=/);expect(row).toContain('class="metadata-field"');expect(row).toContain('class="metadata-origin" :title=');expect(row).toContain('class="metadata-restore"');expect(row).toContain('metadata-row--overview');
 expect(source).toContain('grid-template-columns:112px minmax(0,1fr) auto');expect(source).toContain('@media(max-width:480px)');
 const images=read('./ImageEditor.vue');expect(images).toMatch(/class="image-label"[\s\S]*class="image-preview"[\s\S]*class="image-actions"[\s\S]*class="image-restore"/);expect(images).not.toContain('PagePagination');
});
it('hides rather than unmounts read-only details on both host pages',()=>{
 const source=read('../views/MediaWorkspaceView.vue');expect(source).toContain('v-show="!editorOpen" class="catalog-readonly"');expect(source).toContain('v-show="!editorOpen" class="catalog-subtitle"');expect(source).toContain('<dl v-show="!editorOpen">');expect(source).toContain('v-show="activeEditor===');
 expect(source.indexOf('class="catalog-readonly"')).toBeLessThan(source.indexOf('aria-label="作品简介"'));expect(source.lastIndexOf('<SubtitlePanel')).toBeGreaterThan(source.indexOf('class="catalog-readonly"'));
 const organize=read('../views/OrganizeWorkspaceView.vue');expect(organize).toContain('v-show="!assetImagesOpen"');expect(organize).not.toContain('technical-data');
});
it('confirms dirty collapse, keeps drafts on both cancel and continue, and reopens without a read',async()=>{
 const v=setupComponent(MetadataEditor,{itemId:'movie',hideTrigger:true}),s=v.state;s.open();await flush();s.draft.title='保留草稿';
 confirm.mockRejectedValueOnce('cancel');await s.requestClose();expect(s.isOpen()).toBe(true);expect(s.draft.title).toBe('保留草稿');
 confirm.mockResolvedValueOnce('confirm');await s.requestClose();expect(s.isOpen()).toBe(false);s.open();await flush();expect(s.draft.title).toBe('保留草稿');expect(fetch).toHaveBeenCalledTimes(1);expect(confirm).toHaveBeenCalledTimes(2);v.unmount();
});
it('switch cancel leaves metadata visible; continue closes only the previous editor; busy blocks switching',async()=>{
 const v=setupComponent(Detail),s=v.state;let metadataOpen=true,imageOpen=false;const canClose=vi.fn(async()=>false);
 s.activeEditor='metadata';s.metadataEditor={busy:false,isOpen:()=>metadataOpen,canClose,close:()=>{metadataOpen=false},open:()=>{metadataOpen=true}};s.imageEditor={busy:false,isOpen:()=>imageOpen,close:()=>{imageOpen=false},open:()=>{imageOpen=true}};
 await s.edit('images');expect(s.activeEditor).toBe('metadata');expect(metadataOpen).toBe(true);expect(imageOpen).toBe(false);
 canClose.mockResolvedValueOnce(true);await s.edit('images');expect(s.activeEditor).toBe('images');expect(metadataOpen).toBe(false);expect(imageOpen).toBe(true);
 s.imageEditor.busy=true;await s.edit('metadata');expect(s.activeEditor).toBe('images');v.unmount();
});
it('read-only visibility follows actual editor open state and returns after collapse or save',async()=>{
 const v=setupComponent(Detail),s=v.state,metadataOpen=ref(false),imagesOpen=ref(false);
 s.metadataEditor={busy:false,isOpen:()=>metadataOpen.value,canClose:async()=>true,open:()=>{metadataOpen.value=true},close:()=>{metadataOpen.value=false}};
 s.imageEditor={busy:false,isOpen:()=>imagesOpen.value,open:()=>{imagesOpen.value=true},close:()=>{imagesOpen.value=false}};
 expect(s.editorOpen).toBe(false);await s.edit('metadata');expect(s.editorOpen).toBe(true);s.metadataEditor.close();expect(s.editorOpen).toBe(false);
 await s.edit('images');expect(s.editorOpen).toBe(true);expect(metadataOpen.value).toBe(false);await s.metadataSaved();expect(s.editorOpen).toBe(false);expect(s.activeEditor).toBe('');expect(s.load).toHaveBeenCalledTimes(1);v.unmount();
});
